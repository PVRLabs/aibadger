package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/PVRLabs/aibadger/internal/dirpolicy"
)

const settingsDirName = ".badger"

type SettingsLimits struct {
	MaxFilesPerDirectory   int `json:"max_files_per_directory,omitempty"`
	MaxContextFileBytes    int `json:"max_context_file_bytes,omitempty"`
	MaxPromptTwoBytes      int `json:"max_prompt_two_bytes,omitempty"`
	MaxTopologyPromptBytes int `json:"max_topology_prompt_bytes,omitempty"`
}

type Settings struct {
	ExcludeGeneratedDirectories   *bool           `json:"exclude_generated_directories,omitempty"`
	AdditionalExcludedDirectories *[]string       `json:"additional_excluded_directories,omitempty"`
	Warnings                      []string        `json:"-"`
	FirstRunOnboardingCompleted   bool            `json:"first_run_onboarding_completed"`
	WhitespaceMode                string          `json:"whitespace_mode,omitempty"`
	Limits                        *SettingsLimits `json:"limits,omitempty"`
}

type LimitsDecodeError struct{ Err error }

func (e LimitsDecodeError) Error() string { return "invalid limits object: " + e.Err.Error() }

type settingsDocument struct {
	ExcludeGeneratedDirectories   json.RawMessage `json:"exclude_generated_directories"`
	AdditionalExcludedDirectories json.RawMessage `json:"additional_excluded_directories"`
	FirstRunOnboardingCompleted   bool            `json:"first_run_onboarding_completed"`
	WhitespaceMode                string          `json:"whitespace_mode,omitempty"`
	Limits                        json.RawMessage `json:"limits"`
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
	decodeDirectories(&settings, document.ExcludeGeneratedDirectories, document.AdditionalExcludedDirectories)
	if len(document.Limits) > 0 && string(document.Limits) != "null" {
		var limits SettingsLimits
		if err := json.Unmarshal(document.Limits, &limits); err != nil {
			return settings, LimitsDecodeError{Err: err}
		}
		settings.Limits = &limits
	}
	return settings, nil
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

func decodeDirectories(settings *Settings, toggle, names json.RawMessage) {
	if len(toggle) > 0 {
		var enabled bool
		if string(toggle) == "null" || json.Unmarshal(toggle, &enabled) != nil {
			settings.Warnings = append(settings.Warnings, "settings exclude_generated_directories must be a boolean; keeping the default")
		} else {
			settings.ExcludeGeneratedDirectories = &enabled
		}
	}
	if len(names) > 0 {
		var values []json.RawMessage
		if string(names) == "null" || json.Unmarshal(names, &values) != nil {
			settings.Warnings = append(settings.Warnings, "settings additional_excluded_directories must be an array of directory names; keeping the default")
			return
		}
		valid := []string{}
		for _, raw := range values {
			var name string
			if json.Unmarshal(raw, &name) != nil || dirpolicy.ValidateName(name) != nil {
				settings.Warnings = append(settings.Warnings, "settings additional_excluded_directories contains an invalid directory name; ignoring that entry")
				continue
			}
			valid = append(valid, name)
		}
		settings.AdditionalExcludedDirectories = &valid
	}
}

func (s Settings) DirectoryPolicy() dirpolicy.Policy {
	var names []string
	if s.AdditionalExcludedDirectories != nil {
		names = *s.AdditionalExcludedDirectories
	}
	return dirpolicy.New(s.ExcludeGeneratedDirectories, names)
}

// LoadDirectoryPolicy reads only user-level settings. Missing files use defaults.
// Partially invalid unrelated settings do not discard valid discovery options.
func LoadDirectoryPolicy(path string) (dirpolicy.Policy, []string) {
	if path == "" {
		return dirpolicy.Policy{}, nil
	}
	s, err := LoadSettings(path)
	warnings := append([]string(nil), s.Warnings...)
	if err != nil && !os.IsNotExist(err) {
		warnings = append(warnings, fmt.Sprintf("Could not fully load settings from %s: %v", path, err))
	}
	return s.DirectoryPolicy(), warnings
}

func DefaultDirectoryPolicy() (dirpolicy.Policy, []string) {
	path, err := DefaultSettingsPath()
	if err != nil {
		return dirpolicy.Policy{}, []string{err.Error()}
	}
	return LoadDirectoryPolicy(path)
}
