package badger

import (
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

func TestDefaultConfigUsesOSSDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Root == "" {
		t.Fatal("Root is empty")
	}
	if cfg.TUISubtitle == "" {
		t.Fatal("TUISubtitle is empty")
	}
	if cfg.Focus != protocol.FocusCode {
		t.Fatalf("Focus = %q, want %q", cfg.Focus, protocol.FocusCode)
	}
	if len(cfg.ScanFrames) == 0 {
		t.Fatal("ScanFrames is empty")
	}
	if cfg.ExitCommand != workflow.ExitCommand {
		t.Fatalf("ExitCommand = %q, want %q", cfg.ExitCommand, workflow.ExitCommand)
	}
	if cfg.LargeProjectFileThreshold != workflow.LargeProjectFileThreshold {
		t.Fatalf("LargeProjectFileThreshold = %d, want %d", cfg.LargeProjectFileThreshold, workflow.LargeProjectFileThreshold)
	}
	if cfg.LargePromptByteThreshold != workflow.LargePromptBytes {
		t.Fatalf("LargePromptByteThreshold = %d, want %d", cfg.LargePromptByteThreshold, workflow.LargePromptBytes)
	}
	if cfg.TruncatedMaxPackages != workflow.TruncatedMaxPackages {
		t.Fatalf("TruncatedMaxPackages = %d, want %d", cfg.TruncatedMaxPackages, workflow.TruncatedMaxPackages)
	}
	if cfg.MaxContextFileBytes != workflow.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", cfg.MaxContextFileBytes, workflow.MaxContextFileBytes)
	}
	if cfg.MaxTopologyPromptBytes != workflow.MaxTopologyPromptBytes {
		t.Fatalf("MaxTopologyPromptBytes = %d, want %d", cfg.MaxTopologyPromptBytes, workflow.MaxTopologyPromptBytes)
	}
	if cfg.MaxPromptTwoBytes != 0 {
		t.Fatalf("MaxPromptTwoBytes = %d, want 0 (DefaultConfig leaves this zero so legacy callers can override)", cfg.MaxPromptTwoBytes)
	}
	if cfg.MaxTotalContextBytes != workflow.MaxPromptTwoBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d (legacy default)", cfg.MaxTotalContextBytes, workflow.MaxPromptTwoBytes)
	}
	if cfg.WhitespaceMode != string(writer.DefaultWhitespaceMode) {
		t.Fatalf("WhitespaceMode = %q, want %q", cfg.WhitespaceMode, writer.DefaultWhitespaceMode)
	}
}

func TestConfigCanDisableContextLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextFileBytes = -1
	cfg.MaxTopologyPromptBytes = -1
	cfg.MaxPromptTwoBytes = -1

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxContextFileBytes != -1 {
		t.Fatalf("MaxContextFileBytes = %d, want disabled", tuiCfg.MaxContextFileBytes)
	}
	if tuiCfg.MaxTopologyPromptBytes != -1 {
		t.Fatalf("MaxTopologyPromptBytes = %d, want disabled", tuiCfg.MaxTopologyPromptBytes)
	}
	if tuiCfg.MaxPromptTwoBytes != -1 {
		t.Fatalf("MaxPromptTwoBytes = %d, want disabled", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestConfigZeroContextLimitsUseDefaults(t *testing.T) {
	tuiCfg := Config{}.tuiConfig()
	if tuiCfg.Focus != protocol.FocusCode {
		t.Fatalf("Focus = %q, want %q", tuiCfg.Focus, protocol.FocusCode)
	}
	if tuiCfg.MaxContextFileBytes != workflow.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", tuiCfg.MaxContextFileBytes, workflow.MaxContextFileBytes)
	}
	if tuiCfg.MaxTopologyPromptBytes != workflow.MaxTopologyPromptBytes {
		t.Fatalf("MaxTopologyPromptBytes = %d, want %d", tuiCfg.MaxTopologyPromptBytes, workflow.MaxTopologyPromptBytes)
	}
	if tuiCfg.MaxPromptTwoBytes != workflow.MaxPromptTwoBytes {
		t.Fatalf("MaxPromptTwoBytes = %d, want %d", tuiCfg.MaxPromptTwoBytes, workflow.MaxPromptTwoBytes)
	}
	if tuiCfg.WhitespaceMode != writer.DefaultWhitespaceMode {
		t.Fatalf("WhitespaceMode = %q, want %q", tuiCfg.WhitespaceMode, writer.DefaultWhitespaceMode)
	}
}

func TestConfigCopiesScanFrames(t *testing.T) {
	cfg := DefaultConfig()
	tuiCfg := cfg.tuiConfig()
	tuiCfg.ScanFrames[0] = "changed"

	if cfg.ScanFrames[0] == "changed" {
		t.Fatal("tuiConfig shared ScanFrames backing array with public config")
	}
}

func TestConfigCustomPromptInstructions(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SchemaAConstraint = "custom a"
	cfg.SchemaBConstraint = "custom b"

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.SchemaAConstraint != "custom a" {
		t.Fatalf("SchemaAConstraint = %q, want custom a", tuiCfg.SchemaAConstraint)
	}
	if tuiCfg.SchemaBConstraint != "custom b" {
		t.Fatalf("SchemaBConstraint = %q, want custom b", tuiCfg.SchemaBConstraint)
	}
}

func TestConfigPassesCustomMetadataToTUI(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TUIVersion = "v9.9.9-test"
	cfg.BuildInfo = "badger v9.9.9-test (custom)"
	cfg.Focus = protocol.FocusDesign

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.Version != "v9.9.9-test" {
		t.Fatalf("Version = %q, want custom version", tuiCfg.Version)
	}
	if tuiCfg.BuildInfo != "badger v9.9.9-test (custom)" {
		t.Fatalf("BuildInfo = %q, want custom build info", tuiCfg.BuildInfo)
	}
	if tuiCfg.Focus != protocol.FocusDesign {
		t.Fatalf("Focus = %q, want %q", tuiCfg.Focus, protocol.FocusDesign)
	}
}

func TestLegacyMaxTotalContextBytesZeroValueStruct(t *testing.T) {
	// Old caller style: zero-value Config with only the legacy field set.
	root := t.TempDir()
	cfg := Config{
		Root:                 root,
		MaxTotalContextBytes: 999,
	}

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != 999 {
		t.Fatalf("MaxPromptTwoBytes = %d, want 999 (legacy from zero-value struct)", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesFromDefaultConfig(t *testing.T) {
	// Old caller style: DefaultConfig then mutate only the legacy field.
	cfg := DefaultConfig()
	cfg.MaxTotalContextBytes = 999

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != 999 {
		t.Fatalf("MaxPromptTwoBytes = %d, want 999 (legacy after DefaultConfig)", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesNewFieldWinsExplicit(t *testing.T) {
	// Both fields non-zero — new field must win.
	cfg := Config{
		MaxPromptTwoBytes:    111,
		MaxTotalContextBytes: 999,
	}

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != 111 {
		t.Fatalf("MaxPromptTwoBytes = %d, want 111 (new field wins both non-zero)", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesNewFieldWinsDefaultValue(t *testing.T) {
	// New field set to exactly the default must still win.
	cfg := DefaultConfig()
	cfg.MaxPromptTwoBytes = workflow.MaxPromptTwoBytes
	cfg.MaxTotalContextBytes = 999

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != workflow.MaxPromptTwoBytes {
		t.Fatalf("MaxPromptTwoBytes = %d, want %d (new field wins even when equal to default)", tuiCfg.MaxPromptTwoBytes, workflow.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesBothZeroUsesDefault(t *testing.T) {
	tuiCfg := Config{}.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != workflow.MaxPromptTwoBytes {
		t.Fatalf("MaxPromptTwoBytes = %d, want %d (default when both zero)", tuiCfg.MaxPromptTwoBytes, workflow.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesNegativeNewField(t *testing.T) {
	cfg := Config{MaxPromptTwoBytes: -1}
	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != -1 {
		t.Fatalf("MaxPromptTwoBytes = %d, want -1 (disabled via new field)", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesNegativeLegacyField(t *testing.T) {
	cfg := Config{MaxTotalContextBytes: -1}
	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != -1 {
		t.Fatalf("MaxPromptTwoBytes = %d, want -1 (disabled via legacy)", tuiCfg.MaxPromptTwoBytes)
	}
}

func TestLegacyMaxTotalContextBytesDoesNotAffectPromptOne(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxTotalContextBytes = 999

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxTopologyPromptBytes != workflow.MaxTopologyPromptBytes {
		t.Fatalf("MaxTopologyPromptBytes = %d, want %d (Prompt 1 unaffected)", tuiCfg.MaxTopologyPromptBytes, workflow.MaxTopologyPromptBytes)
	}
}
