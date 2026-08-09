package badger

import (
	"os"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/internal/tui"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

type StartupContext = startup.Context
type StartupAttachment = startup.Attachment
type StartupStatus = startup.Status

// Config is the public integration surface for launching Badger.
type Config struct {
	Root                      string
	TUISubtitle               string
	TUIVersion                string
	BuildInfo                 string
	Focus                     protocol.Focus
	Startup                   StartupContext
	SkipOnboarding            bool
	ScanFrames                []string
	ExitCommand               string
	SettingsPath              string
	LargeProjectFileThreshold int
	LargePromptByteThreshold  int
	TruncatedMaxPackages      int
	MaxContextFileBytes       int    // 0 uses the default; negative disables per-file trimming.
	MaxTopologyPromptBytes    int    // 0 uses the default; negative disables Prompt 1 byte target.
	MaxPromptTwoBytes         int    // 0 uses the default; negative disables Prompt 2 trimming.
	SchemaAConstraint         string // Optional: overrides Prompt 1 instructions
	SchemaBConstraint         string // Optional: overrides Prompt 2 instructions
	WhitespaceMode            string // "smart" (default), "exact", or "ignore"
	MaxFilesPerDirectory      int
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
		Focus:                     protocol.FocusCode,
		ExitCommand:               workflow.ExitCommand,
		SettingsPath:              settingsPath,
		LargeProjectFileThreshold: workflow.LargeProjectFileThreshold,
		LargePromptByteThreshold:  workflow.LargePromptBytes,
		TruncatedMaxPackages:      workflow.TruncatedMaxPackages,
		MaxContextFileBytes:       workflow.MaxContextFileBytes,
		MaxTopologyPromptBytes:    workflow.MaxTopologyPromptBytes,
		WhitespaceMode:            string(writer.DefaultWhitespaceMode),
		MaxFilesPerDirectory:      workflow.MaxFilesPerDirectory,
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
	c.Focus = protocol.NormalizeFocus(c.Focus)
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
	if c.MaxTopologyPromptBytes == 0 {
		c.MaxTopologyPromptBytes = defaults.MaxTopologyPromptBytes
	}
	resolved := c.MaxPromptTwoBytes
	if resolved == 0 {
		resolved = workflow.MaxPromptTwoBytes
	}
	c.MaxPromptTwoBytes = resolved
	if c.MaxFilesPerDirectory == 0 {
		c.MaxFilesPerDirectory = defaults.MaxFilesPerDirectory
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
		Focus:                     c.Focus,
		Startup:                   c.Startup,
		SkipOnboarding:            c.SkipOnboarding,
		ScanFrames:                append([]string(nil), c.ScanFrames...),
		ExitCommand:               c.ExitCommand,
		SettingsPath:              c.SettingsPath,
		LargeProjectFileThreshold: c.LargeProjectFileThreshold,
		LargePromptByteThreshold:  c.LargePromptByteThreshold,
		TruncatedMaxPackages:      c.TruncatedMaxPackages,
		MaxContextFileBytes:       c.MaxContextFileBytes,
		MaxTopologyPromptBytes:    c.MaxTopologyPromptBytes,
		MaxPromptTwoBytes:         c.MaxPromptTwoBytes,
		SchemaAConstraint:         c.SchemaAConstraint,
		SchemaBConstraint:         c.SchemaBConstraint,
		MaxFilesPerDirectory:      c.MaxFilesPerDirectory,
		WhitespaceMode:            writer.WhitespaceMode(c.WhitespaceMode),
	}
}
