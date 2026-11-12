package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestInteractiveScanReceivesDirectorySettings(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"tool.py", "build/visible.html", ".my-cache/hidden.html", "nested/.MY-CACHE/hidden.html", ".git/hidden.html"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"first_run_onboarding_completed":true,"exclude_generated_directories":false,"additional_excluded_directories":[".my-cache"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	m := NewModel(root, cfg)
	m.goalInput.SetValue("inspect this project")
	_, cmd := m.submitGoal()
	if cmd == nil {
		t.Fatal("missing scan command")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatal("missing scan batch")
	}
	result, ok := batch[0]().(scanDoneMsg)
	if !ok || result.err != nil {
		t.Fatalf("scan result: %+v", result)
	}
	output := result.eng.GenerateTopology()
	if !strings.Contains(output, "visible.html") || strings.Contains(output, "hidden.html") {
		t.Fatalf("interactive policy missing: %s", output)
	}
}

func TestInvalidDirectorySettingsDoNotRewriteSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := `{"first_run_onboarding_completed":false,"whitespace_mode":"ignore","exclude_generated_directories":false,"additional_excluded_directories":["../invalid",".my-cache"]}`
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	m := NewModel(t.TempDir(), cfg)
	if m.cfg.DirectoryPolicy.Skip("build") || !m.cfg.DirectoryPolicy.Skip(".my-cache") || len(m.startupWarnings) == 0 {
		t.Fatal("valid fields discarded or warning missing")
	}
	m.markOnboardingCompleted()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != original {
		t.Fatal("invalid settings rewritten")
	}
}
