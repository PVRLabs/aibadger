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

func TestSaveAndLoadSettingsWithLimits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := Settings{FirstRunOnboardingCompleted: true, WhitespaceMode: "smart", Limits: &SettingsLimits{
		MaxFilesPerDirectory: 5000, MaxContextFileBytes: 512 * 1024,
		MaxPromptTwoBytes: 1024 * 1024, MaxTopologyPromptBytes: 2 * 1024 * 1024,
	}}
	if err := SaveSettings(path, want); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if got.Limits == nil || *got.Limits != *want.Limits {
		t.Fatalf("Limits = %#v, want %#v", got.Limits, want.Limits)
	}
}

func TestResolveSettingsLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxFilesPerDirectory = 400
	cfg.MaxContextFileBytes = 70 * 1024
	cfg.MaxPromptTwoBytes = 220 * 1024
	cfg.MaxTopologyPromptBytes = 600 * 1024

	got, warnings := resolveSettingsLimits(cfg, &SettingsLimits{
		MaxFilesPerDirectory:   250,
		MaxContextFileBytes:    0,
		MaxPromptTwoBytes:      1024 * 1024,
		MaxTopologyPromptBytes: 2 * 1024 * 1024,
	})
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none", warnings)
	}
	if got.MaxFilesPerDirectory != 250 || got.MaxContextFileBytes != cfg.MaxContextFileBytes || got.MaxPromptTwoBytes != 1024*1024 || got.MaxTopologyPromptBytes != 2*1024*1024 {
		t.Fatalf("resolved config = %#v", got)
	}
}

func TestResolveSettingsLimitsInvalidNumbersAreIndependentAndStable(t *testing.T) {
	cfg := DefaultConfig()
	got, warnings := resolveSettingsLimits(cfg, &SettingsLimits{
		MaxFilesPerDirectory: -1, MaxContextFileBytes: 512*1024 + 1,
		MaxPromptTwoBytes: 256 * 1024, MaxTopologyPromptBytes: 1,
	})
	if got.MaxFilesPerDirectory != cfg.MaxFilesPerDirectory || got.MaxContextFileBytes != cfg.MaxContextFileBytes || got.MaxTopologyPromptBytes != cfg.MaxTopologyPromptBytes {
		t.Fatalf("invalid values changed config: %#v", got)
	}
	if got.MaxPromptTwoBytes != 256*1024 {
		t.Fatalf("valid sibling MaxPromptTwoBytes = %d", got.MaxPromptTwoBytes)
	}
	if len(warnings) != 3 || !strings.Contains(warnings[0], "max_files_per_directory") || !strings.Contains(warnings[1], "max_context_file_bytes") || !strings.Contains(warnings[2], "max_topology_prompt_bytes") {
		t.Fatalf("warnings = %v", warnings)
	}
}

func TestLoadSettingsMalformedLimitsPreservesTopLevelSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"first_run_onboarding_completed":true,"whitespace_mode":"ignore","limits":{"max_context_file_bytes":"large"}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSettings(path)
	if err == nil {
		t.Fatal("LoadSettings() error = nil")
	}
	if !got.FirstRunOnboardingCompleted || got.WhitespaceMode != "ignore" || got.Limits != nil {
		t.Fatalf("settings = %#v", got)
	}
}

func TestNewModelMalformedLimitsKeepsIncomingLimitsAndStartupStatus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	data := []byte(`{"first_run_onboarding_completed":true,"whitespace_mode":"ignore","limits":{"max_files_per_directory":"many"}}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	cfg.MaxFilesPerDirectory = 777
	cfg.Startup.Status.Text = "Existing startup guidance."
	cfg.Startup.Status.Severity = "success"
	m := NewModel("/tmp/project", cfg)
	if m.cfg.MaxFilesPerDirectory != 777 || m.cfg.WhitespaceMode != "ignore" {
		t.Fatalf("effective config = %#v", m.cfg)
	}
	view := m.View()
	if !strings.Contains(view, "Existing startup guidance.") || !strings.Contains(view, "limits object is invalid") {
		t.Fatalf("view = %q", view)
	}
}

func TestNewModelAppliesLimitsAndKeepsReviewBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveSettings(path, Settings{FirstRunOnboardingCompleted: true, Limits: &SettingsLimits{
		MaxFilesPerDirectory: 1000, MaxContextFileBytes: 128 * 1024,
		MaxPromptTwoBytes: 512 * 1024, MaxTopologyPromptBytes: 1024 * 1024,
	}}); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	m := NewModel("/tmp/project", cfg)
	if m.cfg.MaxFilesPerDirectory != 1000 || m.engineOptions(0).MaxContextFileBytes != 128*1024 || m.engineOptions(0).MaxPromptTwoBytes != 512*1024 || m.engineOptions(0).MaxTopologyPromptBytes != 1024*1024 {
		t.Fatalf("effective config = %#v", m.cfg)
	}
	if m.reviewMaxPromptBytes != cfg.MaxTopologyPromptBytes {
		t.Fatalf("review budget = %d, want %d", m.reviewMaxPromptBytes, cfg.MaxTopologyPromptBytes)
	}
}

func TestMarkOnboardingCompletedPreservesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	wantLimits := &SettingsLimits{MaxFilesPerDirectory: 1000, MaxContextFileBytes: 128 * 1024, MaxPromptTwoBytes: 512 * 1024, MaxTopologyPromptBytes: 1024 * 1024}
	if err := SaveSettings(path, Settings{WhitespaceMode: "ignore", Limits: wantLimits}); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	m := NewModel("/tmp/project", cfg)
	m.markOnboardingCompleted()
	got, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.FirstRunOnboardingCompleted || got.WhitespaceMode != "ignore" || got.Limits == nil || *got.Limits != *wantLimits {
		t.Fatalf("saved settings = %#v", got)
	}
}

func TestMarkOnboardingCompletedDoesNotOverwriteMalformedSettings(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "top-level document", data: []byte(`{"first_run_onboarding_completed":`)},
		{name: "limits object", data: []byte(`{"first_run_onboarding_completed":false,"limits":{"max_files_per_directory":"many"}}`)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, tc.data, 0600); err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfig()
			cfg.SettingsPath = path
			m := NewModel("/tmp/project", cfg)
			if m.settingsWriteAllowed {
				t.Fatal("settingsWriteAllowed = true after malformed load")
			}
			m.markOnboardingCompleted()
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(tc.data) {
				t.Fatalf("settings bytes changed:\n got %q\nwant %q", got, tc.data)
			}
		})
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
