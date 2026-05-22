package main

import (
	"bytes"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/version"
)

func TestLoadConfigHelp(t *testing.T) {
	cfg := loadConfig([]string{"--help"})

	if !cfg.showHelp {
		t.Fatal("loadConfig() did not enable showHelp for --help")
	}
}

func TestLoadConfigVersionFlag(t *testing.T) {
	cfg := loadConfig([]string{"--version"})

	if !cfg.showVersion {
		t.Fatal("loadConfig() did not enable showVersion for --version")
	}
}

func TestLoadConfigVersionCommand(t *testing.T) {
	cfg := loadConfig([]string{"version"})

	if !cfg.showVersion {
		t.Fatal("loadConfig() did not enable showVersion for version command")
	}
}

func TestLoadConfigFocusCommand(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		focus protocol.Focus
	}{
		{name: "design", args: []string{"design"}, focus: protocol.FocusDesign},
		{name: "review", args: []string{"review", "--headless"}, focus: protocol.FocusReview},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadConfig(tt.args)
			if cfg.focus != tt.focus {
				t.Fatalf("focus = %q, want %q", cfg.focus, tt.focus)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	var out bytes.Buffer

	printVersion(&out)

	if got, want := out.String(), "badger "+version.Version+"\n"; got != want {
		t.Fatalf("printVersion() = %q, want %q", got, want)
	}
}

func TestLoadConfigHeadlessDevStepInput(t *testing.T) {
	cfg := loadConfig([]string{"--headless", "--step", "extraction", "--input", "commands.txt", "--truncate-topology"})

	if !releaseBuild {
		if !cfg.headless {
			t.Fatal("headless = false, want true")
		}
		if cfg.stepFlag != "extraction" {
			t.Fatalf("stepFlag = %q, want %q", cfg.stepFlag, "extraction")
		}
		if cfg.inputFlag != "commands.txt" {
			t.Fatalf("inputFlag = %q, want %q", cfg.inputFlag, "commands.txt")
		}
		if !cfg.truncateTopology {
			t.Fatal("truncateTopology = false, want true")
		}
	}
}

func TestLoadConfigParsesHeadlessOnlyFlagsWithoutHeadless(t *testing.T) {
	cfg := loadConfig([]string{"--step", "extraction", "--input", "commands.txt", "--truncate-topology"})

	if cfg.headless {
		t.Fatal("headless = true without --headless")
	}
	if !hasHeadlessOnlyFlagsWithoutHeadless(cfg) {
		t.Fatalf("hasHeadlessOnlyFlagsWithoutHeadless() = false for step=%q input=%q truncateTopology=%v", cfg.stepFlag, cfg.inputFlag, cfg.truncateTopology)
	}
}

func TestUsedDevOnlyFlags(t *testing.T) {
	got := usedDevOnlyFlags([]string{
		"--step",
		"topology",
		"-input=commands.txt",
		"--headless",
		"--step=context",
		"-truncate-topology",
	})
	want := []string{"--step", "--input", "--headless", "--truncate-topology"}

	if len(got) != len(want) {
		t.Fatalf("usedDevOnlyFlags() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("usedDevOnlyFlags() = %v, want %v", got, want)
		}
	}
}

func TestUsedDevOnlyFlagsDoesNotMatchPrefixes(t *testing.T) {
	got := usedDevOnlyFlags([]string{"--stepper", "--input-file", "--headless-mode", "--truncate-topology-extra"})

	if len(got) != 0 {
		t.Fatalf("usedDevOnlyFlags() = %v, want none", got)
	}
}

func TestUsedDevOnlyFlagsStopsAtOptionTerminator(t *testing.T) {
	got := usedDevOnlyFlags([]string{"--", "--headless", "--step=topology"})

	if len(got) != 0 {
		t.Fatalf("usedDevOnlyFlags() = %v, want none", got)
	}
}

func TestLoadConfigParseError(t *testing.T) {
	cfg := loadConfig([]string{"--step"})

	if cfg.parseErr == nil {
		t.Fatal("parseErr = nil, want missing value error")
	}
}
