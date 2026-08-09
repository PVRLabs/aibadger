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
		t.Fatalf("MaxPromptTwoBytes = %d, want 0 (resolved to the default by withDefaults)", cfg.MaxPromptTwoBytes)
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

func TestZeroPromptTwoLimitUsesDefault(t *testing.T) {
	tuiCfg := Config{}.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != workflow.MaxPromptTwoBytes {
		t.Fatalf("MaxPromptTwoBytes = %d, want %d", tuiCfg.MaxPromptTwoBytes, workflow.MaxPromptTwoBytes)
	}
}

func TestNegativePromptTwoLimitDisablesTrimming(t *testing.T) {
	cfg := Config{MaxPromptTwoBytes: -1}
	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxPromptTwoBytes != -1 {
		t.Fatalf("MaxPromptTwoBytes = %d, want -1 (disabled via new field)", tuiCfg.MaxPromptTwoBytes)
	}
}
