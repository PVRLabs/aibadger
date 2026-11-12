package tui

import (
	"errors"
	"fmt"
	"github.com/PVRLabs/aibadger/internal/defaults"
	sharedsettings "github.com/PVRLabs/aibadger/internal/settings"
)

type Settings = sharedsettings.Settings
type SettingsLimits = sharedsettings.SettingsLimits
type limitsDecodeError = sharedsettings.LimitsDecodeError

func DefaultSettingsPath() (string, error)       { return sharedsettings.DefaultSettingsPath() }
func LoadSettings(path string) (Settings, error) { return sharedsettings.LoadSettings(path) }
func SaveSettings(path string, s Settings) error { return sharedsettings.SaveSettings(path, s) }

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
