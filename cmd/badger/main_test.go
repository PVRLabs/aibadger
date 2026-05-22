package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/pkg/badger"
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

func TestLoadConfigReviewFlagsAndExtraFocus(t *testing.T) {
	cfg := loadConfig([]string{"review", "--branch", "main", "Check error handling and nil guards."})

	if cfg.focus != protocol.FocusReview {
		t.Fatalf("focus = %q, want %q", cfg.focus, protocol.FocusReview)
	}
	if cfg.reviewMode != reviewtask.ModeBranch {
		t.Fatalf("reviewMode = %v, want %v", cfg.reviewMode, reviewtask.ModeBranch)
	}
	if cfg.reviewRef != "main" {
		t.Fatalf("reviewRef = %q, want %q", cfg.reviewRef, "main")
	}
	if cfg.reviewExtraFocus != "Check error handling and nil guards." {
		t.Fatalf("reviewExtraFocus = %q, want %q", cfg.reviewExtraFocus, "Check error handling and nil guards.")
	}
}

func TestLoadConfigReviewPositionalExtraFocus(t *testing.T) {
	cfg := loadConfig([]string{"review", "Check", "error", "paths"})

	if cfg.reviewMode != reviewtask.ModeDefault {
		t.Fatalf("reviewMode = %v, want default", cfg.reviewMode)
	}
	if cfg.reviewExtraFocus != "Check error paths" {
		t.Fatalf("reviewExtraFocus = %q, want %q", cfg.reviewExtraFocus, "Check error paths")
	}
}

func TestLoadConfigReviewFlagsAreMutuallyExclusive(t *testing.T) {
	cfg := loadConfig([]string{"review", "--staged", "--branch", "main"})

	if cfg.parseErr == nil {
		t.Fatal("parseErr = nil, want mutually exclusive review flag error")
	}
}

func TestApplyReviewStartupUsesReviewPrompt(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"updated\")\n}\n")
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	app := appConfig{
		focus:            protocol.FocusReview,
		reviewMode:       reviewtask.ModeDefault,
		reviewExtraFocus: "Check edge cases.",
	}

	if err := applyReviewStartup(&cfg, app); err != nil {
		t.Fatalf("applyReviewStartup() error = %v", err)
	}
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.StartupGoal == "" {
		t.Fatal("StartupGoal is empty")
	}
	if cfg.StartupStatusSeverity != "success" {
		t.Fatalf("StartupStatusSeverity = %q, want %q", cfg.StartupStatusSeverity, "success")
	}
	if cfg.StartupStatus == "" {
		t.Fatal("StartupStatus is empty")
	}
}

func TestApplyReviewStartupUsesFallbackPromptWhenNoDiff(t *testing.T) {
	repo := newGitRepo(t)
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	app := appConfig{focus: protocol.FocusReview}

	if err := applyReviewStartup(&cfg, app); err != nil {
		t.Fatalf("applyReviewStartup() error = %v", err)
	}
	if cfg.StartupStatusSeverity != "warning" {
		t.Fatalf("StartupStatusSeverity = %q, want %q", cfg.StartupStatusSeverity, "warning")
	}
	if cfg.StartupGoal == "" {
		t.Fatal("StartupGoal is empty")
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "checkout", "-b", "main")
	runGitCmd(t, dir, "config", "user.name", "Badger Test")
	runGitCmd(t, dir, "config", "user.email", "badger@example.com")
	writeFile(t, dir, "app.go", "package main\n\nfunc main() {\n\tprintln(\"base\")\n}\n")
	runGitCmd(t, dir, "add", "app.go")
	runGitCmd(t, dir, "commit", "-m", "initial commit")
	return dir
}

func writeFile(t *testing.T, dir, path, contents string) {
	t.Helper()

	fullPath := filepath.Join(dir, path)
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", fullPath, err)
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Badger Test",
		"GIT_AUTHOR_EMAIL=badger@example.com",
		"GIT_COMMITTER_NAME=Badger Test",
		"GIT_COMMITTER_EMAIL=badger@example.com",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
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
