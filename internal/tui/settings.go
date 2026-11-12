package tui

import (
	"errors"
	"fmt"
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
	resolved := SettingsLimits{
		MaxTopFilesPerPackage:  cfg.MaxTopFilesPerPackage,
		MaxFilesPerDirectory:   cfg.MaxFilesPerDirectory,
		MaxContextFileBytes:    cfg.MaxContextFileBytes,
		MaxPromptTwoBytes:      cfg.MaxPromptTwoBytes,
		MaxTopologyPromptBytes: cfg.MaxTopologyPromptBytes,
	}
	var warnings []string
	sharedsettings.MergeLimits(&resolved, limits, func(message string) { warnings = append(warnings, message) })
	cfg.MaxTopFilesPerPackage = resolved.MaxTopFilesPerPackage
	cfg.MaxFilesPerDirectory = resolved.MaxFilesPerDirectory
	cfg.MaxContextFileBytes = resolved.MaxContextFileBytes
	cfg.MaxPromptTwoBytes = resolved.MaxPromptTwoBytes
	cfg.MaxTopologyPromptBytes = resolved.MaxTopologyPromptBytes

	return cfg, warnings
}

func settingsLoadWarning(path string, err error) string {
	var limitsErr limitsDecodeError
	if errors.As(err, &limitsErr) {
		return fmt.Sprintf("Could not apply limits from %s because the limits object is invalid; keeping the current limits.", path)
	}
	return fmt.Sprintf("Could not load settings from %s; keeping the current settings.", path)
}
