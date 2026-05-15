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
	"github.com/PVRLabs/aibadger/internal/writer"
)

const maxTextResponsePreviewLines = 12
const compactPasteRenderBytes = 4 * 1024
const homeGoalVisibleBytes = 16 * 1024
const homeGoalVisibleLines = 30
const homeGoalPreviewBytes = 96
const homeGoalPreviewLines = 3
const headerRule = "────────────────────────────────────────────────────────"

type pasteSpec struct {
	title       string
	placeholder string
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(m.headerView())
	b.WriteString("\n\n")

	if !m.status.empty() {
		b.WriteString(renderMessage(m.status))
		b.WriteString("\n\n")
	}
	if m.err != nil {
		b.WriteString(renderMessage(errorMessage(m.err.Error())))
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
	case stateReviewHelp:
		b.WriteString(m.viewReviewHelp())
	case statePromptFileReveal:
		b.WriteString(m.viewPromptFileReveal())
	}

	b.WriteString("\n\n")
	b.WriteString(helpStyle.Render(m.statusLine()))
	return b.String()
}

func (m Model) viewHome() string {
	var lines []string
	lines = append(lines, "Type a goal or paste a diff for review, then press Enter.")
	lines = append(lines, "Commands: /help, /review, /exit")
	lines = append(lines, "")
	lines = append(lines, m.viewGoalInput())
	return strings.Join(lines, "\n")
}

func (m Model) viewGoalInput() string {
	goal := m.goalInput.Value()
	size := len(goal)
	lines := countTextLines(goal)
	if size > homeGoalVisibleBytes || lines > homeGoalVisibleLines {
		return fmt.Sprintf(
			"%s\nPreview:\n%s\n%s",
			helpStyle.Render(fmt.Sprintf("[Pasted %s, %d lines]", protocol.FormatFileSize(int64(size)), lines)),
			helpStyle.Render(compactGoalPreview(goal)),
			helpStyle.Render("Press Enter to submit."),
		)
	}
	return m.goalInput.View()
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

func mascotFrame(text string, face string) string {
	return fmt.Sprintf(" /\\_/\\  %s\n( %s )", text, face)
}

func (m Model) viewOnboarding() string {
	body := strings.Join([]string{
		"First run",
		"",
		mascotFrame("Local-first by default.", "o.o"),
		"",
		"Badger is a local bridge between this codebase and your AI chat.",
		"It scans locally and never sends data over the network.",
		"You choose when prompts are copied, pasted, and written back.",
		"Prompt 1 (Map) contains project structure and your goal.",
		"Prompt 2 (Extract) contains only source selected by FILE/PREFIX/NEAR commands.",
		"Generated file writes are previewed before anything is applied.",
		"",
		"Press Enter to continue.",
	}, "\n")
	return boxStyle.Render(body)
}

func (m Model) headerView() string {
	version := strings.TrimSpace(m.cfg.Version)
	name := brand.Name
	if version != "" {
		name += " " + version
	}
	lines := []string{
		headerRule,
		headerLine(" /\\_/\\", titleStyle.Render(name)),
		headerLine("( o.o )", helpStyle.Render(m.cfg.Subtitle)),
		headerLine(" > ^ <", helpStyle.Render(m.pipelineView())),
		headerRule,
	}
	return strings.Join(lines, "\n")
}

func headerLine(mascot string, text string) string {
	return fmt.Sprintf("%-8s     %s", mascot, text)
}

func (m Model) pipelineView() string {
	stages := []string{"Map", "Extract", "Apply"}
	active := 0
	switch m.state {
	case stateWaitingForExtractions, stateContextWarning, stateContextReady:
		active = 1
	case stateWaitingForCode, stateTextResponse:
		active = 2
	case stateWritePreview, stateWriting:
		active = 2
	case stateManualCopy:
		if m.manualCopyKind == codeContextPromptKind {
			active = 1
		} else {
			active = 0
		}
	case statePromptFileReveal:
		if m.promptFileKind == codeContextPromptKind {
			active = 1
		} else {
			active = 0
		}
	}

	for i := range stages {
		if i < active {
			stages[i] = "✓ " + stages[i]
		} else if i == active {
			stages[i] = renderBold("[" + stages[i] + "]")
		}
	}
	return "Pipeline: " + strings.Join(stages, " → ")
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
		preview, hiddenLines := textPreview(notes, maxTextResponsePreviewLines)
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

	preview, hiddenLines := textPreview(response, maxTextResponsePreviewLines)
	body := preview
	if hiddenLines > 0 {
		body += "\n\n" + helpStyle.Render(fmt.Sprintf("... [%d more lines hidden] ...", hiddenLines))
	}
	body += "\n\nPress Enter to continue."
	return helpStyle.Render("Info: No file updates found. AI provided a textual response.") + "\n\n" + renderBold("AI Analysis / Explanation:") + "\n\n" + boxStyle.Render(body)
}

func (m Model) viewManualCopy() string {
	text := strings.TrimRight(m.manualCopyText, "\n")
	if text == "" {
		text = "(empty payload)"
	}
	body := fmt.Sprintf(
		"%s\n\n--- BEGIN %s ---\n%s\n--- END %s ---",
		renderWarningLine(fmt.Sprintf("Clipboard is unavailable. Manually copy %s from the block below, then press Enter to continue.", m.manualCopyKind)),
		m.manualCopyKind,
		text,
		m.manualCopyKind,
	)
	return boxStyle.Render(body)
}

func (m Model) viewHelp() string {
	body := strings.Join([]string{
		"Commands",
		"",
		"/help          Show this reference.",
		"/review        Show diff review guidance.",
		"/exit          Quit Badger.",
		"",
		"Keys",
		"",
		"Enter          Submit or continue.",
		"Ctrl+U         Clear line.",
		"Ctrl+C         Quit Badger.",
		"",
		"BYOL loop",
		"",
		"1. Enter a goal.",
		"2. Confirm copying Prompt 1: Topology, or use the manual-copy fallback.",
		"3. Paste FILE/PREFIX/NEAR commands from your AI chat and press Enter.",
		"4. Confirm copying Prompt 2: Code Context, or use the manual-copy fallback.",
		"5. Paste the final AI response and press Enter.",
		"6. Review file writes and confirm with y.",
		"",
		"Press Enter to return home.",
	}, "\n")
	if m.cfg.BuildInfo != "" {
		body = fmt.Sprintf("%s\n\n%s", m.cfg.BuildInfo, body)
	}
	return boxStyle.Render(body)
}

func (m Model) viewReviewHelp() string {
	body := strings.Join([]string{
		"Code review with Badger",
		"",
		"Use this when you want an AI chat to review a change before committing.",
		"",
		"Example goal:",
		"Review my current change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing.",
		"",
		"Tip:",
		reviewGitShowTip(),
		"For larger diffs, prefer asking Badger to map the project first and let the AI request the specific files it needs.",
		"",
		"Preview feature:",
		"Badger does not review local changes directly yet. Coming later: reviewing current unstaged or staged git diffs from your project.",
		"",
		"Press Enter to return home.",
	}, "\n")
	return boxStyle.Render(body)
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
	spec := pasteSpecForState(st)
	size := len(m.paste.Value())
	label := fmt.Sprintf("[text %s] paste submits, Enter fallback", protocol.FormatFileSize(int64(size)))
	body := m.paste.View()
	if size >= compactPasteRenderBytes {
		label = fmt.Sprintf("[Pasted %s] submitting automatically when paste is detected", protocol.FormatFileSize(int64(size)))
		body = helpStyle.Render("Large pasted input is hidden from the visible editor.")
	}
	return fmt.Sprintf("%s\n%s\n\n%s", spec.title, helpStyle.Render(label), body)
}

func pasteSpecForState(st state) pasteSpec {
	switch st {
	case stateWaitingForExtractions:
		return pasteSpec{
			title:       "Paste extraction commands from your AI chat.",
			placeholder: "Paste FILE/PREFIX/NEAR commands here. Paste submits automatically; Enter is a fallback.",
		}
	case stateWaitingForCode:
		return pasteSpec{
			title:       "Paste the final AI response.",
			placeholder: "Paste the final AI response here. Paste submits automatically; Enter is a fallback.",
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
