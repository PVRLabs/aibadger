package tui

// This file owns keyboard routing, state-specific key actions, and the
// keyboard hint status line.

import (
	"fmt"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type statusLineMode int

const (
	statusLineKeyboardHints statusLineMode = iota
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var preKeyCmd tea.Cmd
	if m.state == stateHome && m.goalFocus == goalFocusEditor && goalEditorKeyFinalizesPasteCapture(msg) {
		preKeyCmd = m.finishGoalPasteCapture()
		if goalEditorKeyBypassesAppRouting(msg) {
			next, inputCmd := m.forwardKeyToInput(msg)
			return next, tea.Batch(preKeyCmd, inputCmd)
		}
	}

	if m.goalFocus == goalFocusEditor {
		if next, cmd, handled := m.handleCompletionKey(msg.String()); handled {
			return next, tea.Batch(preKeyCmd, cmd)
		}
	}

	if m.state == stateHome && msg.Type == tea.KeyEnter && msg.Alt {
		m.goalInput.InsertRune('\n')
		m.enforceGoalByteLimit(false)
		m.resizeGoalEditor()
		m.refreshCompletionCandidate()
		return m, textarea.Blink
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit

	case "esc":
		return m.handleKeyEsc()

	case "ctrl+r":
		if m.state == stateHome && protocol.NormalizeFocus(m.cfg.Focus) == protocol.FocusReview {
			return m.handleReviewRefresh()
		}

	case "enter":
		if next, cmd, handled := m.handleKeyEnter(); handled {
			return next, cmd
		}

	case "tab":
		if next, cmd, handled := m.handleKeyTab(); handled {
			return next, cmd
		}

	case "up":
		if next, cmd, handled := m.handleKeyUp(); handled {
			return next, tea.Batch(preKeyCmd, cmd)
		}

	case "down":
		if next, cmd, handled := m.handleKeyDown(); handled {
			return next, tea.Batch(preKeyCmd, cmd)
		}

	case "backspace":
		if next, cmd, handled := m.handleKeyBackspace(); handled {
			return next, cmd
		}

	case "delete":
		if next, cmd, handled := m.handleKeyDelete(); handled {
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

	case "d", "D":
		if next, cmd, handled := m.handleKeySaveToDownloads(); handled {
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
	next, cmd := m.forwardKeyToInput(msg)
	return next, tea.Batch(preKeyCmd, cmd)
}

// handleKeyEsc cancels the current operation and returns to the home screen,
// except during states where interruption is not safe (scanning, writing).
func (m Model) handleKeyEsc() (tea.Model, tea.Cmd) {
	if m.state == stateHome && m.goalFocus == goalFocusAttachments {
		m.focusGoalEditor()
		return m, textarea.Blink
	}
	switch m.state {
	case stateHome, stateScanning, stateWriting, statePromptFileSaving:
		// These states are either already home or mid-operation; esc is a no-op.
		return m, nil
	default:
		m.state = stateHome
		m.status = neutralMessage("Cancelled. Ready for a new goal.")
		m.err = nil
		m.focusGoalEditor()
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
		m.focusGoalEditor()
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
		next, cmd := m.advanceAfterSavedFile(m.promptFileKind, m.promptFilePath, m.promptFileDestination)
		return next, cmd, true

	case stateHelp:
		next, cmd := m.returnHome(neutralMessage("Ready for a goal."))
		return next, cmd, true

	case stateBadge:
		next, cmd := m.returnHome(neutralMessage("Ready for a goal."))
		return next, cmd, true

	case stateWaitingForExtractions, stateWaitingForCode:
		// Enter is a fallback submit for the paste widget (paste events submit
		// automatically; see forwardKeyToInput below).
		next, cmd := m.submitPasteState()
		return next, cmd, true

	case stateScanComplete:
		if m.largeProjectPending {
			return m, nil, true
		}
		return m, copyCmd(topologyPromptKind, m.schemaA), true

	case stateContextReady:
		return m, copyCmd(codeContextPromptKind, m.schemaB), true

	case stateContextWarning:
		next, cmd := m.rejectPartialExtractionWarning()
		return next, cmd, true
	}
	return m, nil, false // key not applicable in current state
}

// handleKeyTab toggles between the goal editor and attachment list when the
// home screen is showing attachments and completion is not active.
func (m Model) handleKeyTab() (tea.Model, tea.Cmd, bool) {
	if m.state == stateHome {
		if m.goalFocus == goalFocusAttachments {
			m.focusGoalEditor()
			return m, textarea.Blink, true
		}
		if len(m.goalAttachments) > 0 {
			if m.focusGoalAttachments() {
				return m, textarea.Blink, true
			}
		}
	}
	return m, nil, false
}

func (m Model) handleKeyUp() (tea.Model, tea.Cmd, bool) {
	if m.state != stateHome {
		return m, nil, false
	}
	if m.goalFocus == goalFocusAttachments {
		if m.goalAttachmentSelected <= 0 {
			m.focusGoalEditor()
			return m, textarea.Blink, true
		}
		m.moveGoalAttachmentSelection(-1)
		return m, textarea.Blink, true
	}
	return m, nil, false
}

func (m Model) handleKeyDown() (tea.Model, tea.Cmd, bool) {
	if m.state != stateHome {
		return m, nil, false
	}
	if m.goalFocus == goalFocusAttachments {
		m.moveGoalAttachmentSelection(1)
		return m, textarea.Blink, true
	}
	if len(m.goalAttachments) > 0 && m.goalInputOnLastLine() {
		if m.focusGoalAttachments() {
			return m, textarea.Blink, true
		}
	}
	return m, nil, false
}

func (m Model) handleKeyBackspace() (tea.Model, tea.Cmd, bool) {
	if m.state != stateHome || m.goalFocus != goalFocusAttachments {
		return m, nil, false
	}
	return m.handleGoalAttachmentDelete()
}

func (m Model) handleKeyDelete() (tea.Model, tea.Cmd, bool) {
	if m.state != stateHome || m.goalFocus != goalFocusAttachments {
		return m, nil, false
	}
	return m.handleGoalAttachmentDelete()
}

func (m Model) handleGoalAttachmentDelete() (tea.Model, tea.Cmd, bool) {
	if !m.deleteGoalAttachmentSelection() {
		return m, nil, false
	}
	if len(m.goalAttachments) == 0 {
		m.status = neutralMessage("Removed the last attachment.")
	} else {
		m.status = neutralMessage(fmt.Sprintf("Removed attachment %d of %d.", m.goalAttachmentSelected+1, len(m.goalAttachments)))
	}
	return m, textarea.Blink, true
}

// handleKeyConfirm handles the "y/Y" key, which confirms actions on screens
// that present a yes/no prompt.
func (m Model) handleKeyConfirm() (tea.Model, tea.Cmd, bool) {
	promptTwoKind := codeContextPromptKind
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
		return m, copyCmd(promptTwoKind, m.schemaB), true
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
		return m, openPromptFileCmd(m.promptFileKind, m.promptFilePath, m.promptFileDestination), true
	}
	return m, nil, false
}

// handleKeyCancel handles the "n/N" key, which declines the current prompt
// and moves on without copying/writing.
func (m Model) handleKeyCancel() (tea.Model, tea.Cmd, bool) {
	promptTwoKind := codeContextPromptKind
	if m.state == stateScanComplete {
		if m.largeProjectPending {
			return m, nil, true
		}
		next, cmd := m.cancelPromptDelivery(topologyPromptKind)
		return next, cmd, true
	}
	if m.state == stateContextReady {
		next, cmd := m.cancelPromptDelivery(promptTwoKind)
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
		m.setGoalAttachments(nil)
		m.completion.suppressedKey = ""
		m.resizeGoalEditor()
		m.goalInput.Focus()
		return m, textarea.Blink, true
	}
	if m.state == statePromptFileReveal {
		next, cmd := m.advanceAfterSavedFile(m.promptFileKind, m.promptFilePath, m.promptFileDestination)
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

func (m Model) handleKeySaveToDownloads() (tea.Model, tea.Cmd, bool) {
	if m.state == stateScanComplete && !m.largeProjectPending {
		m.state = statePromptFileSaving
		m.promptFileSavingKind = topologyPromptKind
		m.status = neutralMessage("Saving Prompt 1: Topology to Downloads...")
		return m, savePromptToDownloadsCmd(topologyPromptKind, m.schemaA), true
	}
	if m.state == stateContextReady {
		m.state = statePromptFileSaving
		m.promptFileSavingKind = codeContextPromptKind
		m.status = neutralMessage("Saving Prompt 2: Code Context to Downloads...")
		return m, savePromptToDownloadsCmd(codeContextPromptKind, m.schemaB), true
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
		m.status = neutralMessage(fmt.Sprintf("%s printed to terminal.", codeContextPromptKind))
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
		if m.goalFocus == goalFocusEditor {
			before := m.goalInput.Value()
			if isGoalPasteInsert(msg) && isLargeGoalPaste(string(msg.Runes)) {
				pastedText := string(msg.Runes)
				m.goalInput.SetValue(before)
				m.resizeGoalEditor()
				m.appendGoalAttachment(newGoalTextAttachment("paste", pastedText))
				m.goalInputLastRuneAt = time.Time{}
				m.goalInputLastRuneBefore = ""
				return m, textarea.Blink
			}
			if m.goalPasteCapture {
				m.goalPasteBuffer += string(msg.Runes)
				if isLargeGoalPaste(m.goalPasteBuffer) {
					m.goalInput.SetValue(m.goalPasteBaseline)
				} else {
					m.goalInput.SetValue(m.goalPasteBaseline + m.goalPasteBuffer)
				}
				m.resizeGoalEditor()
				m.goalInputLastRuneAt = time.Now()
				return m, tea.Tick(goalPasteFlushDelay, func(time.Time) tea.Msg { return goalPasteFlushMsg{} })
			}
			if m.goalPasteBurstLikely(msg) {
				baseline := m.goalInputLastRuneBefore
				buffer := before
				if strings.HasPrefix(before, baseline) {
					buffer = before[len(baseline):]
				}
				buffer += string(msg.Runes)
				m.goalPasteCapture = true
				m.goalPasteBaseline = baseline
				m.goalPasteBuffer = buffer
				if isLargeGoalPaste(buffer) {
					m.goalInput.SetValue(baseline)
				} else {
					m.goalInput.SetValue(baseline + buffer)
				}
				m.resizeGoalEditor()
				m.goalInputLastRuneAt = time.Now()
				return m, tea.Tick(goalPasteFlushDelay, func(time.Time) tea.Msg { return goalPasteFlushMsg{} })
			}
			m.goalInput, cmd = m.goalInput.Update(msg)
			m.enforceGoalByteLimit(pastedKeyMsg(msg))
			m.resizeGoalEditor()
			m.refreshCompletionCandidate()
			m.pruneCompletionSuppression()
			if msg.Type == tea.KeyRunes {
				after := m.goalInput.Value()
				if isLargeGoalPaste(after) {
					pastedText := insertedText(before, after)
					if pastedText == "" {
						pastedText = string(msg.Runes)
					}
					m.goalPasteCapture = true
					m.goalPasteBaseline = before
					m.goalPasteBuffer = pastedText
					m.goalInput.SetValue(before)
					m.resizeGoalEditor()
					m.goalInputLastRuneAt = time.Now()
					return m, tea.Tick(goalPasteFlushDelay, func(time.Time) tea.Msg { return goalPasteFlushMsg{} })
				}
				m.goalInputLastRuneAt = time.Now()
				m.goalInputLastRuneBefore = before
			}
		}
	case stateWaitingForExtractions, stateWaitingForCode:
		m.paste, cmd = m.paste.Update(msg)
		if pastedKeyMsg(msg) {
			// Auto-submit on paste so the user does not have to press Enter.
			return m.submitPasteState()
		}
	}
	return m, cmd
}

func isGoalPasteInsert(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes {
		return false
	}
	return msg.Paste || len(msg.Runes) > 1
}

func goalEditorKeyFinalizesPasteCapture(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyLeft, tea.KeyRight, tea.KeyUp, tea.KeyDown, tea.KeyHome, tea.KeyEnd:
		return true
	default:
		return false
	}
}

func goalEditorKeyBypassesAppRouting(msg tea.KeyMsg) bool {
	switch msg.Type {
	case tea.KeyBackspace, tea.KeyDelete, tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd:
		return true
	default:
		return false
	}
}

func (m Model) goalPasteBurstLikely(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 || m.goalInputLastRuneAt.IsZero() {
		return false
	}
	return time.Since(m.goalInputLastRuneAt) < goalPasteBurstWindow
}

func (m Model) statusLine() string {
	mode := statusLineKeyboardHints
	switch mode {
	case statusLineKeyboardHints:
		hints := keyboardHintsForState(m.state)
		if m.state == stateHome && protocol.NormalizeFocus(m.cfg.Focus) == protocol.FocusReview {
			hints = append([]string{"Ctrl+R refresh review"}, hints...)
		}
		return strings.Join(append([]string{"Focus: " + workflow.FocusDisplayName(m.cfg.Focus)}, hints...), " · ")
	default:
		return ""
	}
}

func keyboardHintsForState(st state) []string {
	hints := []string{"Ctrl+C quit"}
	switch st {
	case stateHome:
		hints = []string{"Enter submit", "Ctrl+C quit"}
	case stateWaitingForExtractions, stateWaitingForCode:
		hints = []string{"Enter submit", "Esc cancel", "Ctrl+C quit"}
	case stateContextWarning:
		hints = []string{"Enter return", "Y proceed", "N return", "Esc cancel", "Ctrl+C quit"}
	case statePromptFileSaving:
		hints = []string{"Ctrl+C quit"}
	case stateBadge:
		hints = []string{"Enter continue", "Ctrl+C quit"}
	case stateScanning, stateWriting:
		// Ctrl+C only.
	default:
		hints = append(hints, "Esc cancel")
	}
	return hints
}
