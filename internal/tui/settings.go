package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PVRLabs/aibadger/internal/defaults"
)

const settingsDirName = ".badger"

type SettingsLimits struct {
	MaxFilesPerDirectory   int `json:"max_files_per_directory,omitempty"`
	MaxContextFileBytes    int `json:"max_context_file_bytes,omitempty"`
	MaxPromptTwoBytes      int `json:"max_prompt_two_bytes,omitempty"`
	MaxTopologyPromptBytes int `json:"max_topology_prompt_bytes,omitempty"`
}

type Settings struct {
	FirstRunOnboardingCompleted bool            `json:"first_run_onboarding_completed"`
	WhitespaceMode              string          `json:"whitespace_mode,omitempty"`
	Limits                      *SettingsLimits `json:"limits,omitempty"`
}

type limitsDecodeError struct{ err error }

func (e limitsDecodeError) Error() string { return "invalid limits object: " + e.err.Error() }

type settingsDocument struct {
	FirstRunOnboardingCompleted bool            `json:"first_run_onboarding_completed"`
	WhitespaceMode              string          `json:"whitespace_mode,omitempty"`
	Limits                      json.RawMessage `json:"limits"`
}

func DefaultSettingsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if home == "" {
		return "", fmt.Errorf("home directory is empty")
	}
	return filepath.Join(home, settingsDirName, "settings.json"), nil
}

func LoadSettings(path string) (Settings, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, err
	}

	var document settingsDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return Settings{}, err
	}
	settings := Settings{
		FirstRunOnboardingCompleted: document.FirstRunOnboardingCompleted,
		WhitespaceMode:              document.WhitespaceMode,
	}
	if len(document.Limits) > 0 && string(document.Limits) != "null" {
		var limits SettingsLimits
		if err := json.Unmarshal(document.Limits, &limits); err != nil {
			return settings, limitsDecodeError{err: err}
		}
		settings.Limits = &limits
	}
	return settings, nil
}

func resolveSettingsLimits(cfg Config, limits *SettingsLimits) (Config, []string) {
	if limits == nil {
		return cfg, nil
	}
	type limitSpec struct {
		key     string
		value   int
		minimum int
		maximum int
		apply   func(int)
	}
	specs := []limitSpec{
		{"max_files_per_directory", limits.MaxFilesPerDirectory, defaults.MaxFilesPerDirectory, defaults.MaxFilesPerDirectoryUpperBound, func(v int) { cfg.MaxFilesPerDirectory = v }},
		{"max_context_file_bytes", limits.MaxContextFileBytes, defaults.MaxContextFileBytes, defaults.MaxContextFileBytesUpperBound, func(v int) { cfg.MaxContextFileBytes = v }},
		{"max_prompt_two_bytes", limits.MaxPromptTwoBytes, defaults.MaxPromptTwoBytes, defaults.MaxPromptTwoBytesUpperBound, func(v int) { cfg.MaxPromptTwoBytes = v }},
		{"max_topology_prompt_bytes", limits.MaxTopologyPromptBytes, defaults.MaxTopologyPromptBytes, defaults.MaxTopologyPromptBytesUpperBound, func(v int) { cfg.MaxTopologyPromptBytes = v }},
	}
	var warnings []string
	for _, spec := range specs {
		if spec.value == 0 {
			continue
		}
		if spec.value < spec.minimum || spec.value > spec.maximum {
			warnings = append(warnings, fmt.Sprintf("settings limits.%s must be between %d and %d; keeping the current value", spec.key, spec.minimum, spec.maximum))
			continue
		}
		spec.apply(spec.value)
	}
	return cfg, warnings
}

func settingsLoadWarning(path string, err error) string {
	var limitsErr limitsDecodeError
	if errors.As(err, &limitsErr) {
		return fmt.Sprintf("Could not apply limits from %s because the limits object is invalid; keeping the current limits.", path)
	}
	return fmt.Sprintf("Could not load settings from %s; keeping the current settings.", path)
}

func SaveSettings(path string, settings Settings) error {
	if path == "" {
		return fmt.Errorf("settings path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(path, data, 0600)
}
