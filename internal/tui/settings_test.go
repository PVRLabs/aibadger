package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveAndLoadSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".badger", "settings.json")

	if err := SaveSettings(path, Settings{FirstRunOnboardingCompleted: true}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	got, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if !got.FirstRunOnboardingCompleted {
		t.Fatal("FirstRunOnboardingCompleted = false, want true")
	}
	if got.WhitespaceMode != "" {
		t.Fatalf("WhitespaceMode = %q, want empty", got.WhitespaceMode)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"first_run_onboarding_completed": true`) {
		t.Fatalf("settings file missing completion flag:\n%s", string(data))
	}
}

func TestSaveAndLoadSettingsWithWhitespaceMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".badger", "settings.json")

	if err := SaveSettings(path, Settings{
		FirstRunOnboardingCompleted: true,
		WhitespaceMode:              "ignore",
	}); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}

	got, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if got.WhitespaceMode != "ignore" {
		t.Fatalf("WhitespaceMode = %q, want ignore", got.WhitespaceMode)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), `"whitespace_mode": "ignore"`) {
		t.Fatalf("settings file missing whitespace mode:\n%s", string(data))
	}
}

func TestLoadSettingsReturnsErrors(t *testing.T) {
	if _, err := LoadSettings(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("LoadSettings() missing file error = nil")
	}

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := LoadSettings(path); err == nil {
		t.Fatal("LoadSettings() invalid JSON error = nil")
	}
}

func TestSaveSettingsReturnsErrors(t *testing.T) {
	if err := SaveSettings("", Settings{}); err == nil {
		t.Fatal("SaveSettings() empty path error = nil")
	}

	dir := t.TempDir()
	fileAsDir := filepath.Join(dir, "settings-parent")
	if err := os.WriteFile(fileAsDir, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(fileAsDir, "settings.json")
	if err := SaveSettings(path, Settings{}); err == nil {
		t.Fatal("SaveSettings() path under file error = nil")
	}
}
