package tui

// This file owns general TUI rendering and small display helpers.

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/brand"
	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

const minTextResponsePreviewLines = 12
const maxTextResponsePreviewLines = 50
const compactPasteRenderBytes = 4 * 1024
const homeGoalVisibleBytes = 16 * 1024
const homeGoalVisibleLines = 30
const homeGoalPreviewBytes = 96
const homeGoalPreviewLines = 3
const goalEditorMinHeight = 2
const goalEditorMaxHeight = 10
const defaultDialogWidth = 78

type slashCommandSuggestion struct {
	command     string
	description string
}

type pasteSpec struct {
	title       string
	placeholder string
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n\n")

	if !m.status.empty() {
		b.WriteString(m.renderMessage(m.status))
		b.WriteString("\n\n")
	}
	if m.err != nil {
		b.WriteString(m.renderMessage(errorMessage(m.err.Error())))
		b.WriteString("\n\n")
	}

	switch m.state {
	case stateOnboarding:
		b.WriteString(m.viewOnboarding())
	case stateHome:
		b.WriteString(m.viewHome())
	case stateScanning:
		b.WriteString(m.viewScanning())
	case stateScanComplete:
		b.WriteString(m.viewScanComplete())
	case stateWaitingForExtractions:
		b.WriteString(m.viewPaste(stateWaitingForExtractions))
	case stateContextWarning:
		b.WriteString(m.viewContextWarning())
	case stateContextReady:
		b.WriteString(m.viewContextReady())
	case stateWaitingForCode:
		b.WriteString(m.viewPaste(stateWaitingForCode))
	case stateTextResponse:
		b.WriteString(m.viewTextResponse())
	case stateWritePreview:
		b.WriteString(m.viewWritePreview())
	case stateWriting:
		b.WriteString("Writing confirmed files to disk...\n")
	case stateManualCopy:
		b.WriteString(m.viewManualCopy())
	case stateHelp:
		b.WriteString(m.viewHelp())
	case statePromptFileReveal:
		b.WriteString(m.viewPromptFileReveal())
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(m.statusLine()))
	return b.String()
}

func (m Model) viewHome() string {
	if m.homeGoalPreviewActive() {
		return m.viewGoalPreview()
	}

	var lines []string
	lines = append(lines, "Type a goal, paste a diff, or use /review or /design, then press Enter.")
	lines = append(lines, "Commands: /help, /review, /design, /exit")
	lines = append(lines, "Tag files with @path/to/file, then press Tab.")
	lines = append(lines, "")
	lines = append(lines, m.viewGoalInput())
	if suggestions := m.viewCompletionSuggestions(); suggestions != "" {
		lines = append(lines, "", suggestions)
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewGoalInput() string {
	if m.homeGoalPreviewActive() {
		return m.viewGoalPreview()
	}

	return m.goalInput.View()
}

func (m Model) homeGoalPreviewActive() bool {
	goal := m.goalInput.Value()
	return len(goal) > homeGoalVisibleBytes || countTextLines(goal) > homeGoalVisibleLines
}

func (m Model) viewGoalPreview() string {
	goal := m.goalInput.Value()
	size := len(goal)
	lines := countTextLines(goal)
	return fmt.Sprintf(
		"%s\n%s\n%s",
		helpStyle.Render(fmt.Sprintf("[Pasted %s, %d lines]", protocol.FormatFileSize(int64(size)), lines)),
		helpStyle.Render(compactGoalPreview(goal)),
		helpStyle.Render("Press Enter to submit."),
	)
}

func (m Model) viewSlashCommandSuggestions() string {
	candidate, ok := m.completionVisible()
	if !ok || candidate.kind != completionKindSlash {
		return ""
	}

	var lines []string
	for _, suggestion := range candidate.suggestions {
		lines = append(lines, fmt.Sprintf("  %-12s %s", suggestion.label, suggestion.description))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func (m Model) viewCompletionSuggestions() string {
	candidate, ok := m.completionVisible()
	if !ok || len(candidate.suggestions) == 0 {
		return ""
	}

	var lines []string
	for _, suggestion := range candidate.suggestions {
		lines = append(lines, fmt.Sprintf("  %-12s %s", suggestion.label, suggestion.description))
	}
	return strings.Join(lines, "\n")
}

func (m Model) slashCommandSuggestions() []slashCommandSuggestion {
	suggestions := []slashCommandSuggestion{
		{command: helpCommand, description: "Show commands and keyboard shortcuts."},
		{command: reviewCommand, description: "Seed an editable review prompt from the current git diff."},
		{command: designCommand, description: "Switch the active focus to Design."},
	}
	if m.cfg.ExitCommand != "" {
		suggestions = append(suggestions, slashCommandSuggestion{
			command:     m.cfg.ExitCommand,
			description: "Quit Badger.",
		})
	}
	return suggestions
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

func compactGoalPreview(text string) string {
	var preview []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) > homeGoalPreviewBytes {
			line = trimStringBytes(line, homeGoalPreviewBytes) + "..."
		}
		preview = append(preview, "  "+line)
		if len(preview) >= homeGoalPreviewLines {
			break
		}
	}
	if len(preview) == 0 {
		return "  (blank)"
	}
	return strings.Join(preview, "\n")
}

func (m Model) viewOnboarding() string {
	symbols := defaultDisplaySymbols()
	body := strings.Join([]string{
		renderBold("First run"),
		"",
		"Badger is a lightweight local bridge between your codebase",
		"and any AI chat (Claude, ChatGPT, DeepSeek, Grok, etc.)",
		"",
		renderBold("How it works:"),
		"",
		fmt.Sprintf("1. %s", renderBold("Map")),
		"   Enter your goal. Badger builds a prompt.",
		fmt.Sprintf("   %s You copy it %s paste into your AI chat", symbols.bentArrow, symbols.arrow),
		"",
		fmt.Sprintf("2. %s", renderBold("Extract")),
		"   AI replies asking for specific files.",
		fmt.Sprintf("   %s You copy that %s paste back into Badger", symbols.bentArrow, symbols.arrow),
		"",
		fmt.Sprintf("3. %s", renderBold("Apply")),
		"   Badger fetches those files, builds a second prompt.",
		fmt.Sprintf("   %s You copy it %s paste into AI %s review before writing", symbols.bentArrow, symbols.arrow, symbols.arrow),
		"",
		fmt.Sprintf("%s Fully local %s nothing leaves your machine until you copy it", symbols.success, symbols.dash),
		fmt.Sprintf("%s You control every paste and every write", symbols.success),
		"",
		renderBold("Press Enter to continue"),
	}, "\n")
	return m.renderBox(body)
}

func (m Model) headerView() string {
	version := strings.TrimSpace(m.cfg.Version)
	lines := []string{
		brand.HeaderRule,
		brand.HeaderLine(" /\\_/\\", titleStyle.Render(brand.VersionedName(version))),
		brand.HeaderLine("( o.o )", helpStyle.Render(m.cfg.Subtitle)),
		brand.HeaderLine(" > ^ <", helpStyle.Render(m.pipelineView())),
		brand.HeaderRule,
	}
	return strings.Join(lines, "\n")
}

func (m Model) pipelineView() string {
	symbols := defaultDisplaySymbols()
	stages := []string{"Map", "Extract", workflow.PipelineFinalLabel(m.cfg.Focus)}
	active := 0
	switch m.state {
	case stateWaitingForExtractions, stateContextWarning, stateContextReady:
		active = 1
	case stateWaitingForCode, stateTextResponse:
		active = 2
	case stateWritePreview, stateWriting:
		active = 2
	case stateManualCopy:
		if m.manualCopyKind == codeContextPromptKind || m.manualCopyKind == workflow.PromptTwoKind(m.cfg.Focus) {
			active = 1
		} else {
			active = 0
		}
	case statePromptFileReveal:
		if m.promptFileKind == codeContextPromptKind || m.promptFileKind == workflow.PromptTwoKind(m.cfg.Focus) {
			active = 1
		} else {
			active = 0
		}
	}

	for i := range stages {
		if i < active {
			stages[i] = symbols.success + " " + stages[i]
		} else if i == active {
			stages[i] = renderBold("[" + stages[i] + "]")
		}
	}
	return "Pipeline: " + strings.Join(stages, symbols.pipelineSep)
}

func (m Model) viewScanning() string {
	return fmt.Sprintf("Scanning project after goal submission.\n\n%s", m.cfg.ScanFrames[m.scanFrame%len(m.cfg.ScanFrames)])
}

func (m Model) viewWritePreview() string {
	var lines []string
	for _, update := range m.updates {
		kind := "write"
		if update.Kind == writer.UpdateKindDelete {
			kind = "delete"
		}
		lines = append(lines, fmt.Sprintf("  [%s] %s", kind, update.Path))
	}

	body := renderWarningLine("About to apply changes to disk:") + "\n\n" + strings.Join(lines, "\n")
	if notes := strings.TrimSpace(m.response); notes != "" {
		preview, hiddenLines := textPreview(notes, m.textResponsePreviewLineLimit())
		body += "\n\n" + renderWarningLine("AI notes included with this response:") + "\n\n" + preview
		if hiddenLines > 0 {
			body += "\n\n" + helpStyle.Render(fmt.Sprintf("... [%d more lines hidden] ...", hiddenLines))
		}
	}
	return body + "\n\n" + renderBold("Apply these changes? (y/N)")
}

func (m Model) viewTextResponse() string {
	response := strings.TrimSpace(m.response)
	if response == "" {
		response = "(empty response)"
	}

	preview, hiddenLines := textPreview(response, m.textResponsePreviewLineLimit())
	body := preview
	if hiddenLines > 0 {
		body += "\n\n" + helpStyle.Render(fmt.Sprintf("... [%d more lines hidden] ...", hiddenLines))
	}
	body += "\n\nPress Enter to continue."
	return helpStyle.Render("Info: No file updates found. AI provided a textual response.") + "\n\n" + renderBold("AI Analysis / Explanation:") + "\n\n" + m.renderBox(body)
}

func (m Model) viewManualCopy() string {
	text := strings.TrimRight(m.manualCopyText, "\n")
	if text == "" {
		text = "(empty payload)"
	}
	body := fmt.Sprintf(
		"%s\n\n--- BEGIN %s ---\n%s\n--- END %s ---",
		fmt.Sprintf("Manually copy %s from the block below, then press Enter to continue.", m.manualCopyKind),
		m.manualCopyKind,
		text,
		m.manualCopyKind,
	)
	return m.renderBox(body)
}

func (m Model) viewHelp() string {
	body := strings.Join([]string{
		"Commands",
		"",
		"/help          Show this reference.",
		"/review        Seed an editable review prompt from the current git diff.",
		"/design        Switch the active focus to Design.",
		"/exit          Quit Badger.",
		"",
		"Keys",
		"",
		"Enter          Submit or continue.",
		"Tab            Complete / commands and @ files.",
		"Ctrl+U         Clear line.",
		"Ctrl+C         Quit Badger.",
		"",
		"BYOL loop",
		"",
		"1. Enter a goal. Use @path/to/file when you want Prompt 1 to include a specific file.",
		"2. Confirm copying Prompt 1: Topology, or use the manual-copy fallback.",
		"3. Paste FILE/PREFIX/NEAR commands from your AI chat and press Enter.",
		fmt.Sprintf("4. Confirm copying %s, or use the manual-copy fallback.", workflow.PromptTwoKind(m.cfg.Focus)),
		"5. Paste the final AI response and press Enter.",
		"6. Review file writes and confirm with y.",
		"",
		"Press Enter to return home.",
	}, "\n")
	if m.cfg.BuildInfo != "" {
		body = fmt.Sprintf("%s\n\n%s", m.cfg.BuildInfo, body)
	}
	return m.renderBox(body)
}

func (m Model) renderBox(body string) string {
	width := m.dialogWidth()
	if width <= 0 {
		return boxStyle.Render(body)
	}
	return boxStyle.Width(width - 2).Render(body)
}

func (m Model) renderMessage(msg tuiMessage) string {
	return renderMessageWithWidth(msg, m.contentWidth())
}

func (m Model) contentWidth() int {
	if m.width <= 0 {
		return 0
	}
	return m.width
}

func (m Model) dialogWidth() int {
	if m.width <= 0 {
		return defaultDialogWidth
	}
	if m.width <= 4 {
		return 0
	}
	if m.width < 22 {
		return m.width
	}
	return m.width - 2
}

func (m Model) textResponsePreviewLineLimit() int {
	if m.height <= 0 {
		return minTextResponsePreviewLines
	}

	const reservedRows = 15
	available := m.height - reservedRows
	return clamp(available, minTextResponsePreviewLines, maxTextResponsePreviewLines)
}

func reviewGitShowTip() string {
	return formatReviewGitShowTip(clipboard.PipeCommand())
}

func formatReviewGitShowTip(pipeCommand string, ok bool) string {
	if ok {
		return fmt.Sprintf("To review the latest commit, run `git show | %s`, then paste it with your review goal.", pipeCommand)
	}
	return "To review the latest commit, run `git show`, copy its output, and paste it with your review goal."
}

func (m Model) viewPaste(st state) string {
	spec := pasteSpecForState(st, m.cfg.Focus)
	size := len(m.paste.Value())
	label := fmt.Sprintf("[text %s] paste submits, Enter fallback", protocol.FormatFileSize(int64(size)))
	body := m.paste.View()
	if size >= compactPasteRenderBytes {
		label = fmt.Sprintf("[Pasted %s] submitting automatically when paste is detected", protocol.FormatFileSize(int64(size)))
		body = helpStyle.Render("Large pasted input is hidden from the visible editor.")
	}
	return fmt.Sprintf("%s\n%s\n\n%s", spec.title, helpStyle.Render(label), body)
}

func pasteSpecForState(st state, focus protocol.Focus) pasteSpec {
	switch st {
	case stateWaitingForExtractions:
		return pasteSpec{
			title:       "Paste extraction commands from your AI chat.",
			placeholder: "Paste FILE/PREFIX/NEAR commands here. Paste submits automatically; Enter is a fallback.",
		}
	case stateWaitingForCode:
		switch protocol.NormalizeFocus(focus) {
		case protocol.FocusReview:
			return pasteSpec{
				title:       "Continue the review in your AI chat.\nIf you want Badger to inspect suggested file changes, paste the AI response here.",
				placeholder: "Optional: paste the AI response here.",
			}
		case protocol.FocusDesign:
			return pasteSpec{
				title:       "Continue the design discussion in your AI chat.\nPaste the AI response here only if you want Badger to display or inspect it.",
				placeholder: "Optional: paste the AI response here.",
			}
		default:
			return pasteSpec{
				title:       "Paste the final AI response.",
				placeholder: "Paste the final AI response here. Paste submits automatically; Enter is a fallback.",
			}
		}
	default:
		return pasteSpec{}
	}
}

func textPreview(text string, maxLines int) (string, int) {
	if maxLines <= 0 {
		return text, 0
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text, 0
	}
	return strings.Join(lines[:maxLines], "\n"), len(lines) - maxLines
}

func renderSummary(t *model.ProjectTopology) string {
	var b strings.Builder
	b.WriteString("Scan complete! Here's what Badger found:\n\n")
	b.WriteString("─────────────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf("%s %s\n", renderLabel("Project:"), displayProjectName(t)))
	if len(t.Languages) > 0 {
		b.WriteString(fmt.Sprintf("%s %s\n", renderLabel("Languages:"), strings.Join(t.Languages, ", ")))
	}
	if len(t.Languages) > 1 && t.PrimaryLanguage != "" && t.PrimaryLanguage != "Unknown" {
		b.WriteString(fmt.Sprintf("Primary: %s\n", t.PrimaryLanguage))
	}
	if len(t.Stack) > 0 {
		b.WriteString(fmt.Sprintf("%s %s\n", renderLabel("Stack:"), strings.Join(t.Stack, ", ")))
	}
	if t.Structure != "" && t.Structure != "Unknown" {
		b.WriteString(fmt.Sprintf("%s %s\n", renderLabel("Structure:"), t.Structure))
	}
	b.WriteString("\n")
	b.WriteString(renderLabel("Main Modules:") + "\n")

	limit := len(t.Modules)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		mod := t.Modules[i]
		b.WriteString(fmt.Sprintf("  - %s (%d files)", displayModuleName(mod), mod.FileCount))
		if mod.Heaviest.Name != "" {
			b.WriteString(fmt.Sprintf(" -> Top: %s (%s)", mod.Heaviest.Name, protocol.FormatFileSize(mod.Heaviest.Size)))
		}
		b.WriteString("\n")
	}
	if len(t.Modules) > limit {
		b.WriteString(fmt.Sprintf("  - ... %d more module(s)\n", len(t.Modules)-limit))
	}
	b.WriteString(fmt.Sprintf("\n%s %d source files across %d modules\n", renderLabel("Total:"), totalFiles(t), len(t.Modules)))
	b.WriteString("─────────────────────────────────────────────────")
	return b.String()
}

func displayProjectName(t *model.ProjectTopology) string {
	if t.Name != "" {
		return t.Name
	}
	if t.ProjectRoot == "" {
		return "unknown"
	}
	return filepath.Base(t.ProjectRoot)
}

func displayModuleName(m model.Module) string {
	if m.Name != "" {
		return m.Name
	}
	if m.Path != "" {
		return m.Path
	}
	return "."
}

func totalFiles(t *model.ProjectTopology) int {
	if t == nil {
		return 0
	}
	total := 0
	for _, mod := range t.Modules {
		total += mod.FileCount
	}
	return total
}

func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func goalEditorHeight(text string, terminalHeight int) int {
	contentLines := countTextLines(text)
	if contentLines < goalEditorMinHeight {
		contentLines = goalEditorMinHeight
	}
	return clamp(contentLines, goalEditorMinHeight, goalEditorMaxHeight)
}
