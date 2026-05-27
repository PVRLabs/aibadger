package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

type goalFocusState int

const (
	goalFocusEditor goalFocusState = iota
	goalFocusAttachments
)

type goalAttachmentType string

const (
	goalAttachmentGitDiff goalAttachmentType = "git diff"
	goalAttachmentText    goalAttachmentType = "text"
)

var (
	goalPasteAttachmentByteThreshold = 16 * 1024
	goalPasteAttachmentLineThreshold = 40
)

const (
	goalPasteBurstWindow = 20 * time.Millisecond
	goalPasteFlushDelay  = time.Second
)

type goalAttachment struct {
	Type         goalAttachmentType
	Source       string
	Summary      string
	Text         string
	SizeBytes    int64
	Lines        int
	FilesChanged int
	Additions    int
	Deletions    int
}

func newGoalTextAttachment(source, text string) goalAttachment {
	return newGoalAttachment(goalAttachmentText, source, text, 0, 0, 0)
}

func newGoalGitDiffAttachment(source, diff string) goalAttachment {
	return newGoalAttachment(goalAttachmentGitDiff, source, diff, 0, 0, 0)
}

func newGoalGitDiffAttachmentWithStats(source, diff string, filesChanged, additions, deletions int) goalAttachment {
	return newGoalAttachment(goalAttachmentGitDiff, source, diff, filesChanged, additions, deletions)
}

func newGoalAttachment(kind goalAttachmentType, source, text string, filesChanged, additions, deletions int) goalAttachment {
	attachment := goalAttachment{
		Type:         kind,
		Source:       strings.TrimSpace(source),
		Text:         text,
		SizeBytes:    int64(len(text)),
		Lines:        countTextLines(text),
		FilesChanged: filesChanged,
		Additions:    additions,
		Deletions:    deletions,
	}
	attachment.Summary = formatGoalAttachmentSummary(attachment)
	return attachment
}

func formatGoalAttachmentSummary(attachment goalAttachment) string {
	switch attachment.Type {
	case goalAttachmentGitDiff:
		if attachment.FilesChanged > 0 || attachment.Additions > 0 || attachment.Deletions > 0 {
			return fmt.Sprintf("[git diff: %d files changed, +%d/-%d]", attachment.FilesChanged, attachment.Additions, attachment.Deletions)
		}
		return fmt.Sprintf("[git diff: %s, %d lines]", protocol.FormatFileSize(attachment.SizeBytes), attachment.Lines)
	case goalAttachmentText:
		fallthrough
	default:
		return fmt.Sprintf("[text: %s, %d lines]", protocol.FormatFileSize(attachment.SizeBytes), attachment.Lines)
	}
}

func isLargeGoalPaste(text string) bool {
	return len(text) > goalPasteAttachmentByteThreshold || countTextLines(text) > goalPasteAttachmentLineThreshold
}

func insertedText(before, after string) string {
	if before == after {
		return ""
	}
	prefix := 0
	for prefix < len(before) && prefix < len(after) && before[prefix] == after[prefix] {
		prefix++
	}
	beforeSuffix := len(before)
	afterSuffix := len(after)
	for beforeSuffix > prefix && afterSuffix > prefix && before[beforeSuffix-1] == after[afterSuffix-1] {
		beforeSuffix--
		afterSuffix--
	}
	if prefix > afterSuffix {
		return ""
	}
	return after[prefix:afterSuffix]
}

func (m *Model) startGoalPasteCapture(baseline, text string) tea.Cmd {
	m.goalPasteCapture = true
	m.goalPasteBaseline = baseline
	m.goalPasteBuffer = text
	m.setGoalInputValue(baseline)
	m.resizeGoalEditor()
	return tea.Tick(goalPasteFlushDelay, func(time.Time) tea.Msg { return goalPasteFlushMsg{} })
}

func (m *Model) appendGoalPasteCapture(text string) tea.Cmd {
	m.goalPasteBuffer += text
	return tea.Tick(goalPasteFlushDelay, func(time.Time) tea.Msg { return goalPasteFlushMsg{} })
}

func (m *Model) finishGoalPasteCapture() tea.Cmd {
	if !m.goalPasteCapture {
		return nil
	}
	baseline := m.goalPasteBaseline
	buffer := m.goalPasteBuffer
	m.goalPasteCapture = false
	m.goalPasteBaseline = ""
	m.goalPasteBuffer = ""
	if buffer == "" {
		return nil
	}
	if isLargeGoalPaste(buffer) {
		m.setGoalInputValue(baseline)
		m.resizeGoalEditor()
		m.appendGoalAttachment(newGoalTextAttachment("paste", buffer))
		return textarea.Blink
	}
	m.setGoalInputValue(baseline + buffer)
	m.resizeGoalEditor()
	return nil
}

func assembleGoalSubmission(instruction string, attachments []goalAttachment) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" && len(attachments) == 0 {
		return ""
	}

	parts := make([]string, 0, 1+len(attachments)*2)
	if instruction != "" {
		parts = append(parts, instruction)
	}
	for _, attachment := range attachments {
		if attachment.Text == "" {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, renderGoalAttachment(attachment))
	}
	return strings.Join(parts, "\n")
}

func renderGoalAttachment(attachment goalAttachment) string {
	return strings.Join([]string{
		fmt.Sprintf("Attached %s:", attachment.Type),
		renderGoalAttachmentBlock(attachment),
	}, "\n")
}

func renderGoalAttachmentBlock(attachment goalAttachment) string {
	fence := goalAttachmentFence(attachment.Text)
	lang := attachmentFenceLanguage(attachment.Type)
	if lang != "" {
		return strings.Join([]string{
			fence + lang,
			attachment.Text,
			fence,
		}, "\n")
	}
	return strings.Join([]string{
		fence,
		attachment.Text,
		fence,
	}, "\n")
}

func attachmentFenceLanguage(kind goalAttachmentType) string {
	switch kind {
	case goalAttachmentGitDiff:
		return "diff"
	case goalAttachmentText:
		return "text"
	default:
		return ""
	}
}

func goalAttachmentFence(text string) string {
	maxRun := 0
	run := 0
	for _, r := range text {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		run = 0
	}
	fenceLen := maxRun + 1
	if fenceLen < 3 {
		fenceLen = 3
	}
	return strings.Repeat("`", fenceLen)
}

func removeGoalAttachmentAt(attachments []goalAttachment, index int) []goalAttachment {
	if index < 0 || index >= len(attachments) {
		return append([]goalAttachment(nil), attachments...)
	}
	next := make([]goalAttachment, 0, len(attachments)-1)
	next = append(next, attachments[:index]...)
	next = append(next, attachments[index+1:]...)
	return next
}

func countTextLines(text string) int {
	if text == "" {
		return 0
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}

func (m Model) viewGoalAttachments() string {
	if len(m.goalAttachments) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, renderBold("Attachments:"))
	for i, attachment := range m.goalAttachments {
		lines = append(lines, m.renderGoalAttachmentRow(attachment, m.goalFocus == goalFocusAttachments && i == m.goalAttachmentSelected))
	}
	return strings.Join(lines, "\n")
}

func (m Model) renderGoalAttachmentRow(attachment goalAttachment, selected bool) string {
	row := formatGoalAttachmentRow(attachment)
	row = truncateGoalAttachmentRow(row, m.goalAttachmentRowWidth()-2)
	if selected {
		return renderBold("> " + row)
	}
	return "  " + row
}

func formatGoalAttachmentRow(attachment goalAttachment) string {
	parts := make([]string, 0, 2)
	if attachment.Source != "" && !strings.EqualFold(attachment.Source, string(attachment.Type)) {
		parts = append(parts, attachment.Source)
	}
	if attachment.Summary != "" {
		parts = append(parts, attachment.Summary)
	}
	return strings.Join(parts, " · ")
}

func (m Model) goalAttachmentRowWidth() int {
	width := m.contentWidth()
	if width <= 0 {
		width = defaultDialogWidth
	}
	width -= 4
	if width < 16 {
		return 16
	}
	return width
}

func truncateGoalAttachmentRow(text string, maxWidth int) string {
	if maxWidth <= 0 || runewidth.StringWidth(text) <= maxWidth {
		return text
	}

	const ellipsis = "…"
	ellipsisWidth := runewidth.StringWidth(ellipsis)
	if maxWidth <= ellipsisWidth {
		return ellipsis
	}

	var b strings.Builder
	width := 0
	limit := maxWidth - ellipsisWidth
	for _, r := range text {
		rw := runewidth.RuneWidth(r)
		if width+rw > limit {
			break
		}
		b.WriteRune(r)
		width += rw
	}
	b.WriteString(ellipsis)
	return b.String()
}

func (m *Model) focusGoalEditor() {
	m.goalFocus = goalFocusEditor
	m.goalInput.Focus()
}

func (m *Model) focusGoalAttachments() bool {
	if len(m.goalAttachments) == 0 {
		return false
	}
	m.goalFocus = goalFocusAttachments
	m.goalInput.Blur()
	if m.goalAttachmentSelected < 0 || m.goalAttachmentSelected >= len(m.goalAttachments) {
		m.goalAttachmentSelected = 0
	}
	return true
}

func (m Model) goalInputOnLastLine() bool {
	value := m.goalInput.Value()
	if value == "" {
		return true
	}
	return m.goalInput.Line() >= countEditorLines(value)-1
}

func (m *Model) moveGoalAttachmentSelection(delta int) bool {
	if len(m.goalAttachments) == 0 {
		return false
	}
	if m.goalAttachmentSelected < 0 {
		m.goalAttachmentSelected = 0
	}
	next := m.goalAttachmentSelected + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.goalAttachments) {
		next = len(m.goalAttachments) - 1
	}
	m.goalAttachmentSelected = next
	return true
}

func (m *Model) deleteGoalAttachmentSelection() bool {
	if len(m.goalAttachments) == 0 {
		return false
	}
	index := m.goalAttachmentSelected
	if index < 0 || index >= len(m.goalAttachments) {
		index = 0
	}
	next := removeGoalAttachmentAt(m.goalAttachments, index)
	if len(next) == 0 {
		m.goalAttachments = nil
		m.goalAttachmentSelected = -1
		m.focusGoalEditor()
		return true
	}
	if index >= len(next) {
		index = len(next) - 1
	}
	m.goalAttachments = next
	m.goalAttachmentSelected = index
	m.goalFocus = goalFocusAttachments
	m.goalInput.Blur()
	return true
}
