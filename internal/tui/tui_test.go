package tui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/github"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func testDisplaySymbols() displaySymbols {
	return defaultDisplaySymbols()
}

func testGetenv(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestDisplaySymbolsUseASCIIFallbackWithoutUnicodeSignalsOnWindows(t *testing.T) {
	got := displaySymbolsForRuntime("windows", testGetenv(nil))

	if got.success != "[OK]" || got.warning != "[!]" || got.error != "[X]" || got.pipelineSep != " -> " {
		t.Fatalf("windows symbols = %#v, want ASCII fallback", got)
	}
}

func TestDisplaySymbolsUseUnicodeForWindowsTerminal(t *testing.T) {
	got := displaySymbolsForRuntime("windows", testGetenv(map[string]string{"WT_SESSION": "session-id"}))

	if got.success != "✓" || got.warning != "⚠️" || got.error != "⛔" || got.pipelineSep != " → " {
		t.Fatalf("windows terminal symbols = %#v, want unicode symbols", got)
	}
}

func TestDisplaySymbolsUseUnicodeForUTF8Locale(t *testing.T) {
	got := displaySymbolsForRuntime("windows", testGetenv(map[string]string{"LANG": "en_US.UTF-8"}))

	if got.success != "✓" || got.warning != "⚠️" || got.error != "⛔" || got.pipelineSep != " → " {
		t.Fatalf("utf-8 symbols = %#v, want unicode symbols", got)
	}
}

func TestDisplaySymbolsUseASCIIFallbackWhenForced(t *testing.T) {
	got := displaySymbolsForRuntime("darwin", testGetenv(map[string]string{"BADGER_ASCII": "1", "LANG": "en_US.UTF-8"}))

	if got.success != "[OK]" || got.warning != "[!]" || got.error != "[X]" || got.pipelineSep != " -> " {
		t.Fatalf("forced ascii symbols = %#v, want ASCII fallback", got)
	}
}

func TestNewModelStartsAtHomeWithoutScanning(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	if m.state != stateHome {
		t.Fatalf("state = %v, want %v", m.state, stateHome)
	}
	if m.eng != nil {
		t.Fatal("NewModel scanned before goal submission")
	}
	if !m.goalInput.Focused() {
		t.Fatal("goal input is not focused on launch")
	}
	if got := m.goalInput.Height(); got != goalEditorMinHeight {
		t.Fatalf("goal input height = %d, want %d", got, goalEditorMinHeight)
	}
}

func TestNewModelShowsOnboardingForMissingSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(t.TempDir(), ".badger", "settings.json")

	m := NewModel("/tmp/project", cfg)

	if m.state != stateOnboarding {
		t.Fatalf("state = %v, want %v", m.state, stateOnboarding)
	}
	if m.goalInput.Focused() {
		t.Fatal("goal input is focused during onboarding")
	}
	if !strings.Contains(m.View(), "First run") {
		t.Fatalf("onboarding view missing first-run copy:\n%s", m.View())
	}
	for _, want := range []string{
		"lightweight local bridge",
		"How it works:",
		"1. Map",
		"Badger builds a prompt",
		"copy it",
		"paste into your AI chat",
		"2. Extract",
		"AI replies asking",
		"paste back into Badger",
		"3. Apply",
		"builds a second prompt",
		"review before writing",
		"Fully local",
		"nothing leaves your machine",
		"You control every paste",
		"Press Enter to continue",
	} {
		if !strings.Contains(m.View(), want) {
			t.Fatalf("onboarding view missing %q:\n%s", want, m.View())
		}
	}
}

func TestOnboardingRespectsASCIIEnv(t *testing.T) {
	t.Setenv("BADGER_ASCII", "1")
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(t.TempDir(), ".badger", "settings.json")

	m := NewModel("/tmp/project", cfg)

	view := m.View()
	if !strings.Contains(view, "[OK] Fully local -") {
		t.Fatalf("onboarding view missing ASCII success '[OK]' or dash '-':\n%s", view)
	}
	if !strings.Contains(view, "-> You copy it ->") {
		t.Fatalf("onboarding view missing ASCII arrows '->':\n%s", view)
	}
}

func TestOnboardingDismissEntersHomeWithoutCreatingSettings(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if !got.goalInput.Focused() {
		t.Fatal("goal input is not focused after onboarding")
	}
	if cmd == nil {
		t.Fatal("onboarding dismissal did not return blink command")
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings file exists after onboarding dismissal, stat error = %v", err)
	}
}

func TestNewModelSkipsOnboardingWhenCompleted(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	if err := SaveSettings(settingsPath, Settings{FirstRunOnboardingCompleted: true}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath

	m := NewModel("/tmp/project", cfg)

	if m.state != stateHome {
		t.Fatalf("state = %v, want %v", m.state, stateHome)
	}
}

func TestNewModelAppliesStartupReviewGoalAndSkipsOnboarding(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg.Startup.Goal = "Review this change for concrete bugs."
	cfg.Startup.Attachments = []startup.Attachment{{
		Type:         "git diff",
		Source:       "git diff",
		Text:         "diff --git a/app.go b/app.go\n",
		FilesChanged: 1,
		Additions:    1,
		Deletions:    0,
	}}
	cfg.Startup.Status = startup.Status{
		Text:     "Loaded review prompt from the current git diff. Edit it before submitting.",
		Severity: "success",
	}
	cfg.SkipOnboarding = true

	m := NewModel("/tmp/project", cfg)

	if m.state != stateHome {
		t.Fatalf("state = %v, want %v", m.state, stateHome)
	}
	if !m.goalInput.Focused() {
		t.Fatal("goal input is not focused for startup review mode")
	}
	if got := m.goalInput.Value(); got != cfg.Startup.Goal {
		t.Fatalf("goal input = %q, want %q", got, cfg.Startup.Goal)
	}
	if len(m.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(m.goalAttachments))
	}
	if m.goalAttachments[0].Type != goalAttachmentGitDiff {
		t.Fatalf("attachment type = %q, want %q", m.goalAttachments[0].Type, goalAttachmentGitDiff)
	}
	if m.goalAttachments[0].Text != cfg.Startup.Attachments[0].Text {
		t.Fatalf("attachment text = %q, want %q", m.goalAttachments[0].Text, cfg.Startup.Attachments[0].Text)
	}
	if m.status.severity != messageSuccess {
		t.Fatalf("status severity = %v, want %v", m.status.severity, messageSuccess)
	}
	if !strings.Contains(m.View(), cfg.Startup.Status.Text) {
		t.Fatalf("view missing startup status:\n%s", m.View())
	}
	if strings.Contains(m.View(), "First run") {
		t.Fatalf("startup review mode should skip onboarding:\n%s", m.View())
	}
}

func TestNewModelAppliesStartupDesignGoalWithEnoughHeight(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg.Startup.Goal = protocol.DefaultDesignPrompt
	cfg.Startup.Status = startup.Status{
		Text:     "Focus set to Design. Edit the goal before submitting.",
		Severity: "success",
	}
	cfg.SkipOnboarding = true

	m := NewModel("/tmp/project", cfg)

	if m.state != stateHome {
		t.Fatalf("state = %v, want %v", m.state, stateHome)
	}
	if got := m.goalInput.Value(); got != cfg.Startup.Goal {
		t.Fatalf("goal input = %q, want %q", got, cfg.Startup.Goal)
	}
	if got := m.goalInput.Height(); got != 4 {
		t.Fatalf("goal input height = %d, want 4", got)
	}
}

func TestNewModelAppliesBadgeStartupPrompt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg.Startup.Goal = "/badge"
	cfg.SkipOnboarding = true

	m := NewModel("/tmp/project", cfg)

	if m.state != stateBadgePermissionPrompt {
		t.Fatalf("state = %v, want %v", m.state, stateBadgePermissionPrompt)
	}
	if m.status != (tuiMessage{}) {
		t.Fatalf("status = %#v, want empty", m.status)
	}
	if got := m.goalInput.Value(); got != "" {
		t.Fatalf("goal input = %q, want empty", got)
	}
	if m.goalInput.Focused() {
		t.Fatal("goal input should be blurred for badge prompt")
	}
}

func TestNewModelAppliesFallbackReviewGoalAndWarningStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Startup.Goal = "Review the following change before committing."
	cfg.Startup.Status = startup.Status{
		Text:     "No git diff was detected. The prompt is editable.",
		Severity: "warning",
	}
	cfg.SkipOnboarding = true

	m := NewModel("/tmp/project", cfg)

	if m.status.severity != messageWarning {
		t.Fatalf("status severity = %v, want %v", m.status.severity, messageWarning)
	}
	if got := m.goalInput.Value(); got != cfg.Startup.Goal {
		t.Fatalf("goal input = %q, want %q", got, cfg.Startup.Goal)
	}
	if len(m.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(m.goalAttachments))
	}
	if !strings.Contains(m.View(), cfg.Startup.Status.Text) {
		t.Fatalf("view missing warning status:\n%s", m.View())
	}
}

func TestNewModelShowsOnboardingWhenSettingsFalse(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	if err := SaveSettings(settingsPath, Settings{FirstRunOnboardingCompleted: false}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath

	m := NewModel("/tmp/project", cfg)

	if m.state != stateOnboarding {
		t.Fatalf("state = %v, want %v", m.state, stateOnboarding)
	}
}

func TestNewModelShowsOnboardingWhenSettingsUnreadable(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(settingsPath, []byte("{"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath

	m := NewModel("/tmp/project", cfg)

	if m.state != stateOnboarding {
		t.Fatalf("state = %v, want %v", m.state, stateOnboarding)
	}
}

func TestNewModelLoadsWhitespaceModeFromSettings(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	if err := SaveSettings(settingsPath, Settings{
		FirstRunOnboardingCompleted: true,
		WhitespaceMode:              "ignore",
	}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath

	m := NewModel("/tmp/project", cfg)

	if m.cfg.WhitespaceMode != writer.WhitespaceModeIgnore {
		t.Fatalf("WhitespaceMode = %q, want %q", m.cfg.WhitespaceMode, writer.WhitespaceModeIgnore)
	}
}

func TestSubmitGoalTransitionsToScanning(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("add tests")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateScanning {
		t.Fatalf("state = %v, want %v", got.state, stateScanning)
	}
	if got.goal != "add tests" {
		t.Fatalf("goal = %q, want %q", got.goal, "add tests")
	}
	if cmd == nil {
		t.Fatal("submitGoal did not return scan command")
	}
}

func TestGoalPasteBelowLimitIsRetainedWithoutWarning(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Repeat("x", goalPasteAttachmentByteThreshold-1)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != paste {
		t.Fatalf("goal length = %d, want %d", len(got.goalInput.Value()), len(paste))
	}
	if !got.status.empty() {
		t.Fatalf("status = %#v, want empty", got.status)
	}
}

func TestLargeGoalPasteBecomesTextAttachmentByBytes(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Repeat("x", goalPasteAttachmentByteThreshold+1)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "" {
		t.Fatalf("goal input = %q, want empty inline editor after attachment conversion", got.goalInput.Value())
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Type != goalAttachmentText {
		t.Fatalf("attachment type = %q, want %q", got.goalAttachments[0].Type, goalAttachmentText)
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text did not preserve pasted payload")
	}
	if got.goalAttachments[0].Source != "paste" {
		t.Fatalf("attachment source = %q, want paste", got.goalAttachments[0].Source)
	}
}

func TestGoalPastePreservesMultilineText(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := "Review this ticket:\n\n- keep line breaks\n- preserve structure"

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != paste {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), paste)
	}
	if !got.status.empty() {
		t.Fatalf("status = %#v, want empty", got.status)
	}
	if got.goalInput.Height() != 4 {
		t.Fatalf("goal input height = %d, want 4", got.goalInput.Height())
	}
}

func TestLargeGoalPasteBecomesTextAttachmentByLines(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Join([]string{
		"line 01", "line 02", "line 03", "line 04", "line 05",
		"line 06", "line 07", "line 08", "line 09", "line 10",
		"line 11", "line 12", "line 13", "line 14", "line 15",
		"line 16", "line 17", "line 18", "line 19", "line 20",
		"line 21", "line 22", "line 23", "line 24", "line 25",
		"line 26", "line 27", "line 28", "line 29", "line 30",
		"line 31", "line 32", "line 33", "line 34", "line 35",
		"line 36", "line 37", "line 38", "line 39", "line 40",
		"line 41",
	}, "\n")

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text did not preserve pasted payload")
	}
	if got.goalAttachments[0].Lines != goalPasteAttachmentLineThreshold+1 {
		t.Fatalf("attachment lines = %d, want %d", got.goalAttachments[0].Lines, goalPasteAttachmentLineThreshold+1)
	}
}

func TestLargeGoalInsertWithoutPasteFlagStillBecomesAttachment(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Join([]string{
		"line 01", "line 02", "line 03", "line 04", "line 05",
		"line 06", "line 07", "line 08", "line 09", "line 10",
		"line 11", "line 12", "line 13", "line 14", "line 15",
		"line 16", "line 17", "line 18", "line 19", "line 20",
		"line 21", "line 22", "line 23", "line 24", "line 25",
		"line 26", "line 27", "line 28", "line 29", "line 30",
		"line 31", "line 32", "line 33", "line 34", "line 35",
		"line 36", "line 37", "line 38", "line 39", "line 40",
		"line 41",
	}, "\n")

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text did not preserve pasted payload")
	}
	if got.goalInput.Value() != "" {
		t.Fatalf("goal input = %q, want empty inline editor after attachment conversion", got.goalInput.Value())
	}
}

func TestLargeGoalPastePreservesExistingInstruction(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	instruction := "Review the following change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing."
	paste := strings.Repeat(strings.Join([]string{
		"[PROJECT TOPOLOGY]",
		"Languages: Go",
		"Stack: Go Modules",
		"Structure: Single Module",
	}, "\n")+"\n", 80)
	m.goalInput.SetValue(instruction)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != instruction {
		t.Fatalf("goal input = %q, want preserved instruction %q", got.goalInput.Value(), instruction)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text = %q, want pasted payload", got.goalAttachments[0].Text)
	}
}

func TestLargeGoalBurstWithoutPasteFlagStillBecomesAttachment(t *testing.T) {
	origByteThreshold := goalPasteAttachmentByteThreshold
	origLineThreshold := goalPasteAttachmentLineThreshold
	goalPasteAttachmentByteThreshold = 8
	goalPasteAttachmentLineThreshold = 30
	t.Cleanup(func() {
		goalPasteAttachmentByteThreshold = origByteThreshold
		goalPasteAttachmentLineThreshold = origLineThreshold
	})

	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Repeat("x", goalPasteAttachmentByteThreshold+1)

	var next tea.Model = m
	for _, r := range []rune(paste) {
		var ok bool
		next, _ = next.(Model).Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{r},
		})
		_, ok = next.(Model)
		if !ok {
			t.Fatalf("Update returned %T, want tui.Model", next)
		}
	}
	stale := next.(Model)
	stale.goalInputLastRuneAt = time.Now().Add(-goalPasteFlushDelay)
	next, _ = stale.Update(goalPasteFlushMsg{})
	got := next.(Model)
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text did not preserve split payload")
	}
	if got.goalInput.Value() != "" {
		t.Fatalf("goal input = %q, want empty inline editor after attachment conversion", got.goalInput.Value())
	}
}

func TestLargeGoalBurstPreservesTypedInstruction(t *testing.T) {
	origByteThreshold := goalPasteAttachmentByteThreshold
	origLineThreshold := goalPasteAttachmentLineThreshold
	goalPasteAttachmentByteThreshold = 8
	goalPasteAttachmentLineThreshold = 30
	t.Cleanup(func() {
		goalPasteAttachmentByteThreshold = origByteThreshold
		goalPasteAttachmentLineThreshold = origLineThreshold
	})

	instruction := "fix the login bug"
	paste := strings.Repeat("x", goalPasteAttachmentByteThreshold+1)
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue(instruction)

	var next tea.Model = m
	for _, r := range []rune(paste) {
		var ok bool
		next, _ = next.(Model).Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{r},
		})
		_, ok = next.(Model)
		if !ok {
			t.Fatalf("Update returned %T, want tui.Model", next)
		}
	}
	stale := next.(Model)
	stale.goalInputLastRuneAt = time.Now().Add(-goalPasteFlushDelay)
	next, _ = stale.Update(goalPasteFlushMsg{})
	got := next.(Model)
	if got.goalInput.Value() != instruction {
		t.Fatalf("goal input = %q, want preserved instruction %q", got.goalInput.Value(), instruction)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatalf("attachment text did not preserve split payload")
	}

	next, _ = got.submitGoal()
	submitted := next.(Model)
	if !strings.Contains(submitted.goal, instruction) {
		t.Fatalf("submitted goal missing instruction:\n%s", submitted.goal)
	}
	if !strings.Contains(submitted.goal, "Attached text:") || !strings.Contains(submitted.goal, paste) {
		t.Fatalf("submitted goal missing attachment:\n%s", submitted.goal)
	}
	if strings.Contains(submitted.goal, instruction+paste) {
		t.Fatalf("submitted goal contains inline pasted payload:\n%s", submitted.goal)
	}
}

func TestTypedInputDoesNotConvertToAttachment(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	input := strings.Repeat("x", 128)
	var next tea.Model = m
	for _, r := range input {
		current := next.(Model)
		current.goalInputLastRuneAt = time.Time{}
		var ok bool
		next, _ = current.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{r},
		})
		_, ok = next.(Model)
		if !ok {
			t.Fatalf("Update returned %T, want tui.Model", next)
		}
	}
	got := next.(Model)
	if len(got.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(got.goalAttachments))
	}
	if got.goalInput.Value() == "" {
		t.Fatal("typed input was not inserted into the goal editor")
	}
}

func TestHumanPacedTypingDoesNotEnterPasteCapture(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	input := "review the failing login flow"
	var next tea.Model = m
	for i, r := range input {
		current := next.(Model)
		if i > 0 {
			current.goalInputLastRuneAt = time.Now().Add(-100 * time.Millisecond)
		}
		var ok bool
		next, _ = current.Update(tea.KeyMsg{
			Type:  tea.KeyRunes,
			Runes: []rune{r},
		})
		_, ok = next.(Model)
		if !ok {
			t.Fatalf("Update returned %T, want tui.Model", next)
		}
	}
	got := next.(Model)
	if got.goalPasteCapture {
		t.Fatal("human-paced typing entered paste capture")
	}
	if len(got.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(got.goalAttachments))
	}
	if got.goalInput.Value() != input {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), input)
	}
}

func TestGoalPastePreservesDiffText(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	diff := strings.Join([]string{
		"Review this change for concrete bugs.",
		"",
		"diff --git a/internal/tui/tui.go b/internal/tui/tui.go",
		"@@ -1,3 +1,4 @@",
		" package tui",
		"+// added context",
		"-// old context",
	}, "\n")

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(diff),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != diff {
		t.Fatalf("diff goal was not preserved:\n got: %q\nwant: %q", got.goalInput.Value(), diff)
	}
}

func TestHomeEnterSubmitsMultilinePastedGoal(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	goal := "Review this diff:\n\n@@ -1 +1 @@\n-old\n+new"
	m.goalInput.SetValue(goal)

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateScanning {
		t.Fatalf("state = %v, want %v", got.state, stateScanning)
	}
	if got.goal != goal {
		t.Fatalf("goal = %q, want %q", got.goal, goal)
	}
	if strings.Contains(got.goalInput.Value(), goal+"\n") {
		t.Fatalf("Enter inserted newline instead of submitting: %q", got.goalInput.Value())
	}
	if cmd == nil {
		t.Fatal("Enter submit did not return scan command")
	}
}

func TestHomeAltEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this diff")
	m.goalInput.SetCursor(len("Review this diff"))

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if got.goalInput.Value() != "Review this diff\n" {
		t.Fatalf("goal input = %q, want newline insertion", got.goalInput.Value())
	}
	if got.goal != "" {
		t.Fatalf("goal = %q, want empty before submit", got.goal)
	}
	if cmd == nil {
		t.Fatal("Alt+Enter did not return a cursor blink command")
	}
}

func TestHomeViewRendersLargePastedGoalCompactly(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	goal := strings.Repeat(strings.Join([]string{
		"Review this diff:",
		"Index: docs/ui-spec.md",
		"===================================================================",
		"@@ -1 +1 @@",
	}, "\n")+"\n", 50)
	m.goalInput.SetValue(goal)

	view := m.viewHome()

	if !strings.Contains(view, "[Pasted 5KB, 200 lines]") {
		t.Fatalf("home view missing compact pasted label:\n%s", view)
	}
	for _, want := range []string{
		"  Review this diff:",
		"  Index: docs/ui-spec.md",
		"  ===================================================================",
		"Press Enter to submit.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{
		"Type a goal, paste a diff, or use /review, /design, /followup, or /badge, then press Enter.",
		"Commands: /help, /review, /design, /followup, /badge, /exit",
		"Tag files with @path/to/file, then press Tab.",
		"Preview:",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("home view unexpectedly contained %q:\n%s", unwanted, view)
		}
	}
	if strings.Contains(view, "@@ -1 +1 @@\nReview this diff:") {
		t.Fatalf("home view rendered large pasted goal instead of hiding it:\n%s", view)
	}
	if m.goalInput.Value() != goal {
		t.Fatal("compact rendering changed retained goal text")
	}
}

func TestHomeViewKeepsShortPastedGoalVisible(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	goal := strings.Join([]string{
		"Review this diff for concrete bugs.",
		"diff --git a/file.go b/file.go",
		"@@ -1 +1 @@",
		"-old",
		"+new",
	}, "\n")
	m.goalInput.SetValue(goal)

	view := m.viewHome()

	if strings.Contains(view, "[Pasted ") {
		t.Fatalf("home view compacted a short pasted goal:\n%s", view)
	}
	if !strings.Contains(view, "Review this diff for concrete bugs.") ||
		!strings.Contains(view, "diff --git a/file.go b/file.go") {
		t.Fatalf("home view did not render short pasted goal text:\n%s", view)
	}
}

func TestGoalPasteOverLimitWarnsWithActualAndRetainedSizes(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Repeat("x", workflow.LargePromptBytes+2048)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "" {
		t.Fatalf("goal input = %q, want empty inline editor after attachment conversion", got.goalInput.Value())
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Type != goalAttachmentText {
		t.Fatalf("attachment type = %q, want %q", got.goalAttachments[0].Type, goalAttachmentText)
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatal("large pasted payload was not preserved exactly")
	}
	if got.status.severity != messageNeutral {
		t.Fatalf("status severity = %v, want neutral", got.status.severity)
	}
}

func TestGoalPasteOverByteLimitTrimsAtUTF8Boundary(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	paste := strings.Repeat("世", workflow.LargePromptBytes)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(paste),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Text != paste {
		t.Fatal("large pasted UTF-8 payload was not preserved exactly")
	}
	if !utf8.ValidString(got.goalAttachments[0].Text) {
		t.Fatal("attachment text is not valid UTF-8")
	}
	if got.goalInput.Value() != "" {
		t.Fatalf("goal input = %q, want empty inline editor after attachment conversion", got.goalInput.Value())
	}
}

func TestWindowResizeClearsScreenBeforeRedraw(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	next, cmd := m.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.width != 100 || got.height != 32 {
		t.Fatalf("terminal size = %dx%d, want 100x32", got.width, got.height)
	}
	if got.goalInput.Height() != goalEditorMinHeight {
		t.Fatalf("goal input height = %d, want %d", got.goalInput.Height(), goalEditorMinHeight)
	}
	if cmd == nil {
		t.Fatal("resize did not request a screen clear")
	}
	if msg := cmd(); msg != tea.ClearScreen() {
		t.Fatalf("resize command = %T, want tea.ClearScreen", msg)
	}
}

func TestGoalEditorHeightAdaptsToContent(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		terminalHeight int
		editorWidth    int
		want           int
	}{
		{name: "empty", text: "", terminalHeight: 0, want: goalEditorMinHeight},
		{name: "one line", text: "review this", terminalHeight: 0, want: goalEditorMinHeight},
		{name: "multiple lines", text: "one\ntwo\nthree", terminalHeight: 0, want: 3},
		{name: "trailing blank line counts", text: "one\ntwo\n", terminalHeight: 0, want: 3},
		{name: "wrapped line counts visual rows", text: "1234567890123456789012345", terminalHeight: 0, editorWidth: 10, want: 3},
		{name: "wrapped lines combine with hard newlines", text: "123456789012345\n\nabc", terminalHeight: 0, editorWidth: 10, want: 4},
		{name: "long input caps at max", text: strings.Repeat("line\n", 20), terminalHeight: 0, want: goalEditorMaxHeight},
		{name: "terminal height does not change cap", text: strings.Repeat("line\n", 20), terminalHeight: 32, want: goalEditorMaxHeight},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := goalEditorHeight(tt.text, tt.terminalHeight, tt.editorWidth); got != tt.want {
				t.Fatalf("goalEditorHeight(%q, %d, %d) = %d, want %d", tt.text, tt.terminalHeight, tt.editorWidth, got, tt.want)
			}
		})
	}
}

func TestGoalEditorHeightGrowsForTypedMultilineGoal(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("Review this:\n- bug\n- regression"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}

	if got.goalInput.Height() != 3 {
		t.Fatalf("goal input height = %d, want 3", got.goalInput.Height())
	}
}

func TestGoalEditorHeightGrowsForWrappedGoalLine(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	next, _ := m.Update(tea.WindowSizeMsg{Width: 48, Height: 32})
	resized, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}

	next, _ = resized.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("Review this long instruction that wraps across several visible rows before the user adds another hard newline."),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}

	if got.goalInput.Height() <= goalEditorMinHeight {
		t.Fatalf("goal input height = %d, want growth for wrapped visual rows", got.goalInput.Height())
	}
}

func TestGoalEditorHeightGrowsForTrailingBlankLines(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("line one\nline two\n")
	m.resizeGoalEditor()

	if got := m.goalInput.Height(); got != 3 {
		t.Fatalf("goal input height = %d, want 3", got)
	}
}

func TestGoalInputByteLimitAppliesToNonPasteUTF8Input(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	input := strings.Repeat("世", workflow.LargePromptBytes)

	next, _ := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(input),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if len(got.goalInput.Value()) > workflow.LargePromptBytes {
		t.Fatalf("goal length = %d, want <= %d", len(got.goalInput.Value()), workflow.LargePromptBytes)
	}
	if !utf8.ValidString(got.goalInput.Value()) {
		t.Fatal("goal input is not valid UTF-8")
	}
	if !got.status.empty() {
		t.Fatalf("status = %#v, want empty", got.status)
	}
}

func TestGoalInputRetainsCommandLettersWhenNoCommandIsActive(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	expected := ""

	for _, letter := range []string{"c", "e", "f", "n", "p", "t", "y"} {
		expected += letter
		next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(letter)})
		got, ok := next.(Model)
		if !ok {
			t.Fatalf("handleKey returned %T, want tui.Model", next)
		}
		if got.state != stateHome {
			t.Fatalf("state = %v, want %v", got.state, stateHome)
		}
		if got.goalInput.Value() != expected {
			t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), expected)
		}
		m = got
	}
}

func TestSubmitGoalTreatsModelCommandAsGoal(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/model claude")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateScanning {
		t.Fatalf("state = %v, want %v", got.state, stateScanning)
	}
	if got.goal != "/model claude" {
		t.Fatalf("goal = %q, want /model claude", got.goal)
	}
	if cmd == nil {
		t.Fatal("model-like goal did not return scan command")
	}
}

func TestSubmitGoalExitCommandQuits(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/exit")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if cmd == nil {
		t.Fatal("exit command did not return quit command")
	}
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("exit command returned %T, want tea.QuitMsg", msg)
	}
}

func TestSubmitGoalHelpCommandShowsHelp(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/help")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateHelp {
		t.Fatalf("state = %v, want %v", got.state, stateHelp)
	}
	if cmd != nil {
		t.Fatal("help command returned unexpected command")
	}
	view := got.View()
	if !strings.Contains(view, "/exit") || !strings.Contains(view, "BYOL loop") {
		t.Fatalf("help view missing command reference:\n%s", view)
	}
	for _, want := range []string{
		"Tab            Complete / commands and @ files.",
		"@path/to/file",
		"/review   - Start review mode",
		"/followup - Start follow-up mode",
		"/badge    - Show GitHub stargazer scoreboard",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("help view missing %q:\n%s", want, view)
		}
	}
}

func TestSubmitGoalBadgeCommandShowsFlow(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/badge")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateBadgePermissionPrompt {
		t.Fatalf("state = %v, want %v", got.state, stateBadgePermissionPrompt)
	}
	if cmd == nil {
		t.Fatal("badge permission prompt returned nil command")
	}
	view := got.View()
	if !strings.Contains(view, "Fetch supporter scoreboard from GitHub? (y/N)") {
		t.Fatalf("badge permission view missing prompt:\n%s", view)
	}

	next, cmd, handled := got.handleKeyConfirm()
	if !handled {
		t.Fatal("handleKeyConfirm did not handle badge permission prompt")
	}
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKeyConfirm returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeFetching {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeFetching)
	}
	if cmd == nil {
		t.Fatal("badge fetch returned unexpected nil command")
	}
	if !strings.Contains(got.View(), "📡 Fetching...") {
		t.Fatalf("badge fetching view missing progress text:\n%s", got.View())
	}

	next, cmd = got.Update(badgeResultMsg{
		logins:    []string{"ada", "bob"},
		total:     2,
		gazillion: false,
	})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeResult {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeResult)
	}
	if cmd != nil {
		t.Fatal("badge result returned unexpected command")
	}
	view = got.View()
	for _, want := range []string{
		"⭐ TOTAL STARS: 2",
		"🌟 Recent supporters (last 10):",
		"@ada",
		"@bob",
		"[S]tar the repo in browser     [Enter] continue",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("badge result view missing %q:\n%s", want, view)
		}
	}

	next, cmd, handled = got.handleKeyEnter()
	if !handled {
		t.Fatal("handleKeyEnter did not handle badge result dismissal")
	}
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKeyEnter returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if cmd == nil {
		t.Fatal("badge return-home command returned unexpected nil command")
	}
}

func TestSubmitGoalBadgeCommandDeclineReturnsHome(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/badge")

	next, _ := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}

	next, cmd, handled := got.handleKeyCancel()
	if !handled {
		t.Fatal("handleKeyCancel did not handle badge permission prompt")
	}
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKeyCancel returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if cmd == nil {
		t.Fatal("badge decline returned unexpected nil command")
	}
	if view := got.View(); !strings.Contains(view, "👍 No problem!") {
		t.Fatalf("badge decline view missing confirmation:\n%s", view)
	}
}

func TestBadgeFetchCmdUsesStubbedFetcherForResult(t *testing.T) {
	originalFetch := fetchStargazersFunc
	fetchStargazersFunc = func() ([]string, int, error) {
		return []string{"ada", "bob"}, 2, nil
	}
	defer func() { fetchStargazersFunc = originalFetch }()

	msg, ok := badgeFetchCmd()().(badgeResultMsg)
	if !ok {
		t.Fatalf("badgeFetchCmd() returned %T, want badgeResultMsg", msg)
	}
	if msg.total != 2 || msg.gazillion {
		t.Fatalf("badgeResultMsg = %#v, want total 2 and gazillion false", msg)
	}

	m := NewModel("/tmp/project", DefaultConfig())
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeResult {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeResult)
	}
	if cmd != nil {
		t.Fatal("badge result returned unexpected command")
	}
	view := got.View()
	for _, want := range []string{
		"⭐ TOTAL STARS: 2",
		"@ada",
		"@bob",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("badge result view missing %q:\n%s", want, view)
		}
	}
}

func TestBadgeFetchCmdUsesStubbedFetcherForGazillionResult(t *testing.T) {
	originalFetch := fetchStargazersFunc
	fetchStargazersFunc = func() ([]string, int, error) {
		logins := make([]string, 100)
		for i := 0; i < 100; i++ {
			logins[i] = fmt.Sprintf("user%d", i)
		}
		return logins, 100, nil
	}
	defer func() { fetchStargazersFunc = originalFetch }()

	msg, ok := badgeFetchCmd()().(badgeResultMsg)
	if !ok {
		t.Fatalf("badgeFetchCmd() returned %T, want badgeResultMsg", msg)
	}
	if msg.total != 100 || !msg.gazillion {
		t.Fatalf("badgeResultMsg = %#v, want total 100 and gazillion true", msg)
	}

	m := NewModel("/tmp/project", DefaultConfig())
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeResult {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeResult)
	}
	if cmd != nil {
		t.Fatal("badge result returned unexpected command")
	}
	view := got.View()
	for _, want := range []string{
		"🦡🦡🦡 A GAZILLION BADGERS have starred this repo!",
		"🌟 Recent supporters (last 10):",
		"@user99",
		"[S]tar the repo in browser     [Enter] continue",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("gazillion result view missing %q:\n%s", want, view)
		}
	}
}

func TestBadgeFetchCmdUsesStubbedFetcherForNetworkError(t *testing.T) {
	originalFetch := fetchStargazersFunc
	fetchStargazersFunc = func() ([]string, int, error) {
		return nil, 0, errors.New("Could not fetch data: timeout")
	}
	defer func() { fetchStargazersFunc = originalFetch }()

	msg, ok := badgeFetchCmd()().(badgeErrorMsg)
	if !ok {
		t.Fatalf("badgeFetchCmd() returned %T, want badgeErrorMsg", msg)
	}
	if msg.text != "❌ Could not fetch data: timeout" {
		t.Fatalf("badgeErrorMsg.text = %q, want network error text", msg.text)
	}

	m := NewModel("/tmp/project", DefaultConfig())
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeError {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeError)
	}
	if cmd != nil {
		t.Fatal("badge error returned unexpected command")
	}
	if view := got.View(); !strings.Contains(view, "❌ Could not fetch data: timeout") {
		t.Fatalf("badge error view missing network error:\n%s", view)
	}
}

func TestBadgeFetchCmdUsesStubbedFetcherForRateLimitError(t *testing.T) {
	originalFetch := fetchStargazersFunc
	fetchStargazersFunc = func() ([]string, int, error) {
		return nil, 0, github.ErrRateLimit
	}
	defer func() { fetchStargazersFunc = originalFetch }()

	msg, ok := badgeFetchCmd()().(badgeErrorMsg)
	if !ok {
		t.Fatalf("badgeFetchCmd() returned %T, want badgeErrorMsg", msg)
	}
	if msg.text != "⚠️ GitHub API rate limit hit. Try again in an hour." {
		t.Fatalf("badgeErrorMsg.text = %q, want rate-limit text", msg.text)
	}

	m := NewModel("/tmp/project", DefaultConfig())
	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateBadgeError {
		t.Fatalf("state = %v, want %v", got.state, stateBadgeError)
	}
	if cmd != nil {
		t.Fatal("badge rate-limit error returned unexpected command")
	}
	if view := got.View(); !strings.Contains(view, "⚠️ GitHub API rate limit hit. Try again in an hour.") {
		t.Fatalf("badge error view missing rate-limit error:\n%s", view)
	}
}

func TestSubmitGoalReviewCommandUsesPreparedPrompt(t *testing.T) {
	repo := newReviewRepo(t, "println(\"updated\")")
	m := NewModel(repo, DefaultConfig())
	m.goalInput.SetValue("/review")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if got.cfg.Focus != protocol.FocusReview {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusReview)
	}
	if got.goalInput.Value() == "" {
		t.Fatal("goal input is empty")
	}
	if strings.Contains(got.goalInput.Value(), "/review") {
		t.Fatalf("goal input still contains command text: %q", got.goalInput.Value())
	}
	if strings.Contains(got.goalInput.Value(), "Diff:") {
		t.Fatalf("goal input unexpectedly contains diff body:\n%s", got.goalInput.Value())
	}
	if !strings.Contains(got.goalInput.Value(), "Review the following change for concrete bugs") {
		t.Fatalf("goal input missing review instruction:\n%s", got.goalInput.Value())
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.goalAttachments[0].Type != goalAttachmentGitDiff {
		t.Fatalf("goal attachment type = %q, want %q", got.goalAttachments[0].Type, goalAttachmentGitDiff)
	}
	if !strings.Contains(got.goalAttachments[0].Text, "println(\"updated\")") {
		t.Fatalf("goal attachment missing diff content:\n%s", got.goalAttachments[0].Text)
	}
	if !strings.Contains(got.status.text, "Loaded review prompt from the current git diff") {
		t.Fatalf("status = %q, want success review status", got.status.text)
	}
	if cmd == nil {
		t.Fatal("review command returned nil blink command")
	}
}

func TestSubmitGoalReviewCommandUsesExtraFocusText(t *testing.T) {
	repo := newReviewRepo(t, "println(\"branch\")")
	m := NewModel(repo, DefaultConfig())
	m.goalInput.SetValue("/review Check error handling and nil guards.")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if !strings.Contains(got.goalInput.Value(), "Additional focus:") {
		t.Fatalf("goal input missing additional focus heading:\n%s", got.goalInput.Value())
	}
	if !strings.Contains(got.goalInput.Value(), "Check error handling and nil guards.") {
		t.Fatalf("goal input missing extra focus text:\n%s", got.goalInput.Value())
	}
	if cmd == nil {
		t.Fatal("review command returned nil blink command")
	}
}

func TestSubmitGoalReviewCommandUsesFallbackPromptWhenNoDiff(t *testing.T) {
	repo := newReviewRepo(t, "")
	m := NewModel(repo, DefaultConfig())
	m.goalInput.SetValue("/review")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if len(got.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(got.goalAttachments))
	}
	if !strings.Contains(got.goalInput.Value(), "Paste the diff below or replace this text with the change you want reviewed.") {
		t.Fatalf("goal input missing manual fallback text:\n%s", got.goalInput.Value())
	}
	if !strings.Contains(got.status.text, "No git diff was detected") {
		t.Fatalf("status = %q, want no-diff warning", got.status.text)
	}
	if cmd == nil {
		t.Fatal("review command returned nil blink command")
	}
}

func TestSubmitGoalDesignCommandSwitchesFocus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/design")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusDesign {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusDesign)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if got.goalInput.Value() != protocol.DefaultDesignPrompt {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), protocol.DefaultDesignPrompt)
	}
	if !strings.Contains(got.status.text, "Focus set to Design.") {
		t.Fatalf("status = %q, want focus confirmation", got.status.text)
	}
	if cmd == nil {
		t.Fatal("design command returned unexpected nil command")
	}
}

func TestSubmitGoalFollowupCommandSwitchesFocus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.setGoalAttachments([]goalAttachment{newGoalTextAttachment("pasted text", "extra context")})
	m.goalInput.SetValue("/followup")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusFollowup {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusFollowup)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if got.goalInput.Value() != protocol.DefaultFollowupPrompt {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), protocol.DefaultFollowupPrompt)
	}
	if len(got.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(got.goalAttachments))
	}
	if !got.goalInput.Focused() {
		t.Fatal("goal input is not focused")
	}
	if !strings.Contains(got.status.text, "Focus set to Follow-up.") {
		t.Fatalf("status = %q, want focus confirmation", got.status.text)
	}
	if cmd == nil {
		t.Fatal("follow-up command returned unexpected nil command")
	}
}

func TestViewShowsPersistentHeader(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()

	view := m.View()

	if !strings.Contains(view, "🦡 AIBADGER "+version.Version) {
		t.Fatalf("home view missing compact version header:\n%s", view)
	}
	if !strings.Contains(view, "Local-first code context for any AI chat") {
		t.Fatalf("home view missing descriptor header:\n%s", view)
	}
	if !strings.Contains(view, "Focus: Code") {
		t.Fatalf("home view missing active focus indicator:\n%s", view)
	}
	if !strings.Contains(view, "Pipeline: [Map]"+symbols.pipelineSep+"Extract"+symbols.pipelineSep+"Apply") {
		t.Fatalf("home view missing pipeline indicator:\n%s", view)
	}
	if !strings.Contains(view, " /\\_/\\") || !strings.Contains(view, "( o.o )") || !strings.Contains(view, " > ^ <") {
		t.Fatalf("home view missing compact mascot header:\n%s", view)
	}
}

func TestHomeViewSurfacesReviewUseCase(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	view := m.View()

	if !strings.Contains(view, "Type a goal, paste a diff, or use /review, /design, /followup, or /badge, then press Enter.") {
		t.Fatalf("home view missing review goal guidance:\n%s", view)
	}
	if !strings.Contains(view, "Commands: /help, /review, /design, /followup, /badge, /exit") {
		t.Fatalf("home view missing review command:\n%s", view)
	}
	for _, want := range []string{
		"@path/to/file",
		"Tag files with @path/to/file, then press Tab.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view missing tagged-file guidance %q:\n%s", want, view)
		}
	}
}

func TestHomeViewShowsSlashCommandSuggestions(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/")
	m.refreshCompletionCandidate()

	view := m.View()

	for _, want := range []string{
		"/help",
		"Show commands and keyboard shortcuts.",
		"/review",
		"Seed an editable review prompt from the current git diff.",
		"/design",
		"Switch the active focus to Design.",
		"/followup",
		"Switch the active focus to Follow-up.",
		"/badge",
		"Show GitHub stargazer scoreboard",
		"/exit",
		"Quit Badger.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view missing slash suggestion %q:\n%s", want, view)
		}
	}
}

func TestSlashCommandSuggestionsFilterByPrefix(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/r")
	m.refreshCompletionCandidate()

	suggestions := m.viewSlashCommandSuggestions()

	if !strings.Contains(suggestions, "/review") {
		t.Fatalf("filtered suggestions missing /review:\n%s", suggestions)
	}
	for _, unwanted := range []string{"/help", "/exit"} {
		if strings.Contains(suggestions, unwanted) {
			t.Fatalf("filtered suggestions unexpectedly included %q:\n%s", unwanted, suggestions)
		}
	}
}

func TestSlashCommandSuggestionsFilterFollowupByPrefix(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/f")
	m.refreshCompletionCandidate()

	suggestions := m.viewSlashCommandSuggestions()

	if !strings.Contains(suggestions, "/followup") {
		t.Fatalf("filtered suggestions missing /followup:\n%s", suggestions)
	}
	for _, unwanted := range []string{"/help", "/review", "/exit"} {
		if strings.Contains(suggestions, unwanted) {
			t.Fatalf("filtered suggestions unexpectedly included %q:\n%s", unwanted, suggestions)
		}
	}
}

func TestSlashCommandSuggestionsDoNotIncludeReservedModelCommand(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/m")
	m.refreshCompletionCandidate()

	suggestions := m.viewSlashCommandSuggestions()

	if suggestions != "" {
		t.Fatalf("reserved /model command should not be suggested:\n%s", suggestions)
	}
}

func TestSlashCommandSuggestionsHiddenForNormalInput(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("review my change")
	m.refreshCompletionCandidate()

	view := m.View()

	if strings.Contains(view, "Show commands and keyboard shortcuts.") {
		t.Fatalf("home view should not show slash suggestions for normal input:\n%s", view)
	}
	if suggestions := m.viewSlashCommandSuggestions(); suggestions != "" {
		t.Fatalf("normal input suggestions = %q, want empty", suggestions)
	}
}

func TestHomeTabCompletesFirstSlashCommandSuggestion(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/")
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "/help" {
		t.Fatalf("goal input = %q, want /help", got.goalInput.Value())
	}
	if cmd != nil {
		t.Fatal("tab completion returned unexpected command")
	}
}

func TestDesignCompletionTabThenEnterSwitchesFocus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	m.goalInput.SetValue("/desi")
	m.refreshCompletionCandidate()

	// Tab completes "/desi" to "/design" and immediately triggers the action
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusDesign {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusDesign)
	}
	if !strings.Contains(got.status.text, "Focus set to Design.") {
		t.Fatalf("status = %q, want focus confirmation", got.status.text)
	}
	if got.goalInput.Value() != protocol.DefaultDesignPrompt {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), protocol.DefaultDesignPrompt)
	}
	if cmd == nil {
		t.Fatal("design action returned unexpected nil command")
	}
}

func TestDesignCompletionEnterThenEnterSwitchesFocus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	m.goalInput.SetValue("/desi")
	m.refreshCompletionCandidate()

	// Enter selects the "/design" completion and immediately triggers the action
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusDesign {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusDesign)
	}
	if !strings.Contains(got.status.text, "Focus set to Design.") {
		t.Fatalf("status = %q, want focus confirmation", got.status.text)
	}
	if got.goalInput.Value() != protocol.DefaultDesignPrompt {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), protocol.DefaultDesignPrompt)
	}
	if cmd == nil {
		t.Fatal("design action returned unexpected nil command")
	}
}

func TestFollowupCompletionTabThenEnterSwitchesFocus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	m.goalInput.SetValue("/fol")
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusFollowup {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusFollowup)
	}
	if !strings.Contains(got.status.text, "Focus set to Follow-up.") {
		t.Fatalf("status = %q, want focus confirmation", got.status.text)
	}
	if got.goalInput.Value() != protocol.DefaultFollowupPrompt {
		t.Fatalf("goal input = %q, want %q", got.goalInput.Value(), protocol.DefaultFollowupPrompt)
	}
	if cmd == nil {
		t.Fatal("follow-up action returned unexpected nil command")
	}
}

func TestBadgeCompletionEnterSwitchesToBadgeFlow(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	m.goalInput.SetValue("/ba")
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateBadgePermissionPrompt {
		t.Fatalf("state = %v, want %v", got.state, stateBadgePermissionPrompt)
	}
	if got.goalInput.Value() != "/badge" {
		t.Fatalf("goal input = %q, want /badge", got.goalInput.Value())
	}
	if cmd == nil {
		t.Fatal("badge action returned unexpected nil command")
	}
}

func TestHomeTabCompletesReviewSuggestionAndStartsReview(t *testing.T) {
	repo := newReviewRepo(t, "println(\"updated\")")
	m := NewModel(repo, DefaultConfig())
	m.goalInput.SetValue("/r")
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusReview {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusReview)
	}
	if strings.Contains(got.goalInput.Value(), "Diff:") {
		t.Fatalf("goal input unexpectedly contains diff body:\n%s", got.goalInput.Value())
	}
	if !strings.Contains(got.goalInput.Value(), "Review the following change for concrete bugs") {
		t.Fatalf("goal input missing review instruction:\n%s", got.goalInput.Value())
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if !strings.Contains(got.status.text, "Loaded review prompt from the current git diff") {
		t.Fatalf("status = %q, want review success", got.status.text)
	}
	if cmd == nil {
		t.Fatal("review completion returned nil command")
	}
}

func TestHomeTabDoesNotCompleteReservedModelCommand(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/m")
	m.refreshCompletionCandidate()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "/m" {
		t.Fatalf("goal input = %q, want /m", got.goalInput.Value())
	}
}

func TestHomeTabLeavesNormalInputToTextarea(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("review")
	m.refreshCompletionCandidate()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "review" {
		t.Fatalf("goal input = %q, want review", got.goalInput.Value())
	}
}

func TestSelectedFocusSurvivesMapToExtractFlow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusReview
	m := NewModel("/tmp/project", cfg)
	m.goalInput.SetValue("implement safer copy handling")

	next, _ := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}

	scanned := engine.FromTopology("/tmp/project", &model.ProjectTopology{Name: "project"})
	next, _ = got.Update(scanDoneMsg{eng: scanned})
	afterScan, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if afterScan.cfg.Focus != protocol.FocusReview {
		t.Fatalf("Focus = %q, want %q", afterScan.cfg.Focus, protocol.FocusReview)
	}
	view := afterScan.View()
	symbols := testDisplaySymbols()
	if !strings.Contains(view, "Focus: Review") {
		t.Fatalf("scan flow view missing active focus:\n%s", view)
	}
	if !strings.Contains(view, "Pipeline: [Map]"+symbols.pipelineSep+"Extract"+symbols.pipelineSep+"Apply") {
		t.Fatalf("scan flow view missing Apply pipeline label:\n%s", view)
	}
}

func TestDesignFocusShowsFocusDesignAndApplyPipeline(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusDesign
	m := NewModel("/tmp/project", cfg)
	m.goalInput.SetValue("design new architecture")

	next, _ := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}

	scanned := engine.FromTopology("/tmp/project", &model.ProjectTopology{Name: "project"})
	next, _ = got.Update(scanDoneMsg{eng: scanned})
	afterScan, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if afterScan.cfg.Focus != protocol.FocusDesign {
		t.Fatalf("Focus = %q, want %q", afterScan.cfg.Focus, protocol.FocusDesign)
	}
	view := afterScan.View()
	symbols := testDisplaySymbols()
	if !strings.Contains(view, "Focus: Design") {
		t.Fatalf("scan flow view missing Focus: Design:\n%s", view)
	}
	if !strings.Contains(view, "Pipeline: [Map]"+symbols.pipelineSep+"Extract"+symbols.pipelineSep+"Apply") {
		t.Fatalf("scan flow view missing Apply pipeline label:\n%s", view)
	}
}

func TestFollowupFocusShowsFocusFollowupAndApplyPipeline(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusFollowup
	m := NewModel("/tmp/project", cfg)
	m.goalInput.SetValue("add context to existing chat")

	next, _ := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}

	scanned := engine.FromTopology("/tmp/project", &model.ProjectTopology{Name: "project"})
	next, _ = got.Update(scanDoneMsg{eng: scanned})
	afterScan, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if afterScan.cfg.Focus != protocol.FocusFollowup {
		t.Fatalf("Focus = %q, want %q", afterScan.cfg.Focus, protocol.FocusFollowup)
	}
	view := afterScan.View()
	symbols := testDisplaySymbols()
	if !strings.Contains(view, "Focus: Follow-up") {
		t.Fatalf("scan flow view missing Focus: Follow-up:\n%s", view)
	}
	if !strings.Contains(view, "Pipeline: [Map]"+symbols.pipelineSep+"Extract"+symbols.pipelineSep+"Apply") {
		t.Fatalf("scan flow view missing Apply pipeline label:\n%s", view)
	}
}

func TestDesignFocusPrompt2CodeContextInContextReadyDialog(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusDesign
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = "design context payload"
	m.metadata = []protocol.ExtractionMetadata{
		{Path: "internal/models/order.go"},
	}

	view := m.viewContextReady()

	for _, want := range []string{
		"Prompt 2: Code Context",
		"Copy Prompt 2: Code Context to clipboard",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("context-ready view missing %q:\n%s", want, view)
		}
	}
}

func TestContextReadyDialogUsesResolvedExternalDisplayPath(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextReady
	m.schemaB = "external context payload"
	m.metadata = []protocol.ExtractionMetadata{
		{Path: "../badger-sidecar/docs/spec.md"},
	}

	view := m.viewContextReady()

	if !strings.Contains(view, "  - ../badger-sidecar/docs/spec.md") {
		t.Fatalf("context-ready view missing resolved external display path:\n%s", view)
	}
	if strings.Contains(view, "  - spec.md") {
		t.Fatalf("context-ready view used shorthand external selector:\n%s", view)
	}
}

func TestDesignFocusPrompt2CodeContextInCopySuccess(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusDesign
	m := NewModel("/tmp/project", cfg)

	next, cmd := m.Update(copyDoneMsg{
		kind: workflow.PromptTwoKind(cfg.Focus),
		text: "design context",
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd == nil {
		t.Fatal("copy completion did not return textarea blink command")
	}
	if got.status.severity != messageSuccess {
		t.Fatalf("status severity = %v, want success", got.status.severity)
	}
	if got.status.text != "Prompt 2: Code Context copied." {
		t.Fatalf("status text = %q, want Prompt 2: Code Context success message", got.status.text)
	}
}

func TestDesignFocusPrompt2CodeContextInCancel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusDesign
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = "design context"

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.status.text != "Prompt 2: Code Context was not copied." {
		t.Fatalf("status = %q, want Prompt 2: Code Context cancel message", got.status.text)
	}
	if cmd == nil {
		t.Fatal("cancel did not return textarea blink command")
	}
}

func TestDesignFocusPrompt2CodeContextInHelpView(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Focus = protocol.FocusDesign
	m := NewModel("/tmp/project", cfg)
	m.state = stateHelp

	view := m.View()

	if !strings.Contains(view, "Confirm copying Prompt 2: Code Context") {
		t.Fatalf("help view missing Prompt 2: Code Context for Design focus:\n%s", view)
	}
}

func TestContextReadyStatusUsesFocusAwareMessage(t *testing.T) {
	wantStatus := workflow.ContextReadyStatus(protocol.FocusCode)
	for _, focus := range []protocol.Focus{protocol.FocusCode, protocol.FocusReview, protocol.FocusDesign} {
		t.Run("focus_"+focus.String(), func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Focus = focus
			m := NewModel("/tmp/project", cfg)
			m.state = stateWaitingForExtractions

			next, _ := m.Update(contextDoneMsg{
				schema: "context payload",
				metadata: []protocol.ExtractionMetadata{
					{Path: "internal/example.go"},
				},
				extractedCount: 1,
			})
			got, ok := next.(Model)
			if !ok {
				t.Fatalf("Update returned %T, want tui.Model", next)
			}
			if got.state != stateContextReady {
				t.Fatalf("state = %v, want %v", got.state, stateContextReady)
			}
			if got.status.text != wantStatus {
				t.Fatalf("status = %q, want %q", got.status.text, wantStatus)
			}
			if strings.Contains(got.status.text, "Respond context ready") {
				t.Fatalf("status unexpectedly contained %q: %q", "Respond context ready", got.status.text)
			}
		})
	}
}

func TestTaggedCompletionActivatesAtExpectedBoundaries(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	writeTaggedCompletionFixture(t, root, "docs/beta.go")

	tests := []struct {
		name       string
		input      string
		wantActive bool
	}{
		{
			name:       "start of input",
			input:      "@docs/a",
			wantActive: true,
		},
		{
			name:       "after whitespace",
			input:      "fix @docs/a",
			wantActive: true,
		},
		{
			name:       "after punctuation",
			input:      "fix, @docs/a",
			wantActive: true,
		},
		{
			name:       "after completed token",
			input:      "@docs/alpha.go ",
			wantActive: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(root, DefaultConfig())
			m.goalInput.SetValue(tt.input)
			m.goalInput.SetCursor(len(tt.input))
			m.refreshCompletionCandidate()

			candidate, ok := m.completionVisible()
			if ok != tt.wantActive {
				t.Fatalf("completionVisible() ok = %v, want %v", ok, tt.wantActive)
			}
			if !ok {
				return
			}
			if candidate.kind != completionKindTagged {
				t.Fatalf("completion kind = %v, want tagged", candidate.kind)
			}
		})
	}
}

func TestTaggedCompletionSkipsNoisyDirectories(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "notes.md")
	writeTaggedCompletionFixture(t, root, "node_modules/pkg/index.js")
	writeTaggedCompletionFixture(t, root, ".git/config")

	m := NewModel(root, DefaultConfig())
	m.goalInput.SetValue("@n")
	m.goalInput.SetCursor(len("@n"))
	m.refreshCompletionCandidate()

	candidate, ok := m.completionVisible()
	if !ok {
		t.Fatal("completionVisible() = false, want tagged completion")
	}

	for _, suggestion := range candidate.suggestions {
		if suggestion.replacement == "@node_modules/" || suggestion.replacement == "@\"node_modules/\"" {
			t.Fatalf("completion suggestions included noisy directory: %+v", candidate.suggestions)
		}
	}
}

func TestCompletionNavigationUsesActiveSuggestion(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	writeTaggedCompletionFixture(t, root, "docs/beta.go")

	m := NewModel(root, DefaultConfig())
	m.goalInput.SetValue("Review @docs/")
	m.goalInput.SetCursor(len("Review @docs/"))
	m.refreshCompletionCandidate()

	candidate, ok := m.completionVisible()
	if !ok {
		t.Fatal("completionVisible() = false, want tagged completion")
	}
	if got := m.completion.activeIndex; got != 0 {
		t.Fatalf("activeIndex = %d, want 0", got)
	}
	if got := candidate.suggestions[0].replacement; got != "@docs/alpha.go" {
		t.Fatalf("first suggestion = %q, want @docs/alpha.go", got)
	}
	lastIndex := len(candidate.suggestions) - 1
	wantLastReplacement := candidate.suggestions[lastIndex].replacement

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "Review @docs/" {
		t.Fatalf("goal input changed during navigation: %q", got.goalInput.Value())
	}
	if got.completion.activeIndex != 0 {
		t.Fatalf("activeIndex after up = %d, want 0", got.completion.activeIndex)
	}
	if cmd != nil {
		t.Fatal("up navigation returned unexpected command")
	}

	for i := 0; i < len(candidate.suggestions)+2; i++ {
		next, cmd = got.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		got, ok = next.(Model)
		if !ok {
			t.Fatalf("handleKey returned %T, want tui.Model", next)
		}
		if got.goalInput.Value() != "Review @docs/" {
			t.Fatalf("goal input changed during clamped navigation: %q", got.goalInput.Value())
		}
		if cmd != nil {
			t.Fatal("down navigation returned unexpected command")
		}
	}
	if got.completion.activeIndex != lastIndex {
		t.Fatalf("activeIndex after repeated down = %d, want %d", got.completion.activeIndex, lastIndex)
	}

	next, cmd = got.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "Review "+wantLastReplacement {
		t.Fatalf("goal input = %q, want active suggestion to insert", got.goalInput.Value())
	}
	if cmd != nil {
		t.Fatal("completion insertion returned unexpected command")
	}
}

func TestCompletionResetsActiveSelectionWhenSuggestionsChange(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	writeTaggedCompletionFixture(t, root, "docs/beta.go")
	writeTaggedCompletionFixture(t, root, "docs/gamma.go")

	m := NewModel(root, DefaultConfig())
	m.goalInput.SetValue("Review @docs/")
	m.goalInput.SetCursor(len("Review @docs/"))
	m.refreshCompletionCandidate()
	m.completion.activeIndex = 1

	m.goalInput.SetValue("Review @docs/g")
	m.goalInput.SetCursor(len("Review @docs/g"))
	m.refreshCompletionCandidate()

	if got := m.completion.activeIndex; got != 0 {
		t.Fatalf("activeIndex after refresh = %d, want 0", got)
	}
	candidate, ok := m.completionVisible()
	if !ok {
		t.Fatal("completionVisible() = false, want tagged completion")
	}
	if got := candidate.suggestions[0].replacement; got != "@docs/gamma.go" {
		t.Fatalf("first suggestion after filtering = %q, want @docs/gamma.go", got)
	}
	wantLine := renderBold(fmt.Sprintf("  %-12s %s", candidate.suggestions[0].label, candidate.suggestions[0].description))
	if view := m.viewCompletionSuggestions(); !strings.Contains(view, wantLine) {
		t.Fatalf("completion view missing bold active suggestion after refresh:\n%s", view)
	}
}

func TestCompletionPreservesSelectionAcrossNonKeyUpdates(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	writeTaggedCompletionFixture(t, root, "docs/beta.go")

	m := NewModel(root, DefaultConfig())
	m.goalInput.SetValue("Review @docs/")
	m.goalInput.SetCursor(len("Review @docs/"))
	m.refreshCompletionCandidate()
	m.completion.activeIndex = 1

	next, cmd := m.Update(struct{}{})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.completion.activeIndex != 1 {
		t.Fatalf("activeIndex after non-key update = %d, want 1", got.completion.activeIndex)
	}
	if got.goalInput.Value() != m.goalInput.Value() {
		t.Fatalf("goal input changed on non-key update: %q", got.goalInput.Value())
	}
	if cmd != nil {
		t.Fatal("non-key update returned unexpected command")
	}
}

func TestCompletionRendersActiveSuggestionBold(t *testing.T) {
	t.Run("tagged", func(t *testing.T) {
		root := t.TempDir()
		writeTaggedCompletionFixture(t, root, "docs/alpha.go")
		writeTaggedCompletionFixture(t, root, "docs/beta.go")

		m := NewModel(root, DefaultConfig())
		m.goalInput.SetValue("Review @docs/")
		m.goalInput.SetCursor(len("Review @docs/"))
		m.refreshCompletionCandidate()
		m.completion.activeIndex = 1

		candidate, ok := m.completionVisible()
		if !ok {
			t.Fatal("completionVisible() = false, want tagged completion")
		}

		view := m.viewCompletionSuggestions()
		wantLine := renderBold(fmt.Sprintf("  %-12s %s", candidate.suggestions[1].label, candidate.suggestions[1].description))
		if !strings.Contains(view, wantLine) {
			t.Fatalf("tagged completion view missing bold active suggestion:\n%s", view)
		}
	})

	t.Run("slash", func(t *testing.T) {
		m := NewModel("/tmp/project", DefaultConfig())
		m.goalInput.SetValue("/")
		m.goalInput.SetCursor(len("/"))
		m.refreshCompletionCandidate()

		candidate, ok := m.completionVisible()
		if !ok {
			t.Fatal("completionVisible() = false, want slash completion")
		}
		if len(candidate.suggestions) < 2 {
			t.Fatalf("slash completion suggestions = %d, want at least 2", len(candidate.suggestions))
		}
		m.completion.activeIndex = 1

		view := m.viewSlashCommandSuggestions()
		wantLine := renderBold(fmt.Sprintf("  %-12s %s", candidate.suggestions[1].label, candidate.suggestions[1].description))
		if !strings.Contains(view, wantLine) {
			t.Fatalf("slash completion view missing bold active suggestion:\n%s", view)
		}
	})
}

func TestSlashCompletionNavigationSelectsNonFirstSuggestion(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/")
	m.goalInput.SetCursor(len("/"))
	m.refreshCompletionCandidate()

	candidate, ok := m.completionVisible()
	if !ok {
		t.Fatal("completionVisible() = false, want slash completion")
	}
	if len(candidate.suggestions) < 2 {
		t.Fatalf("slash completion suggestions = %d, want at least 2", len(candidate.suggestions))
	}

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.completion.activeIndex != 1 {
		t.Fatalf("activeIndex after down = %d, want 1", got.completion.activeIndex)
	}
	if cmd != nil {
		t.Fatal("down navigation returned unexpected command")
	}
}

func TestSlashCompletionEnterStartsReview(t *testing.T) {
	repo := newReviewRepo(t, "println(\"updated\")")
	m := NewModel(repo, DefaultConfig())
	m.goalInput.SetValue("/r")
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.cfg.Focus != protocol.FocusReview {
		t.Fatalf("Focus = %q, want %q", got.cfg.Focus, protocol.FocusReview)
	}
	if strings.Contains(got.goalInput.Value(), "Diff:") {
		t.Fatalf("goal input unexpectedly contains diff body:\n%s", got.goalInput.Value())
	}
	if !strings.Contains(got.goalInput.Value(), "Review the following change for concrete bugs") {
		t.Fatalf("goal input missing review instruction:\n%s", got.goalInput.Value())
	}
	if len(got.goalAttachments) != 1 {
		t.Fatalf("goalAttachments length = %d, want 1", len(got.goalAttachments))
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want home", got.state)
	}
	if !strings.Contains(got.status.text, "Loaded review prompt from the current git diff") {
		t.Fatalf("status = %q, want review success", got.status.text)
	}
	if cmd == nil {
		t.Fatal("completion enter returned nil command")
	}
}

func TestSlashCompletionHelpOnEnterAndTab(t *testing.T) {
	for _, tt := range []struct {
		name string
		key  tea.KeyType
	}{
		{name: "enter", key: tea.KeyEnter},
		{name: "tab", key: tea.KeyTab},
	} {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("/tmp/project", DefaultConfig())
			m.goalInput.SetValue("/h")
			m.refreshCompletionCandidate()

			next, cmd := m.handleKey(tea.KeyMsg{Type: tt.key})
			got, ok := next.(Model)
			if !ok {
				t.Fatalf("handleKey returned %T, want tui.Model", next)
			}
			if got.state != stateHelp {
				t.Fatalf("state = %v, want %v", got.state, stateHelp)
			}
			if cmd != nil {
				t.Fatal("completion returned unexpected command")
			}
		})
	}
}

func TestTaggedCompletionEnterAndTabInsertIntoActiveToken(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	writeTaggedCompletionFixture(t, root, "docs/beta.go")

	tests := []struct {
		name string
		key  tea.KeyType
	}{
		{name: "enter", key: tea.KeyEnter},
		{name: "tab", key: tea.KeyTab},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel(root, DefaultConfig())
			m.goalInput.SetValue("Keep @docs/al and go")
			m.goalInput.SetCursor(len("Keep @docs/al"))
			m.refreshCompletionCandidate()

			next, cmd := m.handleKey(tea.KeyMsg{Type: tt.key})
			got, ok := next.(Model)
			if !ok {
				t.Fatalf("handleKey returned %T, want tui.Model", next)
			}
			if got.goalInput.Value() != "Keep @docs/alpha.go and go" {
				t.Fatalf("goal input = %q, want completion to replace only the tagged token", got.goalInput.Value())
			}
			if got.state != stateHome {
				t.Fatalf("state = %v, want home", got.state)
			}
			if cmd != nil {
				t.Fatal("completion insertion returned unexpected command")
			}
		})
	}
}

func TestTaggedCompletionEscapeDismissesMenu(t *testing.T) {
	root := t.TempDir()
	writeTaggedCompletionFixture(t, root, "docs/alpha.go")
	m := NewModel(root, DefaultConfig())
	m.goalInput.SetValue("Review @docs/al")
	m.goalInput.SetCursor(len("Review @docs/al"))
	m.refreshCompletionCandidate()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.goalInput.Value() != "Review @docs/al" {
		t.Fatalf("goal input = %q, want unchanged input", got.goalInput.Value())
	}
	if suggestions := got.viewCompletionSuggestions(); suggestions != "" {
		t.Fatalf("completion suggestions still visible after escape:\n%s", suggestions)
	}
	if got.completion.suppressedKey == "" {
		t.Fatal("escape should suppress the current completion token")
	}
	if cmd != nil {
		t.Fatal("escape dismissal returned unexpected command")
	}
}

func writeTaggedCompletionFixture(t *testing.T, root, path string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestHomeViewShowsCompactMascotFrame(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	view := m.View()

	if strings.Contains(view, " /\\_/\\\n( o.o )") {
		t.Fatalf("home view should keep mascot in persistent header only:\n%s", view)
	}
}

func TestScanningViewShowsCompactMascotFrame(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanning

	view := m.View()

	if !strings.Contains(view, " /\\_/\\  Sniffing around...\n( o.o )") {
		t.Fatalf("scanning view missing compact mascot frame:\n%s", view)
	}
	if strings.Contains(view, "/     \\") {
		t.Fatalf("scanning view should not use full mascot frame:\n%s", view)
	}
}

func TestPipelineIndicatorTracksManualCodeContextCopy(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateManualCopy
	m.manualCopyKind = codeContextPromptKind
	symbols := testDisplaySymbols()

	view := m.View()

	if !strings.Contains(view, "Pipeline: "+symbols.success+" Map"+symbols.pipelineSep+"[Extract]"+symbols.pipelineSep+"Apply") {
		t.Fatalf("manual code context view missing active context pipeline:\n%s", view)
	}
}

func TestHelpEnterReturnsHome(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateHelp

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want %v", got.state, stateHome)
	}
	if cmd == nil {
		t.Fatal("returning home did not restart text input blink")
	}
}

func TestNeutralStatusRendersWithoutSeverityMarker(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateHelp
	symbols := testDisplaySymbols()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}

	view := got.View()
	if !strings.Contains(view, "Ready for a goal.") {
		t.Fatalf("neutral status text missing:\n%s", view)
	}
	for _, marker := range []string{symbols.success, symbols.warning, symbols.error} {
		if strings.Contains(view, marker+" Ready for a goal.") {
			t.Fatalf("neutral status rendered severity marker %q:\n%s", marker, view)
		}
	}
}

func TestBoldStylesAreScopedToStructuralUI(t *testing.T) {
	if !titleStyle.GetBold() {
		t.Fatal("titleStyle.GetBold() = false, want AIBADGER header bold")
	}
	if !structuralStyle.GetBold() {
		t.Fatal("structuralStyle.GetBold() = false, want structural labels bold")
	}
	if successMarkerStyle.GetBold() || warningMarkerStyle.GetBold() || errorMarkerStyle.GetBold() {
		t.Fatal("severity marker styles should not use bold")
	}
}

func TestSuccessStatusRendersMarkerOnly(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()

	next, _ := m.Update(copyDoneMsg{
		kind: topologyPromptKind,
		text: "payload",
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}

	view := got.View()
	if !strings.Contains(view, symbols.success) ||
		!strings.Contains(view, "Prompt 1: Topology copied. Paste it into any LLM chat interface, then paste extraction commands.") {
		t.Fatalf("success status missing marker or text:\n%s", view)
	}
	if got.status.text != "Prompt 1: Topology copied. Paste it into any LLM chat interface, then paste extraction commands." {
		t.Fatalf("stored status includes rendered decoration: %#v", got.status)
	}
}

func TestNewModelAllowsConfigToDisableContextTrimming(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextFileBytes = -1
	cfg.MaxTotalContextBytes = -1

	m := NewModel("/tmp/project", cfg)

	if m.cfg.MaxContextFileBytes != -1 {
		t.Fatalf("MaxContextFileBytes = %d, want disabled", m.cfg.MaxContextFileBytes)
	}
	if m.cfg.MaxTotalContextBytes != -1 {
		t.Fatalf("MaxTotalContextBytes = %d, want disabled", m.cfg.MaxTotalContextBytes)
	}
}

func TestNewModelZeroContextLimitsUseDefaults(t *testing.T) {
	m := NewModel("/tmp/project", Config{})

	if m.cfg.MaxContextFileBytes != workflow.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", m.cfg.MaxContextFileBytes, workflow.MaxContextFileBytes)
	}
	if m.cfg.MaxTotalContextBytes != workflow.MaxTotalContextBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d", m.cfg.MaxTotalContextBytes, workflow.MaxTotalContextBytes)
	}
}

func TestNewModelZeroLargePromptThresholdUsesDefault(t *testing.T) {
	m := NewModel("/tmp/project", Config{})

	if m.cfg.LargePromptByteThreshold != workflow.LargePromptBytes {
		t.Fatalf("LargePromptByteThreshold = %d, want %d", m.cfg.LargePromptByteThreshold, workflow.LargePromptBytes)
	}
}

func TestNewModelKeepsDefaultContextLimits(t *testing.T) {
	cfg := DefaultConfig()

	m := NewModel("/tmp/project", cfg)

	if m.cfg.MaxContextFileBytes != cfg.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", m.cfg.MaxContextFileBytes, cfg.MaxContextFileBytes)
	}
	if m.cfg.MaxTotalContextBytes != cfg.MaxTotalContextBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d", m.cfg.MaxTotalContextBytes, cfg.MaxTotalContextBytes)
	}
}

func TestSubmitFinalResponseShowsTextResponse(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForCode
	m.paste.SetValue("This is analysis, not a file update.")

	next, cmd := m.submitFinalResponse()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitFinalResponse returned %T, want tui.Model", next)
	}
	if got.state != stateTextResponse {
		t.Fatalf("state = %v, want %v", got.state, stateTextResponse)
	}
	if got.response != "This is analysis, not a file update." {
		t.Fatalf("response = %q", got.response)
	}
	if cmd != nil {
		t.Fatal("text response returned unexpected command")
	}
	if !strings.Contains(got.View(), "This is analysis, not a file update.") {
		t.Fatal("text response view does not include pasted response")
	}
}

func TestTextResponseKeepsInfoLineOutsideBox(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateTextResponse
	m.response = "This is analysis, not a file update."

	view := m.viewTextResponse()

	if !strings.Contains(view, "Info: No file updates found. AI provided a textual response.") {
		t.Fatalf("view missing info line:\n%s", view)
	}
	if strings.Contains(view, "│ Info: No file updates found. AI provided a textual response.") {
		t.Fatalf("info line was rendered inside the box:\n%s", view)
	}
	if !strings.Contains(view, "╭") || !strings.Contains(view, "This is analysis, not a file update.") {
		t.Fatalf("response body was not rendered in the boxed section:\n%s", view)
	}
}

func TestTextResponseWrapsLongLinesToTerminalWidth(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateTextResponse
	m.width = 64
	m.response = strings.Repeat("long analysis ", 20)

	view := m.viewTextResponse()

	assertMaxRenderedLineWidth(t, view, 64)
}

func TestTextResponsePreviewLineLimitUsesTerminalHeight(t *testing.T) {
	tests := []struct {
		name   string
		height int
		want   int
	}{
		{name: "unknown height", height: 0, want: 12},
		{name: "small height", height: 20, want: 12},
		{name: "larger height", height: 40, want: 25},
		{name: "very tall height", height: 100, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("/tmp/project", DefaultConfig())
			m.height = tt.height

			if got := m.textResponsePreviewLineLimit(); got != tt.want {
				t.Fatalf("textResponsePreviewLineLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestTextResponsePreviewUsesDynamicLimit(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateTextResponse
	m.height = 40
	m.response = numberedLines(30)

	view := m.viewTextResponse()

	if !strings.Contains(view, "line 25") {
		t.Fatalf("text response did not include dynamic preview limit line:\n%s", view)
	}
	if strings.Contains(view, "line 26") {
		t.Fatalf("text response included a line beyond the dynamic preview limit:\n%s", view)
	}
	if !strings.Contains(view, "... [5 more lines hidden] ...") {
		t.Fatalf("text response missing hidden-line count:\n%s", view)
	}
}

func TestSubmitFinalResponseShowsNotesAlongsideWritePreview(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForCode
	m.paste.SetValue("Deepseek notes\n--- File: internal/tui/example.txt ---\nfirst line\n--- End File ---\nMore context")

	next, cmd := m.submitFinalResponse()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitFinalResponse returned %T, want tui.Model", next)
	}
	if got.state != stateWritePreview {
		t.Fatalf("state = %v, want %v", got.state, stateWritePreview)
	}
	if got.response != "Deepseek notes\nMore context" {
		t.Fatalf("response = %q", got.response)
	}
	if !strings.Contains(got.viewWritePreview(), "AI notes included with this response:") {
		t.Fatalf("write preview missing notes heading:\n%s", got.viewWritePreview())
	}
	if !strings.Contains(got.viewWritePreview(), "Deepseek notes\nMore context") {
		t.Fatalf("write preview missing notes text:\n%s", got.viewWritePreview())
	}
	if cmd != nil {
		t.Fatal("mixed response returned unexpected command")
	}
}

func TestWritePreviewNotesUseDynamicPreviewLimit(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateWritePreview
	m.height = 40
	m.response = numberedLines(30)

	view := m.viewWritePreview()

	if !strings.Contains(view, "line 25") {
		t.Fatalf("write preview notes did not include dynamic preview limit line:\n%s", view)
	}
	if strings.Contains(view, "line 26") {
		t.Fatalf("write preview notes included a line beyond the dynamic preview limit:\n%s", view)
	}
	if !strings.Contains(view, "... [5 more lines hidden] ...") {
		t.Fatalf("write preview notes missing hidden-line count:\n%s", view)
	}
}

func TestExtractionPasteEnterSubmitsMultilineCommands(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForExtractions
	m.paste.SetValue("FILE:internal/tui/tui.go\nPREFIX:internal/tui/tui_test.go#Test")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if len(got.commands) != 2 {
		t.Fatalf("commands = %d, want 2", len(got.commands))
	}
	if got.commands[0].Path != "internal/tui/tui.go" ||
		got.commands[1].Path != "internal/tui/tui_test.go" ||
		got.commands[1].Pattern != "Test" {
		t.Fatalf("commands not preserved from multiline paste: %#v", got.commands)
	}
	if cmd == nil {
		t.Fatal("extraction submission did not return context command")
	}
}

func TestExtractionPasteWithoutCommandsShowsError(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForExtractions
	m.paste.SetValue("please inspect the TUI")
	symbols := testDisplaySymbols()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForExtractions {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForExtractions)
	}
	if !got.status.empty() {
		t.Fatalf("status = %#v, want empty error status", got.status)
	}
	if got.err == nil {
		t.Fatal("err = nil, want extraction guidance error")
	}
	if strings.Contains(got.err.Error(), symbols.error) {
		t.Fatalf("stored error includes rendered marker: %q", got.err.Error())
	}
	view := got.View()
	if !strings.Contains(view, symbols.error) ||
		!strings.Contains(view, "No extraction commands found. Paste FILE/PREFIX/NEAR commands and press Enter.") {
		t.Fatalf("view missing error marker or message:\n%s", got.View())
	}
	if cmd != nil {
		t.Fatal("invalid extraction submission returned unexpected command")
	}
}

func TestFinalResponsePasteEnterSubmitsMultilineWritePlan(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForCode
	m.paste.SetValue("--- File: internal/tui/example.txt ---\nfirst line\n  second line\n--- End File ---")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWritePreview {
		t.Fatalf("state = %v, want %v", got.state, stateWritePreview)
	}
	if len(got.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(got.updates))
	}
	if got.updates[0].Content != "first line\n  second line\n" {
		t.Fatalf("content = %q", got.updates[0].Content)
	}
	if cmd != nil {
		t.Fatal("final response submission returned unexpected command")
	}
}

func TestFinalResponsePasteEnterRejectsMalformedDeletePlan(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForCode
	m.paste.SetValue("--- Delete File: /tmp/escape.go ---")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.err == nil {
		t.Fatal("err = nil, want validation error")
	}
	if !strings.Contains(got.err.Error(), "absolute paths are not allowed") {
		t.Fatalf("err = %q, want absolute-path validation error", got.err.Error())
	}
	if cmd != nil {
		t.Fatal("malformed delete plan returned unexpected command")
	}
}

func TestExtractionPasteAutoSubmitsBracketedPaste(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForExtractions
	m.paste.Focus()

	next, cmd := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("FILE:internal/tui/tui.go\nPREFIX:internal/tui/tui_test.go#Test"),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if len(got.commands) != 2 {
		t.Fatalf("commands = %d, want 2", len(got.commands))
	}
	if got.commands[1].Pattern != "Test" {
		t.Fatalf("commands not preserved from pasted input: %#v", got.commands)
	}
	if cmd == nil {
		t.Fatal("bracketed extraction paste did not return context command")
	}
}

func TestFinalResponsePasteAutoSubmitsBracketedPaste(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.eng = engine.FromTopology("/tmp/project", nil)
	m.state = stateWaitingForCode
	m.paste.Focus()

	next, cmd := m.Update(tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune("--- File: internal/tui/example.txt ---\nfirst line\n  second line\n--- End File ---"),
		Paste: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWritePreview {
		t.Fatalf("state = %v, want %v", got.state, stateWritePreview)
	}
	if len(got.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(got.updates))
	}
	if got.updates[0].Content != "first line\n  second line\n" {
		t.Fatalf("content = %q", got.updates[0].Content)
	}
	if cmd != nil {
		t.Fatal("final response bracketed paste returned unexpected command")
	}
}

func TestPasteViewsUseAutoSubmitLabel(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	for _, st := range []state{stateWaitingForExtractions, stateWaitingForCode} {
		m.state = st
		view := m.View()
		if !strings.Contains(view, "paste submits, Enter fallback") {
			t.Fatalf("paste view for state %v missing paste auto-submit label:\n%s", st, view)
		}
		if strings.Contains(view, "ctrl+s") {
			t.Fatalf("paste view for state %v still mentions ctrl+s:\n%s", st, view)
		}
	}
}

func TestInputStateStatusLinesShowKeyboardHints(t *testing.T) {
	tests := []struct {
		name  string
		state state
	}{
		{name: "home", state: stateHome},
		{name: "extraction paste", state: stateWaitingForExtractions},
		{name: "final response paste", state: stateWaitingForCode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("/tmp/project", DefaultConfig())
			m.state = tt.state

			view := m.View()

			wants := []string{"Enter submit", "Ctrl+C quit"}
			if tt.state != stateHome {
				wants = append(wants, "Esc cancel")
			}
			for _, want := range wants {
				if !strings.Contains(view, want) {
					t.Fatalf("view missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestHelpDocumentsCtrlU(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateHelp

	view := m.View()

	if !strings.Contains(view, "Ctrl+U") || !strings.Contains(view, "Clear line") {
		t.Fatalf("help view missing Ctrl+U clear-line documentation:\n%s", view)
	}
	if !strings.Contains(view, "Alt+Enter") || !strings.Contains(view, "newline") {
		t.Fatalf("help view missing Alt+Enter newline documentation:\n%s", view)
	}
}

func TestHelpShowsVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BuildInfo = "Version: " + version.Version + " · Build: development · Dev flags: enabled"
	m := NewModel("/tmp/project", cfg)
	m.state = stateHelp

	view := m.View()

	if !strings.Contains(view, "Version: "+version.Version+" · Build: development · Dev flags: enabled") {
		t.Fatalf("help view missing build info:\n%s", view)
	}
}

func TestLargePasteRenderingIsCompact(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateWaitingForCode
	m.paste.SetValue(strings.Repeat("x", 22*1024))

	view := m.View()

	if !strings.Contains(view, "[Pasted 22KB] submitting automatically when paste is detected") {
		t.Fatalf("large paste view missing compact label:\n%s", view)
	}
	if strings.Contains(view, strings.Repeat("x", 120)) {
		t.Fatalf("large paste view leaked pasted content:\n%s", view)
	}
}

func TestCopyTopologyDialogShowsPayloadSize(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	m.state = stateScanComplete
	m.schemaA = "project map payload"
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Languages:       []string{"Go"},
		PrimaryLanguage: "Go",
		Modules: []model.Module{
			{Name: "internal/app", FileCount: 37, Heaviest: model.HeaviestFile{Name: "app.go", Size: 4096}},
		},
	})

	view := m.viewScanComplete()

	for _, want := range []string{
		"Scan complete! Here's what Badger found:",
		"─────────────────────────────────────────────────",
		"Main Modules:\n  - internal/app (37 files) -> Top: app.go (4KB)\n\nTotal: 37 source files across 1 modules",
		"Privacy: Structure only - no source code.",
		"You will pass this prompt to an AI chat.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("topology copy view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, symbols.warning) {
		t.Fatalf("topology privacy note should not use warning severity:\n%s", view)
	}
	if !strings.Contains(view, "Copy Prompt 1: Topology to clipboard (payload: 19B)? (y/N)") {
		t.Fatalf("topology copy prompt missing:\n%s", view)
	}
	if strings.Contains(view, "Clipboard payload:") {
		t.Fatalf("topology payload should be inline with prompt:\n%s", view)
	}
	if strings.Contains(view, "╭") || strings.Contains(view, "│") || strings.Contains(view, "╰") {
		t.Fatalf("topology copy prompt should not be rendered in a box:\n%s", view)
	}
}

func TestLargeTopologyPromptShowsDeliveryMenu(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	symbols := testDisplaySymbols()
	m.state = stateScanComplete
	m.schemaA = strings.Repeat("x", 2*1024)
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Modules: []model.Module{{Name: "internal/app", FileCount: 1}},
	})

	view := m.viewScanComplete()

	for _, want := range []string{
		symbols.warning,
		"Prompt 1: Topology is large (2KB).",
		"Recommended: save it to a temp file and attach/upload it to your AI chat.",
		"  [c] Copy anyway",
		"  [f] Save to temp file",
		"  [p] Print to terminal",
		"  [n] Cancel",
		"Choice (recommended: f):",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("large topology prompt view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Copy Prompt 1: Topology to clipboard") {
		t.Fatalf("large topology prompt used normal copy prompt:\n%s", view)
	}
}

func TestNormalTopologyPromptDoesNotShowLargeWarning(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanComplete
	m.schemaA = "small topology"
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Modules: []model.Module{{Name: "internal/app", FileCount: 1}},
	})

	view := m.viewScanComplete()

	if strings.Contains(view, "Prompt 1: Topology is large") {
		t.Fatalf("normal topology prompt showed large warning:\n%s", view)
	}
	if strings.Contains(view, "Save to temp file") || strings.Contains(view, "Copy anyway") {
		t.Fatalf("normal topology prompt showed large-prompt options:\n%s", view)
	}
}

func TestScanCompleteSurfacesTaggedFileWarnings(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"docs/usage.md",
		"docs/user-guide.md",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	goal := "review @docs/usage.md and @docs/missing.md and @docs/usage.md"
	eng := engine.FromTopology(root, &model.ProjectTopology{
		Languages: []string{"Go"},
		Modules:   []model.Module{{Name: "docs", FileCount: 2}},
	})
	m := NewModel(root, DefaultConfig())
	m.state = stateScanning
	m.goal = goal

	next, cmd := m.Update(scanDoneMsg{eng: eng})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("scan completion returned unexpected command")
	}
	if got.state != stateScanComplete {
		t.Fatalf("state = %v, want %v", got.state, stateScanComplete)
	}
	if got.status.severity != messageWarning {
		t.Fatalf("status severity = %v, want warning", got.status.severity)
	}
	if !strings.Contains(got.status.text, "docs/missing.md") {
		t.Fatalf("warning status missing unresolved tagged path:\n%s", got.status.text)
	}
	if !strings.Contains(got.schemaA, "[USER TAGGED FILES]") {
		t.Fatalf("Prompt 1 missing tagged-files section:\n%s", got.schemaA)
	}
	if strings.Count(got.schemaA, "FILE:docs/usage.md") != 1 {
		t.Fatalf("Prompt 1 should dedupe repeated tagged files:\n%s", got.schemaA)
	}
	if !strings.Contains(got.schemaA, "[TASK]\n"+goal+"\n\n[CONSTRAINT]") {
		t.Fatalf("Prompt 1 did not preserve the original goal text:\n%s", got.schemaA)
	}
}

func TestCopyCodeContextDialogShowsPayloadSize(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	m.state = stateContextReady
	m.schemaB = "context payload"
	m.metadata = []protocol.ExtractionMetadata{
		{Path: "internal/scanner/go.go"},
		{Path: "internal/models/order.go", Truncated: true},
		{Path: "internal/models/user.go", Dropped: true},
	}

	view := m.viewContextReady()

	for _, want := range []string{
		symbols.warning,
		"This WILL include the actual source code from:",
		"  - internal/scanner/go.go",
		"  - internal/models/order.go [TRUNCATED]",
		"  - internal/models/user.go [DROPPED - EXCEEDS TOTAL LIMIT]",
		"Note: Some files were truncated or dropped to fit context limits.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("code context view missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "Copy Prompt 2: Code Context to clipboard (payload: 15B)? (y/N)") {
		t.Fatalf("code context copy prompt missing:\n%s", view)
	}
	if strings.Contains(view, "Clipboard payload:") {
		t.Fatalf("code context payload should be inline with prompt:\n%s", view)
	}
}

func TestPartialExtractionWarningViewShowsFailedRequests(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextWarning
	m.pendingExtractedCount = 3
	m.pendingMetadata = []protocol.ExtractionMetadata{
		{Path: "pom.xml"},
		{Path: "src/main/java/App.java"},
		{Path: "src/test/java/AppTest.java"},
	}
	m.pendingFailedCommands = []string{".mvn/jvm.config: file not found: .mvn/jvm.config"}

	view := m.viewContextWarning()

	for _, want := range []string{
		"Extracted 3 files, but 1 request needs attention:",
		"Available context:",
		"  - pom.xml",
		"  - src/main/java/App.java",
		"  - src/test/java/AppTest.java",
		"Failed:",
		"  - .mvn/jvm.config: file not found: .mvn/jvm.config",
		"Proceed with available context? (y/N)",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("partial extraction warning missing %q:\n%s", want, view)
		}
	}
}

func TestPartialExtractionWarningViewShowsContextTrimmingStatus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextWarning
	m.pendingExtractedCount = 2
	m.pendingMetadata = []protocol.ExtractionMetadata{
		{Path: "large.go", Truncated: true},
		{Path: "overflow.go", Dropped: true},
	}
	m.pendingFailedCommands = []string{"missing.go: file not found: missing.go"}

	view := m.viewContextWarning()

	for _, want := range []string{
		"Available context:",
		"  - large.go [TRUNCATED]",
		"  - overflow.go [DROPPED - EXCEEDS TOTAL LIMIT]",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("partial extraction warning missing %q:\n%s", want, view)
		}
	}
}

func TestPartialExtractionWarningViewShowsSafetyExclusions(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextWarning
	m.pendingExtractedCount = 1
	m.pendingMetadata = []protocol.ExtractionMetadata{{Path: "app.env.example"}}
	m.pendingSafetyExclusions = []string{".env: excluded from Prompt 2"}

	view := m.viewContextWarning()

	for _, want := range []string{
		"Extracted 1 file, but 1 request needs attention:",
		"Available context:",
		"  - app.env.example",
		"Excluded by Prompt 2 safety rules:",
		"  - .env: excluded from Prompt 2",
		"Proceed with available context? (y/N)",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("partial extraction warning missing %q:\n%s", want, view)
		}
	}
}

func TestLargeCodeContextPromptShowsDeliveryMenu(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	symbols := testDisplaySymbols()
	m.state = stateContextReady
	m.schemaB = strings.Repeat("x", 2*1024)
	m.metadata = []protocol.ExtractionMetadata{{Path: "internal/scanner/go.go"}}

	view := m.viewContextReady()

	for _, want := range []string{
		"This WILL include the actual source code from:",
		"  - internal/scanner/go.go",
		symbols.warning,
		"Prompt 2: Code Context is large (2KB).",
		"  [c] Copy anyway",
		"  [f] Save to temp file",
		"  [p] Print to terminal",
		"Choice (recommended: f):",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("large code context view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Copy Prompt 2: Code Context to clipboard") {
		t.Fatalf("large code context used normal copy prompt:\n%s", view)
	}
}

func TestNormalPromptAcceptsHiddenCopyShortcut(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextReady
	m.schemaB = "context payload"

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateContextReady {
		t.Fatalf("state = %v, want %v before copy command completes", got.state, stateContextReady)
	}
	if cmd == nil {
		t.Fatal("hidden c copy shortcut did not return copy command")
	}
}

func TestLargePromptCopyShortcutCopiesAnyway(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = strings.Repeat("x", 32)

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("c")})
	if cmd == nil {
		t.Fatal("large prompt c shortcut did not return copy command")
	}
}

func TestLargePromptFileShortcutReturnsSaveCommand(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateScanComplete
	m.schemaA = strings.Repeat("x", 32)

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if cmd == nil {
		t.Fatal("large prompt f shortcut did not return save command")
	}
}

func TestLargePromptPrintShowsManualCopyPayload(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateScanComplete
	m.schemaA = strings.Repeat("x", 32)

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("large prompt print returned unexpected command")
	}
	if got.state != stateManualCopy {
		t.Fatalf("state = %v, want %v", got.state, stateManualCopy)
	}
	if got.manualCopyKind != topologyPromptKind || got.manualCopyText != strings.Repeat("x", 32) {
		t.Fatalf("manual copy payload not preserved: kind=%q text=%q", got.manualCopyKind, got.manualCopyText)
	}
	if !strings.Contains(got.View(), "--- BEGIN Prompt 1: Topology ---") {
		t.Fatalf("manual copy view missing prompt block:\n%s", got.View())
	}
}

func TestLargePromptCancelAdvancesWorkflow(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = strings.Repeat("x", 32)

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.status.text != "Prompt 2: Code Context was not copied." {
		t.Fatalf("status = %#v", got.status)
	}
	if !got.paste.Focused() {
		t.Fatal("paste input was not focused after cancel")
	}
	if cmd == nil {
		t.Fatal("cancel did not return textarea blink command")
	}
}

func TestPartialExtractionWarningEnterReturnsToExtractionInput(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextWarning
	m.pendingSchemaB = "context payload"
	m.pendingMetadata = []protocol.ExtractionMetadata{{Path: "present.go"}}
	m.pendingExtractedCount = 1
	m.pendingFailedCommands = []string{"missing.go: file not found: missing.go"}
	m.paste.SetValue("FILE:present.go\nFILE:missing.go")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForExtractions {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForExtractions)
	}
	if got.schemaB != "" || len(got.metadata) != 0 {
		t.Fatalf("partial context was not discarded: schemaB=%q metadata=%v", got.schemaB, got.metadata)
	}
	if got.pendingSchemaB != "" || len(got.pendingFailedCommands) != 0 || len(got.pendingSafetyExclusions) != 0 {
		t.Fatalf("pending warning state was not cleared: %#v", got)
	}
	if !got.paste.Focused() {
		t.Fatal("paste input was not focused after returning to extraction input")
	}
	if cmd == nil {
		t.Fatal("warning dismissal did not return textarea blink command")
	}
}

func TestPartialExtractionWarningYProceedsWithAvailableContext(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextWarning
	m.pendingSchemaB = "context payload"
	m.pendingMetadata = []protocol.ExtractionMetadata{{Path: "present.go"}}
	m.pendingExtractedCount = 1
	m.pendingFailedCommands = []string{"missing.go: file not found: missing.go"}

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateContextReady {
		t.Fatalf("state = %v, want %v", got.state, stateContextReady)
	}
	if got.schemaB != "context payload" {
		t.Fatalf("schemaB = %q, want pending context", got.schemaB)
	}
	if len(got.metadata) != 1 || got.metadata[0].Path != "present.go" {
		t.Fatalf("metadata = %#v, want pending metadata", got.metadata)
	}
	if got.pendingSchemaB != "" || len(got.pendingFailedCommands) != 0 || len(got.pendingSafetyExclusions) != 0 {
		t.Fatalf("pending warning state was not cleared: %#v", got)
	}
	if cmd != nil {
		t.Fatal("proceed shortcut returned unexpected command")
	}
}

func TestPrompt2TempFileCompletionPersistsOnboardingCompletion(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = strings.Repeat("x", 32)

	next, cmd := m.Update(savePromptDoneMsg{
		kind: codeContextPromptKind,
		path: filepath.Join(t.TempDir(), "prompt-2-code-context.txt"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if !strings.Contains(got.status.text, "Saved Prompt 2: Code Context to temp file:") ||
		!strings.Contains(got.status.text, "Attach this file to your AI chat") {
		t.Fatalf("status missing saved file guidance: %#v", got.status)
	}
	if cmd == nil {
		t.Fatal("temp file completion did not return textarea blink command")
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
}

func TestPrompt1TempFileWithSupportedRevealPausesBeforeExtractionInput(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	path := filepath.Join(t.TempDir(), "prompt-1-topology.txt")

	next, cmd := m.Update(savePromptDoneMsg{
		kind:      topologyPromptKind,
		path:      path,
		canReveal: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("supported temp-file reveal returned unexpected command before consent")
	}
	if got.state != statePromptFileReveal {
		t.Fatalf("state = %v, want %v", got.state, statePromptFileReveal)
	}
	if got.promptFileKind != topologyPromptKind || got.promptFilePath != path {
		t.Fatalf("prompt file state not preserved: kind=%q path=%q", got.promptFileKind, got.promptFilePath)
	}
	view := got.View()
	if !strings.Contains(view, path) || !strings.Contains(view, "Open containing folder? (y/N)") {
		t.Fatalf("reveal view missing path or prompt:\n%s", view)
	}
}

func TestPrompt2TempFileWithSupportedRevealPausesBeforeFinalResponseInput(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	path := filepath.Join(t.TempDir(), "prompt-2-code-context.txt")

	next, _ := m.Update(savePromptDoneMsg{
		kind:      codeContextPromptKind,
		path:      path,
		canReveal: true,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != statePromptFileReveal {
		t.Fatalf("state = %v, want %v", got.state, statePromptFileReveal)
	}
	if got.promptFileKind != codeContextPromptKind || got.promptFilePath != path {
		t.Fatalf("prompt file state not preserved: kind=%q path=%q", got.promptFileKind, got.promptFilePath)
	}
}

func TestPromptFileRevealNoSkipsOpenerAndAdvances(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = statePromptFileReveal
	m.promptFileKind = topologyPromptKind
	m.promptFilePath = filepath.Join(t.TempDir(), "prompt-1-topology.txt")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForExtractions {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForExtractions)
	}
	if got.promptFileKind != "" || got.promptFilePath != "" {
		t.Fatalf("prompt file state was not cleared: kind=%q path=%q", got.promptFileKind, got.promptFilePath)
	}
	if !strings.Contains(got.status.text, "Attach this file to your AI chat") {
		t.Fatalf("status missing attach guidance: %#v", got.status)
	}
	if cmd == nil {
		t.Fatal("skip reveal did not return textarea blink command")
	}
}

func TestPromptFileRevealYesRunsOpener(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = statePromptFileReveal
	m.promptFileKind = topologyPromptKind
	m.promptFilePath = filepath.Join(t.TempDir(), "prompt-1-topology.txt")

	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd == nil {
		t.Fatal("reveal confirmation did not return opener command")
	}
}

func TestPromptFileRevealSuccessAdvances(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	path := filepath.Join(t.TempDir(), "prompt-1-topology.txt")

	next, cmd := m.Update(openPromptFileDoneMsg{
		kind: topologyPromptKind,
		path: path,
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForExtractions {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForExtractions)
	}
	if !strings.Contains(got.status.text, path) || !strings.Contains(got.status.text, "Attach this file to your AI chat") {
		t.Fatalf("status missing saved path or attach guidance: %#v", got.status)
	}
	if cmd == nil {
		t.Fatal("reveal success did not return textarea blink command")
	}
}

func TestPromptFileRevealFailureKeepsPathVisibleAndAdvances(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	path := filepath.Join(t.TempDir(), "prompt-2-code-context.txt")

	next, cmd := m.Update(openPromptFileDoneMsg{
		kind: codeContextPromptKind,
		path: path,
		err:  errors.New("boom"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.status.severity != messageWarning {
		t.Fatalf("status severity = %v, want warning", got.status.severity)
	}
	if !strings.Contains(got.status.text, "Could not open the file manager automatically.") ||
		!strings.Contains(got.status.text, path) {
		t.Fatalf("status missing failure guidance or path: %#v", got.status)
	}
	if cmd == nil {
		t.Fatal("reveal failure did not return textarea blink command")
	}
}

func TestPrompt2RevealDecisionPersistsOnboardingCompletion(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)
	m.state = statePromptFileReveal
	m.promptFileKind = codeContextPromptKind
	m.promptFilePath = filepath.Join(t.TempDir(), "prompt-2-code-context.txt")

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if cmd == nil {
		t.Fatal("Prompt 2 reveal skip did not return textarea blink command")
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
}

func TestLargeProjectPromptUsesWarningSymbol(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	m.state = stateScanComplete
	m.largeProjectPending = true
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Modules: []model.Module{{Name: "big", FileCount: 2450}},
	})

	view := m.viewScanComplete()

	for _, want := range []string{
		symbols.warning,
		"Large project detected: 2450 files.",
		"Options:",
		"  [c] Continue",
		"  [t] Truncate Prompt 1: Topology to 50 packages",
		"  [e] Exit to home",
		"Choice (recommended: t):",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("large project view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Choose: [c] continue") {
		t.Fatalf("large project view used compact choice line:\n%s", view)
	}
}

func TestWritePreviewUsesWarningSymbol(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	m.updates = []writer.FileUpdate{
		{Path: "internal/service/order.go", Kind: writer.UpdateKindWrite},
		{Path: "internal/service/old_order.go", Kind: writer.UpdateKindDelete},
	}

	view := m.viewWritePreview()

	for _, want := range []string{
		symbols.warning,
		"About to apply changes to disk:",
		"  [write] internal/service/order.go",
		"  [delete] internal/service/old_order.go",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("write preview missing %q:\n%s", want, view)
		}
	}
}

func TestClipboardFailureStartsTempFileFallback(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	payload := "[PROJECT TOPOLOGY]\nGo project"

	next, cmd := m.Update(copyDoneMsg{
		kind: topologyPromptKind,
		text: payload,
		err:  errors.New("pbcopy unavailable"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd == nil {
		t.Fatal("clipboard failure did not return temp-file save command")
	}
	if got.state != stateHome {
		t.Fatalf("state = %v, want unchanged stateHome before save completes", got.state)
	}
	view := got.View()
	if !strings.Contains(view, symbols.warning) ||
		!strings.Contains(view, "pbcopy unavailable") ||
		!strings.Contains(view, "clipboard copy failed") ||
		strings.Contains(view, "[PROJECT TOPOLOGY]") ||
		strings.Contains(view, "Go project") {
		t.Fatalf("clipboard failure view did not preserve warning while hiding payload:\n%s", view)
	}
}

func TestClipboardFailureTempFileFallbackShowsPathAndDocs(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	path := filepath.Join(t.TempDir(), "prompt-1-topology.txt")

	next, cmd := m.Update(savePromptDoneMsg{
		kind:         topologyPromptKind,
		text:         "[PROJECT TOPOLOGY]\nGo project",
		path:         path,
		canReveal:    true,
		clipboardErr: errors.New("pbcopy unavailable"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("supported fallback reveal returned unexpected command before consent")
	}
	if got.state != statePromptFileReveal {
		t.Fatalf("state = %v, want %v", got.state, statePromptFileReveal)
	}
	view := got.View()
	for _, want := range []string{
		symbols.warning,
		"Prompt 1: Topology clipboard copy failed: pbcopy unavailable",
		clipboard.DocsURL,
		"Saved Prompt 1: Topology to temp file:",
		path,
		"Open containing folder? (y/N)",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("fallback temp-file view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "[PROJECT TOPOLOGY]") || strings.Contains(view, "Go project") {
		t.Fatalf("fallback temp-file view rendered prompt payload:\n%s", view)
	}
}

func TestPrompt2ClipboardFailureTempFileFallbackPersistsOnboardingCompletion(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)
	path := filepath.Join(t.TempDir(), "prompt-2-code-context.txt")

	next, cmd := m.Update(savePromptDoneMsg{
		kind:         codeContextPromptKind,
		text:         "context payload",
		path:         path,
		clipboardErr: errors.New("xclip unavailable"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.status.severity != messageWarning ||
		!strings.Contains(got.status.text, "xclip unavailable") ||
		!strings.Contains(got.status.text, clipboard.DocsURL) ||
		!strings.Contains(got.status.text, path) {
		t.Fatalf("status missing clipboard fallback guidance: %#v", got.status)
	}
	if cmd == nil {
		t.Fatal("Prompt 2 fallback completion did not return textarea blink command")
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
}

func TestClipboardFailureTempFileFailureFallsBackToManualCopy(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	payload := "[PROJECT TOPOLOGY]\nGo project"

	next, cmd := m.Update(savePromptDoneMsg{
		kind:         topologyPromptKind,
		text:         payload,
		clipboardErr: errors.New("xclip unavailable"),
		err:          errors.New("permission denied"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("failed temp-file fallback returned unexpected command")
	}
	if got.state != stateManualCopy {
		t.Fatalf("state = %v, want %v", got.state, stateManualCopy)
	}
	if got.status.severity != messageError ||
		!strings.Contains(got.status.text, "xclip unavailable") ||
		!strings.Contains(got.status.text, clipboard.DocsURL) ||
		!strings.Contains(got.status.text, "permission denied") {
		t.Fatalf("status missing combined fallback errors: %#v", got.status)
	}
	if got.manualCopyKind != topologyPromptKind || got.manualCopyText != payload {
		t.Fatalf("manual fallback payload not preserved: kind=%q text=%q", got.manualCopyKind, got.manualCopyText)
	}
	view := got.View()
	if strings.Count(view, clipboard.DocsURL) != 1 {
		t.Fatalf("manual fallback duplicated clipboard docs URL:\n%s", view)
	}
	if !strings.Contains(view, "Manually copy Prompt 1: Topology from the block below") ||
		!strings.Contains(view, "--- BEGIN Prompt 1: Topology ---") ||
		!strings.Contains(view, "[PROJECT TOPOLOGY]") {
		t.Fatalf("manual fallback view missing copy guidance or payload:\n%s", view)
	}
}

func TestClipboardFallbackManualCopyWrapsLongLinesToTerminalWidth(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.width = 64
	payload := strings.Repeat("manual copy payload ", 20)

	next, cmd := m.Update(savePromptDoneMsg{
		kind:         topologyPromptKind,
		text:         payload,
		clipboardErr: errors.New("xclip unavailable"),
		err:          errors.New("permission denied"),
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd != nil {
		t.Fatal("failed temp-file fallback returned unexpected command")
	}
	if got.state != stateManualCopy {
		t.Fatalf("state = %v, want %v", got.state, stateManualCopy)
	}

	assertMaxRenderedLineWidth(t, got.View(), 64)
}

func TestWriteDoneWithErrorsRendersErrorStatus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	symbols := testDisplaySymbols()
	m.state = stateWriting

	next, cmd := m.Update(writeDoneMsg{
		updates: []writer.FileUpdate{{Path: "internal/ok.go", Kind: writer.UpdateKindWrite}},
		errs:    []error{errors.New("internal/fail.go: permission denied")},
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd == nil {
		t.Fatal("write completion did not restart text input blink")
	}
	if got.status.severity != messageError {
		t.Fatalf("status severity = %v, want error", got.status.severity)
	}
	view := got.View()
	if !strings.Contains(view, symbols.error) ||
		!strings.Contains(view, "Finished with 1 apply error(s).") {
		t.Fatalf("write error status missing marker or text:\n%s", view)
	}
}

func TestPrompt2CopyPersistsOnboardingCompletion(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)

	next, cmd := m.Update(copyDoneMsg{
		kind: codeContextPromptKind,
		text: "context payload",
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if cmd == nil {
		t.Fatal("Prompt 2 copy completion did not return textarea blink command")
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
}

func TestPrompt2ManualCopyPersistsOnboardingCompletion(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)
	m.state = stateManualCopy
	m.manualCopyKind = codeContextPromptKind
	m.manualCopyText = "context payload"

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if cmd == nil {
		t.Fatal("manual Prompt 2 completion did not return textarea blink command")
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}

	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
}

func TestPrompt2PersistenceFailureIsSilent(t *testing.T) {
	dir := t.TempDir()
	fileAsDir := filepath.Join(dir, "settings-parent")
	if err := os.WriteFile(fileAsDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = filepath.Join(fileAsDir, "settings.json")
	m := NewModel("/tmp/project", cfg)

	next, cmd := m.Update(copyDoneMsg{
		kind: codeContextPromptKind,
		text: "context payload",
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForCode {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForCode)
	}
	if got.status.text != "Prompt 2: Code Context copied. Next: paste the final AI response." {
		t.Fatalf("status = %#v, want no settings error", got.status)
	}
	if got.status.severity != messageSuccess {
		t.Fatalf("status severity = %v, want success", got.status.severity)
	}
	if cmd == nil {
		t.Fatal("Prompt 2 copy completion did not return textarea blink command")
	}
}

func TestManualCopyEnterAdvancesToExtractionInput(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateManualCopy
	m.manualCopyKind = topologyPromptKind
	m.manualCopyText = "payload"

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}
	if got.state != stateWaitingForExtractions {
		t.Fatalf("state = %v, want %v", got.state, stateWaitingForExtractions)
	}
	if got.manualCopyText != "" || got.manualCopyKind != "" {
		t.Fatal("manual copy payload was not cleared")
	}
	if !strings.Contains(got.status.text, "any LLM chat interface") {
		t.Fatalf("manual copy status missing LLM guidance: %#v", got.status)
	}
	if got.status.severity != messageNeutral {
		t.Fatalf("status severity = %v, want neutral", got.status.severity)
	}
	if !got.paste.Focused() {
		t.Fatal("paste input was not focused")
	}
	if cmd == nil {
		t.Fatal("manual copy advance did not return textarea blink command")
	}
}

func TestTextPreviewHidesExtraLines(t *testing.T) {
	preview, hiddenLines := textPreview("one\ntwo\nthree\nfour", 2)

	if hiddenLines != 2 {
		t.Fatalf("hiddenLines = %d, want 2", hiddenLines)
	}
	if preview != "one\ntwo" {
		t.Fatalf("preview = %q, want %q", preview, "one\ntwo")
	}
}

func numberedLines(count int) string {
	lines := make([]string, 0, count)
	for i := 1; i <= count; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	return strings.Join(lines, "\n")
}

func assertMaxRenderedLineWidth(t *testing.T, rendered string, maxWidth int) {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		if width := lipgloss.Width(line); width > maxWidth {
			t.Fatalf("rendered line width = %d, want <= %d:\n%s\n\nfull view:\n%s", width, maxWidth, line, rendered)
		}
	}
}

func newReviewRepo(t *testing.T, updatedLine string) string {
	t.Helper()

	dir := t.TempDir()
	runReviewGitCmd(t, dir, "init")
	runReviewGitCmd(t, dir, "checkout", "-b", "main")
	runReviewGitCmd(t, dir, "config", "user.name", "Badger Test")
	runReviewGitCmd(t, dir, "config", "user.email", "badger@example.com")
	writeReviewFile(t, dir, "app.go", "package main\n\nfunc main() {\n\tprintln(\"base\")\n}\n")
	runReviewGitCmd(t, dir, "add", "app.go")
	runReviewGitCmd(t, dir, "commit", "-m", "initial commit")
	if updatedLine != "" {
		writeReviewFile(t, dir, "app.go", "package main\n\nfunc main() {\n\t"+updatedLine+"\n}\n")
	}
	return dir
}

func writeReviewFile(t *testing.T, dir, path, contents string) {
	t.Helper()

	fullPath := filepath.Join(dir, path)
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", fullPath, err)
	}
}

func runReviewGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Badger Test",
		"GIT_AUTHOR_EMAIL=badger@example.com",
		"GIT_COMMITTER_NAME=Badger Test",
		"GIT_COMMITTER_EMAIL=badger@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}
