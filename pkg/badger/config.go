package badger

import (
	"os"

	"github.com/PVRLabs/aibadger/internal/tui"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

// Config is the public integration surface for launching Badger.
type Config struct {
	Root                      string
	TUISubtitle               string
	TUIVersion                string
	BuildInfo                 string
	ScanFrames                []string
	ExitCommand               string
	SettingsPath              string
	LargeProjectFileThreshold int
	LargePromptByteThreshold  int
	TruncatedMaxPackages      int
	MaxContextFileBytes       int    // 0 uses the default; negative disables per-file trimming.
	MaxTotalContextBytes      int    // 0 uses the default; negative disables total context trimming.
	SchemaAConstraint         string // Optional: overrides Prompt 1 instructions
	SchemaBConstraint         string // Optional: overrides Prompt 2 instructions
	WhitespaceMode            string // "smart" (default), "exact", or "ignore"
}

// DefaultConfig returns the OSS defaults used by the badger command.
func DefaultConfig() Config {
	root, _ := os.Getwd()
	tuiCfg := tui.DefaultConfig()
	settingsPath, _ := tui.DefaultSettingsPath()
	return Config{
		Root:                      root,
		TUISubtitle:               tuiCfg.Subtitle,
		TUIVersion:                tuiCfg.Version,
		ScanFrames:                append([]string(nil), tuiCfg.ScanFrames...),
		ExitCommand:               workflow.ExitCommand,
		SettingsPath:              settingsPath,
		LargeProjectFileThreshold: workflow.LargeProjectFileThreshold,
		LargePromptByteThreshold:  workflow.LargePromptBytes,
		TruncatedMaxPackages:      workflow.TruncatedMaxPackages,
		MaxContextFileBytes:       workflow.MaxContextFileBytes,
		MaxTotalContextBytes:      workflow.MaxTotalContextBytes,
		WhitespaceMode:            string(writer.DefaultWhitespaceMode),
	}
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.Root == "" {
		c.Root = defaults.Root
	}
	if c.TUISubtitle == "" {
		c.TUISubtitle = defaults.TUISubtitle
	}
	if c.TUIVersion == "" {
		c.TUIVersion = defaults.TUIVersion
	}
	if len(c.ScanFrames) == 0 {
		c.ScanFrames = append([]string(nil), defaults.ScanFrames...)
	}
	if c.ExitCommand == "" {
		c.ExitCommand = defaults.ExitCommand
	}
	if c.SettingsPath == "" {
		c.SettingsPath = defaults.SettingsPath
	}
	if c.LargeProjectFileThreshold == 0 {
		c.LargeProjectFileThreshold = defaults.LargeProjectFileThreshold
	}
	if c.LargePromptByteThreshold == 0 {
		c.LargePromptByteThreshold = defaults.LargePromptByteThreshold
	}
	if c.TruncatedMaxPackages == 0 {
		c.TruncatedMaxPackages = defaults.TruncatedMaxPackages
	}
	if c.MaxContextFileBytes == 0 {
		c.MaxContextFileBytes = defaults.MaxContextFileBytes
	}
	if c.MaxTotalContextBytes == 0 {
		c.MaxTotalContextBytes = defaults.MaxTotalContextBytes
	}
	if c.WhitespaceMode == "" {
		c.WhitespaceMode = defaults.WhitespaceMode
	}
	return c
}

func (c Config) tuiConfig() tui.Config {
	c = c.withDefaults()
	return tui.Config{
		Subtitle:                  c.TUISubtitle,
		Version:                   c.TUIVersion,
		BuildInfo:                 c.BuildInfo,
		ScanFrames:                append([]string(nil), c.ScanFrames...),
		ExitCommand:               c.ExitCommand,
		SettingsPath:              c.SettingsPath,
		LargeProjectFileThreshold: c.LargeProjectFileThreshold,
		LargePromptByteThreshold:  c.LargePromptByteThreshold,
		TruncatedMaxPackages:      c.TruncatedMaxPackages,
		MaxContextFileBytes:       c.MaxContextFileBytes,
		MaxTotalContextBytes:      c.MaxTotalContextBytes,
		SchemaAConstraint:         c.SchemaAConstraint,
		SchemaBConstraint:         c.SchemaBConstraint,
		WhitespaceMode:            writer.WhitespaceMode(c.WhitespaceMode),
	}
}
