package tui

// This file owns TUI configuration defaults and adapters into shared workflow
// options.

import (
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

type Config struct {
	Subtitle                  string
	Version                   string
	BuildInfo                 string
	Focus                     protocol.Focus
	StartupGoal               string
	StartupStatus             string
	StartupStatusSeverity     string
	SkipOnboarding            bool
	ScanFrames                []string
	ExitCommand               string
	SettingsPath              string
	LargeProjectFileThreshold int
	LargePromptByteThreshold  int
	TruncatedMaxPackages      int
	MaxContextFileBytes       int // 0 uses the default; negative disables per-file trimming.
	MaxTotalContextBytes      int // 0 uses the default; negative disables total context trimming.
	SchemaAConstraint         string
	SchemaBConstraint         string
	WhitespaceMode            writer.WhitespaceMode
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
		MaxTotalContextBytes:      workflow.MaxTotalContextBytes,
		WhitespaceMode:            writer.DefaultWhitespaceMode,
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
	if c.MaxTotalContextBytes == 0 {
		c.MaxTotalContextBytes = defaults.MaxTotalContextBytes
	}
	if c.WhitespaceMode == "" {
		c.WhitespaceMode = defaults.WhitespaceMode
	}
	return c
}

func (m Model) engineOptions(maxPackages int) workflow.EngineOptions {
	return workflow.EngineOptions{
		MaxContextFileBytes:  m.cfg.MaxContextFileBytes,
		MaxTotalContextBytes: m.cfg.MaxTotalContextBytes,
		MaxPackages:          maxPackages,
		Focus:                m.cfg.Focus,
		SchemaAConstraint:    m.cfg.SchemaAConstraint,
		SchemaBConstraint:    m.cfg.SchemaBConstraint,
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
		mascotFrame("Sniffing around...", "o.o"),
		mascotFrame("Found some trails...", "-.-"),
		mascotFrame("Almost there...", "o.o"),
	}
}
