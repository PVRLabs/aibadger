package badger

import (
	"testing"

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
	if cfg.MaxTotalContextBytes != workflow.MaxTotalContextBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d", cfg.MaxTotalContextBytes, workflow.MaxTotalContextBytes)
	}
	if cfg.WhitespaceMode != string(writer.DefaultWhitespaceMode) {
		t.Fatalf("WhitespaceMode = %q, want %q", cfg.WhitespaceMode, writer.DefaultWhitespaceMode)
	}
}

func TestConfigCanDisableContextLimits(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxContextFileBytes = -1
	cfg.MaxTotalContextBytes = -1

	tuiCfg := cfg.tuiConfig()
	if tuiCfg.MaxContextFileBytes != -1 {
		t.Fatalf("MaxContextFileBytes = %d, want disabled", tuiCfg.MaxContextFileBytes)
	}
	if tuiCfg.MaxTotalContextBytes != -1 {
		t.Fatalf("MaxTotalContextBytes = %d, want disabled", tuiCfg.MaxTotalContextBytes)
	}
}

func TestConfigZeroContextLimitsUseDefaults(t *testing.T) {
	tuiCfg := Config{}.tuiConfig()
	if tuiCfg.MaxContextFileBytes != workflow.MaxContextFileBytes {
		t.Fatalf("MaxContextFileBytes = %d, want %d", tuiCfg.MaxContextFileBytes, workflow.MaxContextFileBytes)
	}
	if tuiCfg.MaxTotalContextBytes != workflow.MaxTotalContextBytes {
		t.Fatalf("MaxTotalContextBytes = %d, want %d", tuiCfg.MaxTotalContextBytes, workflow.MaxTotalContextBytes)
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
