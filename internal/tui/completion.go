package tui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/taggedfile"
	tea "github.com/charmbracelet/bubbletea"
)

const taggedFileCompletionLimit = 8

type completionKind int

const (
	completionKindNone completionKind = iota
	completionKindSlash
	completionKindTagged
)

type completionSuggestion struct {
	label       string
	replacement string
	description string
}

type completionCandidate struct {
	kind        completionKind
	key         string
	start       int
	end         int
	prefix      string
	suggestions []completionSuggestion
}

type completionState struct {
	suppressedKey string
	candidate     completionCandidate
}

func (m Model) currentCompletionCandidate() (completionCandidate, bool) {
	if m.state != stateHome || m.completion.candidate.kind == completionKindNone {
		return completionCandidate{}, false
	}
	return m.completion.candidate, true
}

func (m *Model) refreshCompletionCandidate() {
	if m.state != stateHome {
		m.completion.candidate = completionCandidate{}
		return
	}
	input := m.goalInput.Value()
	cursor := m.goalInputCursorByteIndex()

	if candidate, ok := m.taggedCompletionCandidate(input, cursor); ok {
		m.completion.candidate = candidate
		return
	}
	if candidate, ok := m.slashCompletionCandidate(input, cursor); ok {
		m.completion.candidate = candidate
		return
	}
	m.completion.candidate = completionCandidate{}
}

func (m *Model) setGoalInputValue(value string) {
	m.goalInput.SetValue(value)
	m.refreshCompletionCandidate()
}

func (m Model) completionVisible() (completionCandidate, bool) {
	candidate, ok := m.currentCompletionCandidate()
	if !ok || candidate.key == m.completion.suppressedKey {
		return completionCandidate{}, false
	}
	return candidate, true
}

func (m Model) handleCompletionKey(msg string) (tea.Model, tea.Cmd, bool) {
	candidate, ok := m.completionVisible()
	if !ok {
		return m, nil, false
	}

	switch msg {
	case "esc":
		m.completion.suppressedKey = candidate.key
		return m, nil, true
	case "enter", "tab":
		next, cmd := m.applyCompletionCandidate(candidate)
		return next, cmd, true
	default:
		return m, nil, false
	}
}

func (m Model) applyCompletionCandidate(candidate completionCandidate) (tea.Model, tea.Cmd) {
	if len(candidate.suggestions) == 0 {
		return m, nil
	}

	replacement := candidate.suggestions[0].replacement
	input := m.goalInput.Value()
	if candidate.start < 0 || candidate.end < candidate.start || candidate.end > len(input) {
		return m, nil
	}

	updated := input[:candidate.start] + replacement + input[candidate.end:]
	m.goalInput.SetValue(updated)
	m.resizeGoalEditor()
	m.refreshCompletionCandidate()
	m.completion.suppressedKey = candidate.kind.String() + ":" + replacement

	if reviewExtraFocus, ok := parseReviewCommand(updated); ok {
		return m.handleReviewCommand(reviewExtraFocus)
	}
	if strings.TrimSpace(updated) == designCommand {
		return m.handleDesignCommand()
	}

	return m, nil
}

func (m *Model) pruneCompletionSuppression() {
	if m.completion.suppressedKey == "" {
		return
	}

	candidate, ok := m.currentCompletionCandidate()
	if !ok || candidate.key != m.completion.suppressedKey {
		m.completion.suppressedKey = ""
	}
}

func (m Model) slashCompletionCandidate(input string, cursor int) (completionCandidate, bool) {
	if input == "" || input[0] != '/' {
		return completionCandidate{}, false
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(input) {
		cursor = len(input)
	}

	tokenEnd := 0
	for tokenEnd < len(input) {
		r, width := utf8.DecodeRuneInString(input[tokenEnd:])
		if unicode.IsSpace(r) {
			break
		}
		tokenEnd += width
	}
	tokenText := strings.TrimRight(input[:tokenEnd], ".,;:)]}")
	if tokenText == "" {
		return completionCandidate{}, false
	}

	prefixEnd := cursor
	if prefixEnd > tokenEnd {
		prefixEnd = tokenEnd
	}
	prefix := input[:prefixEnd]

	suggestions := m.slashCompletionSuggestions(prefix)
	if len(suggestions) == 0 {
		return completionCandidate{}, false
	}

	return completionCandidate{
		kind:        completionKindSlash,
		key:         completionKindSlash.String() + ":" + tokenText,
		start:       0,
		end:         tokenEnd,
		prefix:      prefix,
		suggestions: suggestions,
	}, true
}

func (m Model) taggedCompletionCandidate(input string, cursor int) (completionCandidate, bool) {
	ref, ok := taggedfile.ActiveTokenAt(input, cursor)
	if !ok {
		return completionCandidate{}, false
	}

	prefixEnd := cursor
	if prefixEnd < ref.ContentStart {
		prefixEnd = ref.ContentStart
	}
	if prefixEnd > ref.End {
		prefixEnd = ref.End
	}
	prefix := input[ref.ContentStart:prefixEnd]

	var externalRoots []taggedfile.ExternalRoot
	if len(m.externalRoots) > 0 {
		externalRoots = m.externalRoots
	} else if m.eng != nil {
		externalRoots = m.eng.ExternalRoots()
	}

	suggestions, err := taggedfile.Complete(m.root, prefix, externalRoots, taggedFileCompletionLimit, taggedfile.DefaultCompletionSkip)
	if err != nil || len(suggestions) == 0 {
		return completionCandidate{}, false
	}

	items := make([]completionSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		replacement := formatTaggedFileCompletion(ref.Quoted, suggestion.Path)
		items = append(items, completionSuggestion{
			label:       replacement,
			replacement: replacement,
			description: taggedFileSuggestionDescription(suggestion.Path, suggestion.IsDir),
		})
	}

	return completionCandidate{
		kind:        completionKindTagged,
		key:         completionKindTagged.String() + ":" + input[ref.Start:ref.End],
		start:       ref.Start,
		end:         ref.End,
		prefix:      prefix,
		suggestions: items,
	}, true
}

func (m Model) slashCompletionSuggestions(prefix string) []completionSuggestion {
	var suggestions []completionSuggestion
	for _, suggestion := range m.slashCommandSuggestions() {
		if !strings.HasPrefix(suggestion.command, prefix) {
			continue
		}
		suggestions = append(suggestions, completionSuggestion{
			label:       suggestion.command,
			replacement: suggestion.command,
			description: suggestion.description,
		})
	}
	return suggestions
}

func formatTaggedFileCompletion(quoted bool, path string) string {
	if !quoted && !needsTaggedFileQuotes(path) {
		return "@" + path
	}
	var b strings.Builder
	b.WriteString(`@"`)
	b.WriteString(strings.ReplaceAll(strings.ReplaceAll(path, `\`, `\\`), `"`, `\"`))
	b.WriteString(`"`)
	return b.String()
}

func needsTaggedFileQuotes(path string) bool {
	if path == "" {
		return true
	}
	for _, r := range path {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			continue
		case r == '/', r == '.', r == '-', r == '_', r == '~', r == '+':
			continue
		default:
			return true
		}
	}
	switch path[len(path)-1] {
	case '.', ',', ';', ':', ')', ']', '}':
		return true
	default:
		return false
	}
}

func taggedFileSuggestionDescription(_ string, isDir bool) string {
	if isDir {
		return "directory"
	}
	return "file"
}

func (k completionKind) String() string {
	switch k {
	case completionKindSlash:
		return "slash"
	case completionKindTagged:
		return "tagged"
	default:
		return "none"
	}
}

func (m Model) goalInputCursorByteIndex() int {
	input := m.goalInput.Value()
	if input == "" {
		return 0
	}

	lines := strings.Split(input, "\n")
	row := m.goalInput.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}

	li := m.goalInput.LineInfo()
	runeIndex := li.StartColumn + li.CharOffset
	if runeIndex < 0 {
		runeIndex = 0
	}
	if runeIndex > utf8.RuneCountInString(lines[row]) {
		runeIndex = utf8.RuneCountInString(lines[row])
	}

	offset := 0
	for i := 0; i < row; i++ {
		offset += len(lines[i]) + 1
	}
	return offset + runeIndexToByteIndex(lines[row], runeIndex)
}

func runeIndexToByteIndex(s string, idx int) int {
	if idx <= 0 {
		return 0
	}

	runeCount := 0
	for byteIndex := range s {
		if runeCount == idx {
			return byteIndex
		}
		runeCount++
	}
	return len(s)
}
