package tui

// This file owns rendering for prompt delivery choices and large prompt
// handling screens.

import (
	"fmt"
	"strings"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func (m Model) viewScanComplete() string {
	if m.largeProjectPending {
		return fmt.Sprintf(
			"%s\n\n%s\n\n%s",
			renderSummary(m.eng.Topology),
			renderWarningLine(fmt.Sprintf("Large project detected: %d files.", totalFiles(m.eng.Topology))),
			m.viewLargeProjectDelivery(),
		)
	}

	if m.promptDeliveryIsLarge(topologyPromptKind) {
		return fmt.Sprintf("%s\n\n%s", renderSummary(m.eng.Topology), m.viewLargePromptDelivery(topologyPromptKind, m.schemaA))
	}

	note := fmt.Sprintf(
		"Ready to copy %s to your clipboard.\n\n%s\nYou will pass this prompt to an AI chat.\n\n%s",
		renderBold("Prompt 1: Topology"),
		"Privacy: Structure only - no source code.",
		renderBold(fmt.Sprintf("Copy Prompt 1: Topology to clipboard (payload: %s)? (y/N)", protocol.FormatFileSize(int64(len(m.schemaA))))),
	)
	return fmt.Sprintf("%s\n\n%s", renderSummary(m.eng.Topology), note)
}

func (m Model) viewContextReady() string {
	var lines []string
	hasTruncation := false
	for _, meta := range m.metadata {
		status := ""
		if meta.Dropped {
			status = " [DROPPED - EXCEEDS TOTAL LIMIT]"
			hasTruncation = true
		} else if meta.Truncated {
			status = " [TRUNCATED]"
			hasTruncation = true
		}
		lines = append(lines, fmt.Sprintf("  - %s%s", meta.Path, status))
	}
	if len(lines) == 0 {
		lines = append(lines, "  - no files listed")
	}

	warning := ""
	if hasTruncation {
		warning = "\n" + renderWarningLine("Note: Some files were truncated or dropped to fit context limits.") + "\n"
	}

	if m.promptDeliveryIsLarge(codeContextPromptKind) {
		return fmt.Sprintf(
			"%s\n%s\n%s",
			renderWarningLine("This WILL include the actual source code from:"),
			strings.Join(lines, "\n"),
			warning,
		) + "\n" + m.viewLargePromptDelivery(codeContextPromptKind, m.schemaB)
	}

	note := fmt.Sprintf(
		"Ready to copy %s to your clipboard.\n\n%s\n%s\n%s\n%s",
		renderBold("Prompt 2: Code Context"),
		renderWarningLine("This WILL include the actual source code from:"),
		strings.Join(lines, "\n"),
		warning,
		renderBold(fmt.Sprintf("Copy Prompt 2: Code Context to clipboard (payload: %s)? (y/N)", protocol.FormatFileSize(int64(len(m.schemaB))))),
	)
	return note
}

func (m Model) viewContextWarning() string {
	var failureLines []string
	for _, failure := range m.pendingFailedCommands {
		failureLines = append(failureLines, "  - "+failure)
	}
	var exclusionLines []string
	for _, exclusion := range m.pendingSafetyExclusions {
		exclusionLines = append(exclusionLines, "  - "+exclusion)
	}

	extractedLabel := "file"
	if m.pendingExtractedCount != 1 {
		extractedLabel = "files"
	}
	issueCount := len(m.pendingFailedCommands) + len(m.pendingSafetyExclusions)
	issueLabel := "request"
	if issueCount != 1 {
		issueLabel = "requests"
	}
	verb := "needs"
	if issueCount != 1 {
		verb = "need"
	}
	summary := fmt.Sprintf("Extracted %d %s, but %d %s %s attention:", m.pendingExtractedCount, extractedLabel, issueCount, issueLabel, verb)

	sections := []string{renderWarningLine(summary), ""}
	if len(failureLines) > 0 {
		sections = append(sections, renderBold("Failed:"), strings.Join(failureLines, "\n"), "")
	}
	if len(exclusionLines) > 0 {
		sections = append(sections, renderBold("Excluded by Prompt 2 safety rules:"), strings.Join(exclusionLines, "\n"), "")
	}
	sections = append(sections, renderBold("Proceed with available context? (y/N)"))

	return strings.Join(sections, "\n")
}

func (m Model) promptDeliveryText(kind string) string {
	switch kind {
	case topologyPromptKind:
		return m.schemaA
	case codeContextPromptKind:
		return m.schemaB
	default:
		return ""
	}
}

func (m Model) promptDeliveryIsLarge(kind string) bool {
	return isLargePrompt(m.promptDeliveryText(kind), m.cfg.LargePromptByteThreshold)
}

func (m Model) viewLargePromptDelivery(kind, text string) string {
	return strings.Join([]string{
		renderWarningLine(fmt.Sprintf("%s is large (%s).", kind, protocol.FormatFileSize(int64(len(text))))),
		"",
		"Some AI chats may reject or truncate large pasted text.",
		"Recommended: save it to a temp file and attach/upload it to your AI chat.",
		"",
		renderLabel("Options:"),
		"  [c] Copy anyway",
		"  [f] Save to temp file",
		"  [p] Print to terminal",
		"  [n] Cancel",
		"",
		renderBold("Choice (recommended: f):"),
	}, "\n")
}

func (m Model) viewLargeProjectDelivery() string {
	return strings.Join([]string{
		renderLabel("Options:"),
		"  [c] Continue",
		fmt.Sprintf("  [t] Truncate Prompt 1: Topology to %d packages", m.cfg.TruncatedMaxPackages),
		"  [e] Exit to home",
		"",
		renderBold("Choice (recommended: t):"),
	}, "\n")
}

func (m Model) viewPromptFileReveal() string {
	return fmt.Sprintf("Open containing folder? (y/N)\n\n%s", helpStyle.Render("The saved path above stays visible if opening fails or is skipped."))
}
