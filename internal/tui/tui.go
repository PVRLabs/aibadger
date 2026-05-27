package tui

// This file owns the high-level Bubble Tea model, message handling, and
// workflow transitions for the TUI.

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/taggedfile"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/term"
)

type state int

const (
	stateHome state = iota
	stateOnboarding
	stateScanning
	stateScanComplete
	stateWaitingForExtractions
	stateContextWarning
	stateContextReady
	stateWaitingForCode
	stateTextResponse
	stateWritePreview
	stateWriting
	stateManualCopy
	stateHelp
	statePromptFileReveal
	stateBadgePermissionPrompt
	stateBadgeFetching
	stateBadgeResult
	stateBadgeError
)

const (
	topologyPromptKind    = workflow.TopologyPromptKind
	codeContextPromptKind = workflow.CodeContextPromptKind
	helpCommand           = "/help"
	reviewCommand         = "/review"
	designCommand         = "/design"
	badgeCommand          = "/badge"
)

type Model struct {
	root string
	cfg  Config

	state  state
	goal   string
	status tuiMessage
	err    error

	goalInput              textarea.Model
	paste                  textarea.Model
	goalFocus              goalFocusState
	goalAttachmentSelected int
	goalAttachments        []goalAttachment

	eng                     *engine.Engine
	session                 *workflow.Session
	schemaA                 string
	schemaB                 string
	commands                []extractor.Command
	metadata                []protocol.ExtractionMetadata
	pendingSchemaB          string
	pendingMetadata         []protocol.ExtractionMetadata
	pendingExtractedCount   int
	pendingFailedCommands   []string
	pendingSafetyExclusions []string
	updates                 []writer.FileUpdate
	response                string

	onboardingCompletionSaved bool

	manualCopyKind string
	manualCopyText string

	promptFileKind string
	promptFilePath string

	badgeLogins    []string
	badgeTotal     int
	badgeGazillion bool
	badgeErrorText string

	externalRoots       []taggedfile.ExternalRoot
	completion          completionState
	largeProjectPending bool
	scanFrame           int
	width               int
	height              int
}

type scanDoneMsg struct {
	eng *engine.Engine
	err error
}

type tickMsg time.Time

type copyDoneMsg struct {
	kind string
	text string
	err  error
}

type savePromptDoneMsg struct {
	kind         string
	text         string
	path         string
	canReveal    bool
	clipboardErr error
	err          error
}

type openPromptFileDoneMsg struct {
	kind string
	path string
	err  error
}

type contextDoneMsg struct {
	schema           string
	metadata         []protocol.ExtractionMetadata
	extractedCount   int
	failedCommands   []string
	safetyExclusions []string
	err              error
}

type writeDoneMsg struct {
	updates []writer.FileUpdate
	errs    []error
}

type badgePermissionPromptMsg struct{}

type badgeFetchingMsg struct{}

type badgeResultMsg struct {
	logins    []string
	total     int
	gazillion bool
}

type badgeErrorMsg struct {
	text string
}

// Run starts the interactive Bubble Tea TUI.
func Run(root string) error {
	return RunWithConfig(root, DefaultConfig())
}

func RunWithConfig(root string, cfg Config) error {
	if err := engine.CheckDisabledAndExit(root, os.Stdout); err != nil {
		return nil
	}
	_, err := tea.NewProgram(NewModel(root, cfg), tea.WithAltScreen()).Run()
	return err
}

func NewModel(root string, cfg Config) Model {
	cfg = cfg.withDefaults()
	goalInput := textarea.New()
	goalInput.Placeholder = "Describe the task..."
	goalInput.Prompt = "> "
	goalInput.CharLimit = 0
	goalInput.SetWidth(initialEditorWidth())
	goalInput.SetHeight(goalEditorHeight("", 0))
	goalInput.Focus()

	paste := textarea.New()
	paste.Placeholder = "Paste here..."
	paste.Prompt = "  "
	paste.CharLimit = 0
	paste.SetWidth(initialEditorWidth())
	paste.SetHeight(12)
	paste.Blur()

	m := Model{
		root:                   root,
		cfg:                    cfg,
		state:                  stateHome,
		goalInput:              goalInput,
		paste:                  paste,
		goalFocus:              goalFocusEditor,
		goalAttachmentSelected: -1,
		session:                workflow.NewSession(nil, cfg.WhitespaceMode),
		externalRoots:          loadExternalRoots(root),
	}

	settings, showOnboarding, onboardingCompleted := loadSettingsState(cfg.SettingsPath)
	if settings.WhitespaceMode != "" {
		cfg.WhitespaceMode = writer.WhitespaceMode(settings.WhitespaceMode)
		m.cfg = cfg
		m.session.WhitespaceMode = cfg.WhitespaceMode
	}
	m.onboardingCompletionSaved = onboardingCompleted
	if m.cfg.StartupGoal != "" {
		m.applyStartupGoal()
	}
	if showOnboarding && !m.cfg.SkipOnboarding && m.cfg.StartupGoal == "" {
		m.state = stateOnboarding
		m.goalInput.Blur()
	}
	return m
}

func (m *Model) applyStartupGoal() {
	if strings.TrimSpace(m.cfg.StartupGoal) == badgeCommand {
		m.state = stateBadgePermissionPrompt
		m.status = tuiMessage{}
		m.err = nil
		m.setGoalInputValue("")
		m.setGoalAttachments(nil)
		m.resizeGoalEditor()
		m.completion.suppressedKey = ""
		m.goalInput.Blur()
		m.paste.Blur()
		return
	}
	m.state = stateHome
	m.status = startupMessage(m.cfg.StartupStatusSeverity, m.cfg.StartupStatus)
	m.err = nil
	m.setGoalInputValue(m.cfg.StartupGoal)
	m.setGoalAttachments(startupGoalAttachments(m.cfg))
	m.resizeGoalEditor()
	m.completion.suppressedKey = ""
	m.goalInput.Focus()
	m.paste.Blur()
}

func (m *Model) setGoalAttachments(attachments []goalAttachment) {
	m.goalAttachments = append([]goalAttachment(nil), attachments...)
	m.goalFocus = goalFocusEditor
	if len(m.goalAttachments) == 0 {
		m.goalAttachmentSelected = -1
		return
	}
	if m.goalAttachmentSelected < 0 || m.goalAttachmentSelected >= len(m.goalAttachments) {
		m.goalAttachmentSelected = 0
	}
}

func startupGoalAttachments(cfg Config) []goalAttachment {
	text := strings.TrimSpace(cfg.StartupAttachmentText)
	if text == "" {
		return nil
	}

	kind := goalAttachmentType(strings.TrimSpace(cfg.StartupAttachmentType))
	source := strings.TrimSpace(cfg.StartupAttachmentSource)
	if kind == goalAttachmentText {
		return []goalAttachment{newGoalTextAttachment(source, cfg.StartupAttachmentText)}
	}
	return []goalAttachment{newGoalGitDiffAttachmentWithStats(source, cfg.StartupAttachmentText, cfg.StartupAttachmentFilesChanged, cfg.StartupAttachmentAdditions, cfg.StartupAttachmentDeletions)}
}

func startupMessage(severity, text string) tuiMessage {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "success":
		return successMessage(text)
	case "warning":
		return warningMessage(text)
	case "error":
		return errorMessage(text)
	default:
		return neutralMessage(text)
	}
}

func loadExternalRoots(root string) []taggedfile.ExternalRoot {
	contexts, err := externalcontext.Load(root)
	if err != nil || len(contexts) == 0 {
		return nil
	}
	roots := make([]taggedfile.ExternalRoot, 0, len(contexts))
	for _, ctx := range contexts {
		ctx := ctx
		roots = append(roots, taggedfile.ExternalRoot{
			Path:    ctx.Path,
			AbsPath: ctx.AbsPath,
			IsOmitted: func(relPath, absPath string) bool {
				return externalcontext.IsOmittedPath(ctx.AbsPath, absPath, relPath)
			},
		})
	}
	return roots
}

func loadSettingsState(settingsPath string) (Settings, bool, bool) {
	if settingsPath == "" {
		return Settings{}, false, true
	}
	settings, err := LoadSettings(settingsPath)
	if err != nil {
		return Settings{}, true, false
	}
	return settings, !settings.FirstRunOnboardingCompleted, settings.FirstRunOnboardingCompleted
}

func (m Model) refreshTopologyPrompt() (Model, []string) {
	schema, warnings := m.workflowSession().GenerateMapDetailed(m.goal)
	m.schemaA = schema
	return m, warnings
}

func taggedFileWarningMessage(warnings []string) tuiMessage {
	if len(warnings) == 0 {
		return tuiMessage{}
	}
	lines := []string{"Tagged file references produced warnings:"}
	for _, warning := range warnings {
		lines = append(lines, "- "+warning)
	}
	return warningMessage(strings.Join(lines, "\n"))
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.goalInput.SetWidth(clamp(msg.Width-8, 40, 100))
		m.resizeGoalEditor()
		m.paste.SetWidth(clamp(msg.Width-8, 40, 100))
		m.paste.SetHeight(clamp(msg.Height-14, 8, 18))
		return m, tea.ClearScreen
	case tickMsg:
		if m.state != stateScanning {
			return m, nil
		}
		m.scanFrame++
		return m, scanTick()
	case scanDoneMsg:
		if msg.err != nil {
			m.state = stateHome
			m.err = msg.err
			m.status = tuiMessage{}
			m.goalInput.Focus()
			return m, textarea.Blink
		}
		m.eng = msg.eng
		workflow.ConfigureEngine(m.eng, m.engineOptions(0))
		m.session = workflow.NewSession(m.eng, m.cfg.WhitespaceMode)
		m.err = nil
		m.state = stateScanComplete
		m.largeProjectPending = totalFiles(m.eng.Topology) > m.cfg.LargeProjectFileThreshold
		if !m.largeProjectPending {
			var warnings []string
			m, warnings = m.refreshTopologyPrompt()
			if warning := taggedFileWarningMessage(warnings); !warning.empty() {
				m.status = warning
			}
		}
		return m, nil
	case copyDoneMsg:
		if msg.err != nil {
			m.status = warningMessage(fmt.Sprintf("%s clipboard copy failed: %v", msg.kind, msg.err))
			return m, savePromptAfterClipboardFailureCmd(msg.kind, msg.text, msg.err)
		}
		m.status = successMessage(fmt.Sprintf("%s copied to clipboard.", msg.kind))
		return m.advanceAfterCopy(msg.kind, false)
	case savePromptDoneMsg:
		if msg.clipboardErr != nil {
			return m.handleClipboardFallbackSave(msg)
		}
		if msg.err != nil {
			m.status = errorMessage(fmt.Sprintf("Could not save %s to temp file: %v", msg.kind, msg.err))
			return m, nil
		}
		if msg.canReveal {
			m.state = statePromptFileReveal
			m.promptFileKind = msg.kind
			m.promptFilePath = msg.path
			m.status = successMessage(fmt.Sprintf("Saved %s to temp file:\n\n%s", msg.kind, msg.path))
			return m, nil
		}
		return m.advanceAfterTempFile(msg.kind, msg.path)
	case openPromptFileDoneMsg:
		if msg.err != nil {
			m.status = warningMessage(fmt.Sprintf("Could not open the file manager automatically.\nAttach this file to your AI chat:\n\n%s", msg.path))
		} else {
			m.status = successMessage(fmt.Sprintf("Saved %s to temp file:\n\n%s\n\nAttach this file to your AI chat.", msg.kind, msg.path))
		}
		return m.advanceAfterTempFileWithStatus(msg.kind, msg.path, m.status)
	case contextDoneMsg:
		if msg.err != nil {
			m.pendingSchemaB = ""
			m.pendingMetadata = nil
			m.pendingExtractedCount = 0
			m.pendingFailedCommands = nil
			m.pendingSafetyExclusions = nil
			m.err = msg.err
			m.status = tuiMessage{}
			return m, nil
		}
		if len(msg.failedCommands) > 0 || len(msg.safetyExclusions) > 0 {
			m.pendingSchemaB = msg.schema
			m.pendingMetadata = append([]protocol.ExtractionMetadata(nil), msg.metadata...)
			m.pendingExtractedCount = msg.extractedCount
			m.pendingFailedCommands = append([]string(nil), msg.failedCommands...)
			m.pendingSafetyExclusions = append([]string(nil), msg.safetyExclusions...)
			m.state = stateContextWarning
			m.status = warningMessage("Partial extraction detected. Review the warning below.")
			m.err = nil
			return m, nil
		}
		m.schemaB = msg.schema
		m.metadata = msg.metadata
		m.state = stateContextReady
		m.status = neutralMessage(workflow.ContextReadyStatus(m.cfg.Focus))
		m.err = nil
		return m, nil
	case writeDoneMsg:
		m.state = stateHome
		m.goal = ""
		m.schemaA = ""
		m.schemaB = ""
		m.commands = nil
		m.updates = nil
		m.response = ""
		m.setGoalAttachments(nil)
		m.setGoalInputValue("")
		m.resizeGoalEditor()
		m.completion.suppressedKey = ""
		m.goalInput.Focus()
		m.paste.Blur()
		if len(msg.errs) > 0 {
			m.status = errorMessage(fmt.Sprintf("Finished with %d apply error(s).", len(msg.errs)))
		} else {
			writes, deletes := countAppliedKinds(msg.updates)
			switch {
			case writes > 0 && deletes > 0:
				m.status = successMessage(fmt.Sprintf("Applied %d write(s) and %d delete(s). Ready for the next goal.", writes, deletes))
			case deletes > 0:
				m.status = successMessage(fmt.Sprintf("Deleted %d file(s). Ready for the next goal.", deletes))
			default:
				m.status = successMessage(fmt.Sprintf("Wrote %d file(s). Ready for the next goal.", writes))
			}
		}
		return m, textarea.Blink
	case badgePermissionPromptMsg:
		return m, nil
	case badgeFetchingMsg:
		return m, nil
	case badgeResultMsg:
		m.state = stateBadgeResult
		m.badgeLogins = append([]string(nil), msg.logins...)
		m.badgeTotal = msg.total
		m.badgeGazillion = msg.gazillion
		m.badgeErrorText = ""
		m.status = tuiMessage{}
		m.err = nil
		return m, nil
	case badgeErrorMsg:
		m.state = stateBadgeError
		m.badgeErrorText = msg.text
		m.badgeLogins = nil
		m.badgeTotal = 0
		m.badgeGazillion = false
		m.status = tuiMessage{}
		m.err = nil
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	var cmd tea.Cmd
	switch m.state {
	case stateHome:
		m.goalInput, cmd = m.goalInput.Update(msg)
		m.resizeGoalEditor()
		m.refreshCompletionCandidate()
	case stateWaitingForExtractions, stateWaitingForCode:
		m.paste, cmd = m.paste.Update(msg)
	}
	return m, cmd
}

func (m Model) handleClipboardFallbackSave(msg savePromptDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.state = stateManualCopy
		m.status = errorMessage(fmt.Sprintf(
			"%s clipboard copy failed: %v\nFor instructions on installing a clipboard tool visit %s\nCould not save %s to temp file: %v\nManually copy from the block below.",
			msg.kind,
			msg.clipboardErr,
			clipboard.DocsURL,
			msg.kind,
			msg.err,
		))
		m.manualCopyKind = msg.kind
		m.manualCopyText = msg.text
		return m, nil
	}

	status := warningMessage(fmt.Sprintf(
		"%s clipboard copy failed: %v\nFor instructions on installing a clipboard tool visit %s\nSaved %s to temp file:\n\n%s",
		msg.kind,
		msg.clipboardErr,
		clipboard.DocsURL,
		msg.kind,
		msg.path,
	))
	if msg.canReveal {
		m.state = statePromptFileReveal
		m.promptFileKind = msg.kind
		m.promptFilePath = msg.path
		m.status = status
		return m, nil
	}
	return m.advanceAfterTempFileWithStatus(msg.kind, msg.path, warningMessage(fmt.Sprintf(
		"%s\n\nAttach this file to your AI chat, or open it and copy from it manually.",
		status.text,
	)))
}

func (m Model) submitGoal() (tea.Model, tea.Cmd) {
	instruction := strings.TrimSpace(m.goalInput.Value())
	goal := assembleGoalSubmission(instruction, m.goalAttachments)
	if goal == "" {
		return m, nil
	}
	if instruction == m.cfg.ExitCommand {
		return m, tea.Quit
	}
	if instruction == helpCommand {
		m.state = stateHelp
		m.goalInput.Blur()
		m.status = tuiMessage{}
		return m, nil
	}
	if reviewExtraFocus, ok := parseReviewCommand(instruction); ok {
		return m.handleReviewCommand(reviewExtraFocus)
	}
	if instruction == designCommand {
		return m.handleDesignCommand()
	}
	if instruction == badgeCommand {
		return m.handleBadgeCommand()
	}

	m.goal = goal
	m.status = tuiMessage{}
	m.err = nil
	m.state = stateScanning
	m.scanFrame = 0
	m.goalInput.Blur()
	return m, tea.Batch(scanProjectCmd(m.root, m.cfg.MaxFilesPerDirectory), scanTick())
}

func (m Model) handleDesignCommand() (tea.Model, tea.Cmd) {
	m.cfg.Focus = protocol.FocusDesign
	m.status = successMessage("Focus set to Design.")
	m.err = nil
	m.setGoalInputValue(protocol.DefaultDesignPrompt)
	m.setGoalAttachments(nil)
	m.resizeGoalEditor()
	m.completion.suppressedKey = ""
	m.goalInput.Focus()
	return m, textarea.Blink
}

func (m Model) handleBadgeCommand() (tea.Model, tea.Cmd) {
	m.state = stateBadgePermissionPrompt
	m.badgeLogins = nil
	m.badgeTotal = 0
	m.badgeGazillion = false
	m.badgeErrorText = ""
	m.status = tuiMessage{}
	m.err = nil
	m.setGoalAttachments(nil)
	m.goalInput.Blur()
	return m, func() tea.Msg { return badgePermissionPromptMsg{} }
}

func parseReviewCommand(goal string) (string, bool) {
	goal = strings.TrimSpace(goal)
	if goal == reviewCommand {
		return "", true
	}
	if !strings.HasPrefix(goal, reviewCommand+" ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(goal, reviewCommand)), true
}

func (m Model) handleReviewCommand(extraFocus string) (tea.Model, tea.Cmd) {
	task, err := reviewtask.Build(m.root, reviewtask.Options{
		Mode:       reviewtask.ModeDefault,
		ExtraFocus: extraFocus,
	})
	if err != nil {
		m.status = errorMessage(fmt.Sprintf("Unable to prepare review prompt: %v", err))
		m.err = nil
		m.goalInput.Focus()
		return m, textarea.Blink
	}

	m.cfg.Focus = protocol.FocusReview
	m.state = stateHome
	m.goal = ""
	m.err = nil
	m.completion.suppressedKey = ""
	if task.FailureClassification == reviewtask.FailureNone {
		m.setGoalInputValue(task.Instruction)
		m.setGoalAttachments([]goalAttachment{newGoalGitDiffAttachmentWithStats("git diff", task.Diff, task.FilesChanged, task.Additions, task.Deletions)})
	} else {
		m.setGoalInputValue(task.StartupPrompt())
		m.setGoalAttachments(nil)
	}
	m.resizeGoalEditor()
	m.goalInput.Focus()
	m.paste.Blur()

	status, severity := task.StartupStatus()
	m.status = startupMessage(severity, status)

	return m, textarea.Blink
}

func (m Model) advanceAfterCopy(kind string, manual bool) (tea.Model, tea.Cmd) {
	m.manualCopyKind = ""
	m.manualCopyText = ""
	promptTwoKind := workflow.PromptTwoKind(m.cfg.Focus)
	switch {
	case kind == topologyPromptKind:
		if manual {
			m.status = neutralMessage("Prompt 1: Topology shown for manual copy. Paste it into any LLM chat interface, then paste extraction commands.")
		} else {
			m.status = successMessage("Prompt 1: Topology copied. Paste it into any LLM chat interface, then paste extraction commands.")
		}
		m.state = stateWaitingForExtractions
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
	case kind == codeContextPromptKind || kind == promptTwoKind:
		m.markOnboardingCompleted()
		m.state = stateWaitingForCode
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
		statusText := fmt.Sprintf("%s copied.", promptTwoKind)
		if protocol.NormalizeFocus(m.cfg.Focus) == protocol.FocusCode {
			statusText += " Next: paste the final AI response."
		}
		if manual {
			m.status = neutralMessage(statusText)
		} else {
			m.status = successMessage(statusText)
		}
	}
	return m, textarea.Blink
}

func (m Model) advanceAfterTempFile(kind, path string) (tea.Model, tea.Cmd) {
	return m.advanceAfterTempFileWithStatus(kind, path, successMessage(fmt.Sprintf("Saved %s to temp file:\n\n%s\n\nAttach this file to your AI chat, or open it and copy from it manually.", kind, path)))
}

func (m Model) advanceAfterTempFileWithStatus(kind, path string, status tuiMessage) (tea.Model, tea.Cmd) {
	m.manualCopyKind = ""
	m.manualCopyText = ""
	m.promptFileKind = ""
	m.promptFilePath = ""
	m.status = status
	promptTwoKind := workflow.PromptTwoKind(m.cfg.Focus)
	switch {
	case kind == topologyPromptKind:
		m.state = stateWaitingForExtractions
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
	case kind == codeContextPromptKind || kind == promptTwoKind:
		m.markOnboardingCompleted()
		m.state = stateWaitingForCode
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
	}
	return m, textarea.Blink
}

func (m Model) cancelPromptDelivery(kind string) (tea.Model, tea.Cmd) {
	promptTwoKind := workflow.PromptTwoKind(m.cfg.Focus)
	switch {
	case kind == topologyPromptKind:
		m.status = neutralMessage("Prompt 1: Topology was not copied.")
		m.state = stateWaitingForExtractions
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
	case kind == codeContextPromptKind || kind == promptTwoKind:
		m.status = neutralMessage(fmt.Sprintf("%s was not copied.", promptTwoKind))
		m.state = stateWaitingForCode
		m.resetPaste(pasteSpecForState(m.state, m.cfg.Focus).placeholder)
	}
	return m, textarea.Blink
}

func (m Model) submitExtractions() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.paste.Value())
	session := m.workflowSession()
	result := session.ParseExtractionInput(input)
	m.commands = result.Commands
	if result.Empty {
		m.status = tuiMessage{}
		m.err = fmt.Errorf("No extraction commands found. Paste FILE/PREFIX/NEAR commands and press Enter.")
		return m, nil
	}
	m.status = successMessage(fmt.Sprintf("Parsed %d extraction command(s).", result.Count))
	m.err = nil
	return m, contextCmd(session, m.goal, m.commands)
}

func (m Model) acceptPartialExtractionWarning() (tea.Model, tea.Cmd) {
	m.schemaB = m.pendingSchemaB
	m.metadata = append([]protocol.ExtractionMetadata(nil), m.pendingMetadata...)
	m.pendingSchemaB = ""
	m.pendingMetadata = nil
	m.pendingExtractedCount = 0
	m.pendingFailedCommands = nil
	m.pendingSafetyExclusions = nil
	m.state = stateContextReady
	m.status = warningMessage("Proceeding with available context after extraction warnings.")
	m.err = nil
	return m, nil
}

func (m Model) rejectPartialExtractionWarning() (tea.Model, tea.Cmd) {
	m.pendingSchemaB = ""
	m.pendingMetadata = nil
	m.pendingExtractedCount = 0
	m.pendingFailedCommands = nil
	m.pendingSafetyExclusions = nil
	m.schemaB = ""
	m.metadata = nil
	m.state = stateWaitingForExtractions
	m.status = neutralMessage("Returned to extraction input. Adjust the file requests and try again.")
	m.err = nil
	m.paste.Focus()
	m.goalInput.Blur()
	return m, textarea.Blink
}

func (m Model) submitFinalResponse() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.paste.Value())
	final := m.workflowSession().ParseFinalResponse(input)
	m.updates = final.Parse.Updates
	m.response = final.Parse.Text
	if final.HasErrors {
		m.state = stateWaitingForCode
		m.status = tuiMessage{}
		m.err = errors.Join(final.Parse.Errors...)
		return m, nil
	}
	if !final.HasUpdates {
		m.state = stateTextResponse
		m.status = tuiMessage{}
		m.paste.Blur()
		return m, nil
	}
	m.state = stateWritePreview
	if final.HasNotes {
		m.status = successMessage(fmt.Sprintf("Parsed %d file operation(s). AI also included notes.", len(m.updates)))
	} else {
		m.status = successMessage(fmt.Sprintf("Parsed %d file operation(s).", len(m.updates)))
	}
	return m, nil
}

func (m Model) submitPasteState() (tea.Model, tea.Cmd) {
	switch m.state {
	case stateWaitingForExtractions:
		return m.submitExtractions()
	case stateWaitingForCode:
		return m.submitFinalResponse()
	default:
		return m, nil
	}
}

func (m Model) returnHome(status tuiMessage) (tea.Model, tea.Cmd) {
	m.state = stateHome
	m.status = status
	m.err = nil
	m.goal = ""
	m.schemaA = ""
	m.schemaB = ""
	m.commands = nil
	m.pendingSchemaB = ""
	m.pendingMetadata = nil
	m.pendingExtractedCount = 0
	m.pendingFailedCommands = nil
	m.pendingSafetyExclusions = nil
	m.updates = nil
	m.response = ""
	m.badgeLogins = nil
	m.badgeTotal = 0
	m.badgeGazillion = false
	m.badgeErrorText = ""
	m.setGoalInputValue("")
	m.resizeGoalEditor()
	m.completion.suppressedKey = ""
	m.goalInput.Focus()
	m.paste.Blur()
	return m, textarea.Blink
}

func (m *Model) resizeGoalEditor() {
	m.goalInput.SetHeight(goalEditorHeight(m.goalInput.Value(), m.height))
}

func initialEditorWidth() int {
	if width, _, err := term.GetSize(os.Stdout.Fd()); err == nil && width > 0 {
		return clamp(width-8, 40, 100)
	}
	return 76
}

func (m *Model) resetPaste(placeholder string) {
	m.paste.SetValue("")
	m.paste.Placeholder = placeholder
	m.paste.Focus()
	m.goalInput.Blur()
}

func (m *Model) markOnboardingCompleted() {
	if m.cfg.SettingsPath == "" || m.onboardingCompletionSaved {
		return
	}
	_ = SaveSettings(m.cfg.SettingsPath, Settings{FirstRunOnboardingCompleted: true})
	m.onboardingCompletionSaved = true
}
