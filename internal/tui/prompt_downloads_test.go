package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	tea "github.com/charmbracelet/bubbletea"
)

func stubDownloadsSave(t *testing.T, path string) *[]byte {
	t.Helper()
	originalResolve := resolveDownloadsDirectory
	originalSave := savePromptToDownloads
	var saved []byte
	resolveDownloadsDirectory = func() (string, error) { return filepath.Dir(path), nil }
	savePromptToDownloads = func(directory string, payload []byte) (string, error) {
		saved = append([]byte(nil), payload...)
		return filepath.Join(directory, filepath.Base(path)), nil
	}
	t.Cleanup(func() {
		resolveDownloadsDirectory = originalResolve
		savePromptToDownloads = originalSave
	})
	return &saved
}

func TestNormalPromptViewsOfferDownloads(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanComplete
	m.schemaA = "topology payload"
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{})
	if view := m.viewScanComplete(); !strings.Contains(view, "Copy Prompt 1: Topology to clipboard (payload: 16B)? [Enter/Y]\n\n[D] Save to Downloads   [N] Skip") {
		t.Fatalf("Prompt 1 view missing Downloads choices:\n%s", view)
	}
	if strings.Contains(m.viewScanComplete(), "(y/N)") || strings.Contains(m.viewScanComplete(), "Deliver Prompt") {
		t.Fatalf("Prompt 1 view has contradictory default guidance:\n%s", m.viewScanComplete())
	}

	m.state = stateContextReady
	m.schemaB = "context payload"
	if view := m.viewContextReady(); !strings.Contains(view, "Copy Prompt 2: Code Context to clipboard (payload: 15B)? [Enter/Y]\n\n[D] Save to Downloads   [N] Skip") {
		t.Fatalf("Prompt 2 view missing Downloads choices:\n%s", view)
	}
	if strings.Contains(m.viewContextReady(), "(y/N)") || strings.Contains(m.viewContextReady(), "Deliver Prompt") {
		t.Fatalf("Prompt 2 view has contradictory default guidance:\n%s", m.viewContextReady())
	}
}

func TestPromptDownloadsSavingFeedbackMatchesBlockedState(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = statePromptFileSaving
	m.promptFileSavingKind = codeContextPromptKind

	if !strings.Contains(m.pipelineView(), "[Extract]") || strings.Contains(m.pipelineView(), "[Map]") {
		t.Fatalf("Prompt 2 saving pipeline = %q, want Extract active", m.pipelineView())
	}
	hints := keyboardHintsForState(statePromptFileSaving)
	if strings.Contains(strings.Join(hints, " "), "Esc") || !strings.Contains(strings.Join(hints, " "), "Ctrl+C quit") {
		t.Fatalf("Prompt saving keyboard hints = %#v, want no Esc cancel", hints)
	}
}

func TestLargePromptViewsRetainTempAndOfferDownloads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LargePromptByteThreshold = 8
	m := NewModel("/tmp/project", cfg)
	m.state = stateScanComplete
	m.schemaA = strings.Repeat("x", 32)
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{})

	view := m.viewScanComplete()
	for _, want := range []string{"[c] Copy to clipboard", "[d] Save to Downloads", "[f] Save to temp file", "[p] Print to terminal", "[n] Skip"} {
		if !strings.Contains(view, want) {
			t.Fatalf("large Prompt 1 view missing %q:\n%s", want, view)
		}
	}

	m.state = stateContextReady
	m.schemaB = strings.Repeat("y", 32)
	view = m.viewContextReady()
	for _, want := range []string{"[c] Copy to clipboard", "[d] Save to Downloads", "[f] Save to temp file", "[p] Print to terminal"} {
		if !strings.Contains(view, want) {
			t.Fatalf("large Prompt 2 view missing %q:\n%s", want, view)
		}
	}
}

func TestDownloadsShortcutCarriesExactPromptPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badger-prompt.txt")
	saved := stubDownloadsSave(t, path)

	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextReady
	m.schemaB = "[CODE CONTEXT]\nexact\x00payload\n"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := next.(Model)
	if got.state != statePromptFileSaving || got.promptFileSavingKind != codeContextPromptKind {
		t.Fatalf("Downloads start = state %v kind %q, want saving Prompt 2", got.state, got.promptFileSavingKind)
	}
	msg := executeCmd(t, cmd).(savePromptDoneMsg)
	if msg.destination != promptFileDestinationDownloads || msg.kind != codeContextPromptKind || msg.text != m.schemaB {
		t.Fatalf("Downloads result = %#v, want exact Prompt 2 payload", msg)
	}
	if string(*saved) != m.schemaB {
		t.Fatalf("Downloads writer payload = %q, want %q", *saved, m.schemaB)
	}
}

func TestDownloadsPrompt1ShortcutCarriesExactPromptPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badger-prompt.txt")
	saved := stubDownloadsSave(t, path)

	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanComplete
	m.schemaA = "[PROJECT TOPOLOGY]\nexact\x00payload\n"
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{})
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := next.(Model)
	if got.state != statePromptFileSaving || got.promptFileSavingKind != topologyPromptKind {
		t.Fatalf("Downloads start = state %v kind %q, want saving Prompt 1", got.state, got.promptFileSavingKind)
	}
	msg := executeCmd(t, cmd).(savePromptDoneMsg)
	if msg.destination != promptFileDestinationDownloads || msg.kind != topologyPromptKind || msg.text != m.schemaA {
		t.Fatalf("Downloads result = %#v, want exact Prompt 1 payload", msg)
	}
	if string(*saved) != m.schemaA {
		t.Fatalf("Downloads writer payload = %q, want %q", *saved, m.schemaA)
	}
}

func TestDownloadsSaveFailureReturnsToDeliveryScreen(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanComplete
	m.schemaA = "topology payload"

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil || next.(Model).state != statePromptFileSaving {
		t.Fatal("Downloads save did not enter saving state")
	}
	next, cmd = next.(Model).Update(savePromptDoneMsg{
		kind:        topologyPromptKind,
		text:        m.schemaA,
		destination: promptFileDestinationDownloads,
		err:         errors.New("Downloads unavailable"),
	})
	got := next.(Model)
	if got.state != stateScanComplete || got.schemaA != m.schemaA {
		t.Fatalf("failed Downloads save = state %v schema %q, want delivery state and active prompt", got.state, got.schemaA)
	}
	if got.status.severity != messageError || !strings.Contains(got.status.text, "Could not save Prompt 1: Topology to Downloads") {
		t.Fatalf("failed Downloads status = %#v", got.status)
	}
	if cmd != nil {
		t.Fatal("failed Downloads save returned unexpected command")
	}
}

func TestDownloadsSaveBlocksCompetingDeliveryKeys(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateContextReady
	m.schemaB = "context payload"
	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	got := next.(Model)
	if cmd == nil || got.state != statePromptFileSaving {
		t.Fatal("Downloads save did not enter saving state")
	}
	for _, key := range []string{"c", "n", "esc"} {
		next, competingCmd := got.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
		if key == "esc" {
			next, competingCmd = got.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		}
		got = next.(Model)
		if got.state != statePromptFileSaving || competingCmd != nil {
			t.Fatalf("key %q changed in-flight save: state=%v cmd=%v", key, got.state, competingCmd)
		}
	}
}

func TestStaleDownloadsResultIsIgnoredAfterSaveCompletes(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = statePromptFileSaving
	m.promptFileSavingKind = topologyPromptKind
	path := filepath.Join(t.TempDir(), "badger-prompt.txt")

	next, _ := m.Update(savePromptDoneMsg{
		kind:        topologyPromptKind,
		path:        path,
		destination: promptFileDestinationDownloads,
	})
	completed := next.(Model)
	if completed.state != stateWaitingForExtractions {
		t.Fatalf("first Downloads result state = %v, want extraction input", completed.state)
	}
	next, cmd := completed.Update(savePromptDoneMsg{
		kind:        topologyPromptKind,
		path:        path,
		destination: promptFileDestinationDownloads,
	})
	got := next.(Model)
	if got.state != stateWaitingForExtractions || cmd != nil {
		t.Fatalf("stale Downloads result changed state=%v cmd=%v", got.state, cmd)
	}
}

func TestDownloadsSaveAlwaysAdvancesWithDestinationWording(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	cfg.Focus = protocol.FocusReview
	m := NewModel("/tmp/project", cfg)
	m.state = statePromptFileSaving
	m.promptFileSavingKind = topologyPromptKind
	m.schemaA = "review topology"

	next, cmd := m.Update(savePromptDoneMsg{
		kind:        topologyPromptKind,
		text:        m.schemaA,
		path:        filepath.Join(t.TempDir(), "badger-prompt.txt"),
		destination: promptFileDestinationDownloads,
		canReveal:   true,
	})
	got := next.(Model)
	if got.state != stateWaitingForExtractions || got.state == statePromptFileReveal {
		t.Fatalf("Downloads Prompt 1 completion state = %v, want direct extraction input", got.state)
	}
	for _, want := range []string{"Saved Prompt 1: Topology to Downloads", "Upload this file to your AI chat", reviewPromptOneNextStep} {
		if !strings.Contains(got.status.text, want) {
			t.Fatalf("Downloads completion status missing %q: %q", want, got.status.text)
		}
	}
	if cmd != nil {
		t.Fatal("Downloads completion returned unexpected command")
	}
}

func TestPrompt2DownloadsCompletionPersistsOnboarding(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.SettingsPath = settingsPath
	m := NewModel("/tmp/project", cfg)
	m.state = statePromptFileSaving
	m.promptFileSavingKind = codeContextPromptKind

	next, cmd := m.Update(savePromptDoneMsg{
		kind:        codeContextPromptKind,
		path:        filepath.Join(t.TempDir(), "badger-prompt.txt"),
		destination: promptFileDestinationDownloads,
	})
	got := next.(Model)
	if got.state != stateWaitingForCode || !strings.Contains(got.status.text, "Saved Prompt 2: Code Context to Downloads") {
		t.Fatalf("Prompt 2 Downloads completion = state %v status %q", got.state, got.status.text)
	}
	if cmd != nil {
		t.Fatal("Prompt 2 Downloads completion returned unexpected command")
	}
	settings, err := LoadSettings(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.FirstRunOnboardingCompleted {
		t.Fatal("Downloads Prompt 2 completion did not persist onboarding")
	}
}
