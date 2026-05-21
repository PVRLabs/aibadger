package tui

// This file owns keyboard routing, state-specific key actions, and the
// keyboard hint status line.

import (
	"strings"

	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type statusLineMode int

const (
	statusLineKeyboardHints statusLineMode = iota
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.handleCompletionKey(msg.String()); handled {
		return next, cmd
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		return m.handleKeyEsc()

	case "enter":
		if next, cmd, handled := m.handleKeyEnter(); handled {
			return next, cmd
		}

	case "tab":
		if next, cmd, handled := m.handleKeyTab(); handled {
			return next, cmd
		}

	case "y", "Y":
		if next, cmd, handled := m.handleKeyConfirm(); handled {
			return next, cmd
		}

	case "n", "N":
		if next, cmd, handled := m.handleKeyCancel(); handled {
			return next, cmd
		}

	case "c", "C":
		if next, cmd, handled := m.handleKeyCopy(); handled {
			return next, cmd
		}

	case "f", "F":
		if next, cmd, handled := m.handleKeySaveToFile(); handled {
			return next, cmd
		}

	case "p", "P":
		if next, cmd, handled := m.handleKeyPrintToTerminal(); handled {
			return next, cmd
		}

	case "t", "T":
		if next, cmd, handled := m.handleKeyTruncate(); handled {
			return next, cmd
		}

	case "e", "E":
		if next, cmd, handled := m.handleKeyExitToHome(); handled {
			return next, cmd
		}
	}

	// Forward unhandled keys to the active input widget.
	return m.forwardKeyToInput(msg)
}

// handleKeyEsc cancels the current operation and returns to the home screen,
// except during states where interruption is not safe (scanning, writing).
func (m Model) handleKeyEsc() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateHome, stateScanning, stateWriting:
		// These states are either already home or mid-operation; esc is a no-op.
		return m, nil
	default:
		m.state = stateHome
		m.status = neutralMessage("Cancelled. Ready for a new goal.")
		m.err = nil
		m.goalInput.Focus()
		m.paste.Blur()
		return m, textarea.Blink
	}
}

// handleKeyEnter advances the workflow from whatever the current state is.
// Each state has exactly one action on Enter: submit, advance, or dismiss.
func (m Model) handleKeyEnter() (tea.Model, tea.Cmd, bool) {
	switch m.state {
	case stateOnboarding:
		// Dismiss the first-run onboarding screen and go to the goal input.
		m.state = stateHome
		m.goalInput.Focus()
		return m, textarea.Blink, true

	case stateHome:
		next, cmd := m.submitGoal()
		return next, cmd, true

	case stateTextResponse:
		// The AI returned text only (no file writes); acknowledge and reset.
		next, cmd := m.returnHome(neutralMessage("Ready for the next goal."))
		return next, cmd, true

	case stateManualCopy:
		// User has manually copied the prompt shown in the terminal; advance.
		next, cmd := m.advanceAfterCopy(m.manualCopyKind, true)
		return next, cmd, true

	case statePromptFileReveal:
		// User has seen the saved-file path; advance without opening a folder.
		next, cmd := m.advanceAfterTempFile(m.promptFileKind, m.promptFilePath)
		return next, cmd, true

	case stateHelp:
		next, cmd := m.returnHome(neutralMessage("Ready for a goal."))
		return next, cmd, true

	case stateReviewHelp:
		next, cmd := m.returnHome(neutralMessage("Ready for a goal."))
		return next, cmd, true

	case stateWaitingForExtractions, stateWaitingForCode:
		// Enter is a fallback submit for the paste widget (paste events submit
		// automatically; see forwardKeyToInput below).
		next, cmd := m.submitPasteState()
		return next, cmd, true

	case stateContextWarning:
		next, cmd := m.rejectPartialExtractionWarning()
		return next, cmd, true
	}
	return m, nil, false // key not applicable in current state
}

// handleKeyTab completes the home slash-command input to the first visible
// suggestion. Other states keep their existing input handling.
func (m Model) handleKeyTab() (tea.Model, tea.Cmd, bool) {
	if m.state != stateHome {
		return m, nil, false
	}
	candidate, ok := m.completionVisible()
	if !ok || candidate.kind != completionKindSlash {
		return m, nil, false
	}
	next, cmd := m.applyCompletionCandidate(candidate)
	return next, cmd, true
}

// handleKeyConfirm handles the "y/Y" key, which confirms actions on screens
// that present a yes/no prompt.
func (m Model) handleKeyConfirm() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete {
		if m.largeProjectPending || m.promptDeliveryIsLarge(topologyPromptKind) {
			return m, nil, true
		}
		return m, copyCmd(topologyPromptKind, m.schemaA), true
	}
	if m.state == stateContextReady {
		if m.promptDeliveryIsLarge(codeContextPromptKind) {
			return m, nil, true
		}
		return m, copyCmd(codeContextPromptKind, m.schemaB), true
	}
	if m.state == stateContextWarning {
		next, cmd := m.acceptPartialExtractionWarning()
		return next, cmd, true
	}
	if m.state == stateWritePreview {
		m.state = stateWriting
		return m, writeCmd(m.workflowSession(), m.updates), true
	}
	if m.state == statePromptFileReveal {
		return m, openPromptFileCmd(m.promptFileKind, m.promptFilePath), true
	}
	return m, nil, false
}

// handleKeyCancel handles the "n/N" key, which declines the current prompt
// and moves on without copying/writing.
func (m Model) handleKeyCancel() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete {
		if m.largeProjectPending {
			return m, nil, true
		}
		next, cmd := m.cancelPromptDelivery(topologyPromptKind)
		return next, cmd, true
	}
	if m.state == stateContextReady {
		next, cmd := m.cancelPromptDelivery(codeContextPromptKind)
		return next, cmd, true
	}
	if m.state == stateContextWarning {
		next, cmd := m.rejectPartialExtractionWarning()
		return next, cmd, true
	}
	if m.state == stateWritePreview {
		m.state = stateHome
		m.status = neutralMessage("Write cancelled. Ready for a new goal.")
		m.setGoalInputValue("")
		m.completion.suppressedKey = ""
		m.resizeGoalEditor()
		m.goalInput.Focus()
		return m, textarea.Blink, true
	}
	if m.state == statePromptFileReveal {
		next, cmd := m.advanceAfterTempFile(m.promptFileKind, m.promptFilePath)
		return next, cmd, true
	}
	return m, nil, false
}

// handleKeyCopy handles the "c/C" key.
//
// In the large-project menu it resumes generation at full size.
// On scan-complete and context-ready screens it copies the prompt regardless
// of size (unlike y/Y, which is blocked when the prompt is large).
func (m Model) handleKeyCopy() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && m.largeProjectPending {
		m.largeProjectPending = false
		m, warnings := m.refreshTopologyPrompt()
		if warning := taggedFileWarningMessage(warnings); !warning.empty() {
			m.status = warning
		}
		return m, nil, true
	}

	if m.state == stateScanComplete {
		return m, copyCmd(topologyPromptKind, m.schemaA), true
	}
	if m.state == stateContextReady {
		return m, copyCmd(codeContextPromptKind, m.schemaB), true
	}
	return m, nil, false
}

// handleKeySaveToFile handles the "f/F" key, which is only active when the
// prompt is large and the UI is offering alternative delivery options.
func (m Model) handleKeySaveToFile() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && m.promptDeliveryIsLarge(topologyPromptKind) {
		return m, savePromptCmd(topologyPromptKind, m.schemaA), true
	}
	if m.state == stateContextReady && m.promptDeliveryIsLarge(codeContextPromptKind) {
		return m, savePromptCmd(codeContextPromptKind, m.schemaB), true
	}
	return m, nil, false
}

// handleKeyPrintToTerminal handles the "p/P" key, which is only active when
// the prompt is large. It transitions to stateManualCopy so the full prompt
// text is rendered in the terminal for the user to copy manually.
func (m Model) handleKeyPrintToTerminal() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && m.promptDeliveryIsLarge(topologyPromptKind) {
		m.state = stateManualCopy
		m.manualCopyKind = topologyPromptKind
		m.manualCopyText = m.schemaA
		m.status = neutralMessage("Prompt 1: Topology printed to terminal.")
		return m, nil, true
	}
	if m.state == stateContextReady && m.promptDeliveryIsLarge(codeContextPromptKind) {
		m.state = stateManualCopy
		m.manualCopyKind = codeContextPromptKind
		m.manualCopyText = m.schemaB
		m.status = neutralMessage("Prompt 2: Code Context printed to terminal.")
		return m, nil, true
	}
	return m, nil, false
}

// handleKeyTruncate handles the "t/T" key, which is only active in the
// large-project menu. It regenerates the topology prompt with the configured
// package cap so it fits comfortably in an AI chat.
func (m Model) handleKeyTruncate() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && m.largeProjectPending {
		m.largeProjectPending = false
		// Truncation mode is enforced via the formatter package limit.
		workflow.ConfigureEngine(m.eng, m.engineOptions(m.cfg.TruncatedMaxPackages))
		m, warnings := m.refreshTopologyPrompt()
		if warning := taggedFileWarningMessage(warnings); !warning.empty() {
			m.status = warning
		}
		return m, nil, true
	}
	return m, nil, false
}

// handleKeyExitToHome handles the "e/E" key, which is only active in the
// large-project menu. It discards the scan and returns to the home screen,
// prompting the user to try a smaller subproject root.
func (m Model) handleKeyExitToHome() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && m.largeProjectPending {
		m.state = stateHome
		m.status = neutralMessage("Scan discarded. Try running Badger from a smaller subproject.")
		m.setGoalInputValue("")
		m.completion.suppressedKey = ""
		m.resizeGoalEditor()
		m.goalInput.Focus()
		return m, textarea.Blink, true
	}
	return m, nil, false
}

// forwardKeyToInput passes an unhandled key to whichever input widget is
// currently active. For paste widgets, a detected paste event also triggers
// an immediate submit so the user does not have to press Enter separately.
func (m Model) forwardKeyToInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.state {
	case stateHome:
		m.goalInput, cmd = m.goalInput.Update(msg)
		m.enforceGoalByteLimit(pastedKeyMsg(msg))
		m.resizeGoalEditor()
		m.pruneCompletionSuppression()
	case stateWaitingForExtractions, stateWaitingForCode:
		m.paste, cmd = m.paste.Update(msg)
		if pastedKeyMsg(msg) {
			// Auto-submit on paste so the user does not have to press Enter.
			return m.submitPasteState()
		}
	}
	return m, cmd
}

func (m Model) statusLine() string {
	mode := statusLineKeyboardHints
	switch mode {
	case statusLineKeyboardHints:
		return strings.Join(keyboardHintsForState(m.state), " · ")
	default:
		return ""
	}
}

func keyboardHintsForState(st state) []string {
	hints := []string{"Ctrl+C quit"}
	switch st {
	case stateHome:
		hints = []string{"Enter submit", "Ctrl+U clear line", "Ctrl+C quit"}
	case stateWaitingForExtractions, stateWaitingForCode:
		hints = []string{"Enter submit", "Ctrl+U clear line", "Esc cancel", "Ctrl+C quit"}
	case stateContextWarning:
		hints = []string{"Enter return", "Y proceed", "N return", "Esc cancel", "Ctrl+C quit"}
	case stateScanning, stateWriting:
		// Ctrl+C only.
	default:
		hints = append(hints, "Esc cancel")
	}
	return hints
}
