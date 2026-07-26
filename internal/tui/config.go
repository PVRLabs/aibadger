package tui

// This file owns TUI configuration defaults and adapters into shared workflow
// options.

import (
	"github.com/PVRLabs/aibadger/internal/brand"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

type Config struct {
	Subtitle                  string
	Version                   string
	BuildInfo                 string
	Focus                     protocol.Focus
	Startup                   startup.Context
	SkipOnboarding            bool
	ScanFrames                []string
	ExitCommand               string
	SettingsPath              string
	LargeProjectFileThreshold int
	LargePromptByteThreshold  int
	TruncatedMaxPackages      int
	MaxContextFileBytes       int // 0 uses the default; negative disables per-file trimming.
	MaxTopologyPromptBytes    int // 0 uses the default; negative disables Prompt 1 byte target.
	MaxPromptTwoBytes         int // 0 uses the default; negative disables Prompt 2 byte target.
	SchemaAConstraint         string
	SchemaBConstraint         string
	WhitespaceMode            writer.WhitespaceMode
	MaxFilesPerDirectory      int
}

func DefaultConfig() Config {
	return Config{
		Subtitle:                  "Local-first code context for any AI chat",
		Version:                   version.Version,
		Focus:                     protocol.FocusCode,
		ScanFrames:                defaultScanFrames(),
		ExitCommand:               workflow.ExitCommand,
		LargeProjectFileThreshold: workflow.LargeProjectFileThreshold,
		LargePromptByteThreshold:  workflow.LargePromptBytes,
		TruncatedMaxPackages:      workflow.TruncatedMaxPackages,
		MaxContextFileBytes:       workflow.MaxContextFileBytes,
		MaxTopologyPromptBytes:    workflow.MaxTopologyPromptBytes,
		MaxPromptTwoBytes:         workflow.MaxPromptTwoBytes,
		WhitespaceMode:            writer.DefaultWhitespaceMode,
		MaxFilesPerDirectory:      workflow.MaxFilesPerDirectory,
	}
}

func (c Config) withDefaults() Config {
	defaults := DefaultConfig()
	if c.Subtitle == "" {
		c.Subtitle = defaults.Subtitle
	}
	if c.Version == "" {
		c.Version = defaults.Version
	}
	c.Focus = protocol.NormalizeFocus(c.Focus)
	if len(c.ScanFrames) == 0 {
		c.ScanFrames = defaults.ScanFrames
	}
	if c.ExitCommand == "" {
		c.ExitCommand = defaults.ExitCommand
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
	if c.MaxPromptTwoBytes == 0 {
		c.MaxPromptTwoBytes = defaults.MaxPromptTwoBytes
	}
	if c.MaxFilesPerDirectory == 0 {
		c.MaxFilesPerDirectory = defaults.MaxFilesPerDirectory
	}
	if c.WhitespaceMode == "" {
		c.WhitespaceMode = defaults.WhitespaceMode
	}
	return c
}

func (m Model) engineOptions(maxPackages int) workflow.EngineOptions {
	return workflow.EngineOptions{
		MaxContextFileBytes:    m.cfg.MaxContextFileBytes,
		MaxTopologyPromptBytes: m.cfg.MaxTopologyPromptBytes,
		MaxPromptTwoBytes:      m.cfg.MaxPromptTwoBytes,
		MaxPackages:            maxPackages,
		Focus:                  m.cfg.Focus,
		SchemaAConstraint:      m.cfg.SchemaAConstraint,
		SchemaBConstraint:      m.cfg.SchemaBConstraint,
	}
}

func (m *Model) workflowSession() *workflow.Session {
	if m.session == nil {
		m.session = workflow.NewSession(m.eng, m.cfg.WhitespaceMode)
	}
	if m.session.Engine == nil && m.eng != nil {
		m.session.Engine = m.eng
	}
	m.session.WhitespaceMode = m.cfg.WhitespaceMode
	return m.session
}

func defaultScanFrames() []string {
	return []string{
		brand.MascotFrame("Sniffing around...", "o.o"),
		brand.MascotFrame("Found some trails...", "-.-"),
		brand.MascotFrame("Almost there...", "o.o"),
	}
}
