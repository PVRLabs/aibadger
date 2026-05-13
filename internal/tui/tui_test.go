package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
	tea "github.com/charmbracelet/bubbletea"
)

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
	if !strings.Contains(m.View(), " /\\_/\\  Local-first by default.") ||
		!strings.Contains(m.View(), "( o.o )") {
		t.Fatalf("onboarding view missing compact mascot frame:\n%s", m.View())
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
	paste := strings.Repeat("x", workflow.LargePromptBytes-1)

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
		"Preview:",
		"  Review this diff:",
		"  Index: docs/ui-spec.md",
		"  ===================================================================",
		"Press Enter to submit.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("home view missing %q:\n%s", want, view)
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
	if len(got.goalInput.Value()) != workflow.LargePromptBytes {
		t.Fatalf("goal length = %d, want %d", len(got.goalInput.Value()), workflow.LargePromptBytes)
	}
	if got.status.severity != messageWarning {
		t.Fatalf("status severity = %v, want warning", got.status.severity)
	}
	if got.status.text != "Pasted goal was truncated from 52KB to 50KB." {
		t.Fatalf("status text = %q", got.status.text)
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
	if len(got.goalInput.Value()) > workflow.LargePromptBytes {
		t.Fatalf("goal length = %d, want <= %d", len(got.goalInput.Value()), workflow.LargePromptBytes)
	}
	if !utf8.ValidString(got.goalInput.Value()) {
		t.Fatal("goal input is not valid UTF-8")
	}
	if got.status.severity != messageWarning {
		t.Fatalf("status severity = %v, want warning", got.status.severity)
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
	if cmd == nil {
		t.Fatal("resize did not request a screen clear")
	}
	if msg := cmd(); msg != tea.ClearScreen() {
		t.Fatalf("resize command = %T, want tea.ClearScreen", msg)
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
}

func TestSubmitGoalReviewCommandShowsReviewHelp(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("/review")

	next, cmd := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateReviewHelp {
		t.Fatalf("state = %v, want %v", got.state, stateReviewHelp)
	}
	if got.goal != "" {
		t.Fatalf("goal = %q, want empty", got.goal)
	}
	if cmd != nil {
		t.Fatal("review command returned unexpected scan command")
	}
	view := got.View()
	for _, want := range []string{
		"Code review with Badger",
		"Use this when you want an AI chat to review a change before committing.",
		"Example goal:",
		"Review my current change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing.",
		"Tip:",
		reviewGitShowTip(),
		"For larger diffs, prefer asking Badger to map the project first and let the AI request the specific files it needs.",
		"Preview feature:",
		"Badger does not review local changes directly yet. Coming later: reviewing current unstaged or staged git diffs from your project.",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("review help missing %q:\n%s", want, view)
		}
	}
}

func TestFormatReviewGitShowTip(t *testing.T) {
	tests := []struct {
		name        string
		pipeCommand string
		ok          bool
		want        string
	}{
		{
			name:        "clipboard pipe available",
			pipeCommand: "pbcopy",
			ok:          true,
			want:        "To review the latest commit, run `git show | pbcopy`, then paste it with your review goal.",
		},
		{
			name: "clipboard fallback",
			want: "To review the latest commit, run `git show`, copy its output, and paste it with your review goal.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatReviewGitShowTip(tt.pipeCommand, tt.ok)
			if got != tt.want {
				t.Fatalf("tip = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestViewShowsPersistentHeader(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	view := m.View()

	if !strings.Contains(view, "🦡 AIBADGER "+version.Version) {
		t.Fatalf("home view missing compact version header:\n%s", view)
	}
	if !strings.Contains(view, "Local-first code context for any AI chat") {
		t.Fatalf("home view missing descriptor header:\n%s", view)
	}
	if !strings.Contains(view, "Pipeline: [Map] → Extract → Apply") {
		t.Fatalf("home view missing pipeline indicator:\n%s", view)
	}
	if !strings.Contains(view, " /\\_/\\") || !strings.Contains(view, "( o.o )") || !strings.Contains(view, " > ^ <") {
		t.Fatalf("home view missing compact mascot header:\n%s", view)
	}
}

func TestHomeViewSurfacesReviewUseCase(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	view := m.View()

	if !strings.Contains(view, "Type a goal or paste a diff for review, then press Enter.") {
		t.Fatalf("home view missing review goal guidance:\n%s", view)
	}
	if !strings.Contains(view, "Commands: /help, /review, /exit") {
		t.Fatalf("home view missing review command:\n%s", view)
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

	view := m.View()

	if !strings.Contains(view, "Pipeline: ✓ Map → [Extract] → Apply") {
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

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey returned %T, want tui.Model", next)
	}

	view := got.View()
	if !strings.Contains(view, "Ready for a goal.") {
		t.Fatalf("neutral status text missing:\n%s", view)
	}
	for _, marker := range []string{"✓", "⚠️", "⛔"} {
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

	next, _ := m.Update(copyDoneMsg{
		kind: topologyPromptKind,
		text: "payload",
	})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want tui.Model", next)
	}

	view := got.View()
	if !strings.Contains(view, "✓") ||
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
	if strings.Contains(got.err.Error(), "⛔") {
		t.Fatalf("stored error includes rendered marker: %q", got.err.Error())
	}
	view := got.View()
	if !strings.Contains(view, "⛔") ||
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

			for _, want := range []string{"Enter submit", "Ctrl+U clear line", "Ctrl+C quit"} {
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
	if strings.Contains(view, "⚠️") {
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
	m.state = stateScanComplete
	m.schemaA = strings.Repeat("x", 2*1024)
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Modules: []model.Module{{Name: "internal/app", FileCount: 1}},
	})

	view := m.viewScanComplete()

	for _, want := range []string{
		"⚠️",
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

func TestCopyCodeContextDialogShowsPayloadSize(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextReady
	m.schemaB = "context payload"
	m.metadata = []protocol.ExtractionMetadata{
		{Path: "internal/scanner/go.go"},
		{Path: "internal/models/order.go", Truncated: true},
		{Path: "internal/models/user.go", Dropped: true},
	}

	view := m.viewContextReady()

	for _, want := range []string{
		"⚠️",
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

func TestLargeCodeContextPromptShowsDeliveryMenu(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateContextReady
	m.schemaB = strings.Repeat("x", 2*1024)
	m.metadata = []protocol.ExtractionMetadata{{Path: "internal/scanner/go.go"}}

	view := m.viewContextReady()

	for _, want := range []string{
		"This WILL include the actual source code from:",
		"  - internal/scanner/go.go",
		"⚠️",
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
	m.state = stateScanComplete
	m.largeProjectPending = true
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
		Modules: []model.Module{{Name: "big", FileCount: 2450}},
	})

	view := m.viewScanComplete()

	for _, want := range []string{
		"⚠️",
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
	m.updates = []writer.FileUpdate{
		{Path: "internal/service/order.go", Kind: writer.UpdateKindWrite},
		{Path: "internal/service/old_order.go", Kind: writer.UpdateKindDelete},
	}

	view := m.viewWritePreview()

	for _, want := range []string{
		"⚠️",
		"About to apply changes to disk:",
		"  [write] internal/service/order.go",
		"  [delete] internal/service/old_order.go",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("write preview missing %q:\n%s", want, view)
		}
	}
}

func TestClipboardFailureShowsManualCopyPayload(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
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
	if cmd != nil {
		t.Fatal("clipboard failure returned unexpected command")
	}
	if got.state != stateManualCopy {
		t.Fatalf("state = %v, want %v", got.state, stateManualCopy)
	}
	view := got.View()
	if !strings.Contains(view, "⚠️") ||
		!strings.Contains(view, "clipboard copy failed: pbcopy unavailable") ||
		!strings.Contains(view, "Clipboard is unavailable") ||
		!strings.Contains(view, "[PROJECT TOPOLOGY]") ||
		!strings.Contains(view, "Go project") {
		t.Fatalf("manual copy view missing payload:\n%s", view)
	}
}

func TestWriteDoneWithErrorsRendersErrorStatus(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
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
	if !strings.Contains(view, "⛔") ||
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
