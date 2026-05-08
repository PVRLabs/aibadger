package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const settingsDirName = ".badger"

type Settings struct {
	FirstRunOnboardingCompleted bool   `json:"first_run_onboarding_completed"`
	WhitespaceMode              string `json:"whitespace_mode,omitempty"`
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

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		return Settings{}, err
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
