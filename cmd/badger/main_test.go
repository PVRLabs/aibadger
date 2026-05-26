package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/github"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout pipe error = %v", err)
	}
	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		t.Fatalf("read stdout pipe error = %v", err)
	}
	return out.String()
}

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

func TestLoadConfigBadgeCommand(t *testing.T) {
	cfg := loadConfig([]string{"badge"})

	if !cfg.showBadge {
		t.Fatal("showBadge = false, want true")
	}
	if cfg.focus != "" {
		t.Fatalf("focus = %q, want empty for badge command", cfg.focus)
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

func TestApplyDesignStartupInteractive(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	app := appConfig{
		focus: protocol.FocusDesign,
	}

	goal := applyDesignStartup(&cfg, app)
	if goal != "" {
		t.Fatalf("design goal = %q, want empty for interactive startup", goal)
	}
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.StartupGoal != protocol.DefaultDesignPrompt {
		t.Fatalf("StartupGoal = %q, want %q", cfg.StartupGoal, protocol.DefaultDesignPrompt)
	}
	if cfg.StartupStatusSeverity != "success" {
		t.Fatalf("StartupStatusSeverity = %q, want %q", cfg.StartupStatusSeverity, "success")
	}
	if cfg.StartupStatus == "" {
		t.Fatal("StartupStatus is empty")
	}
	if !strings.Contains(cfg.StartupStatus, "Design") {
		t.Fatalf("StartupStatus = %q, want message mentioning Design", cfg.StartupStatus)
	}
}

func TestApplyDesignStartupHeadless(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	app := appConfig{
		focus:    protocol.FocusDesign,
		headless: true,
	}

	goal := applyDesignStartup(&cfg, app)
	if goal == "" {
		t.Fatal("headless design goal is empty")
	}
	if !strings.Contains(goal, "Design") {
		t.Fatalf("headless design goal missing design template:\n%s", goal)
	}
	if cfg.StartupGoal != "" {
		t.Fatalf("StartupGoal = %q, want empty for headless startup", cfg.StartupGoal)
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

	if goal, err := applyReviewStartup(&cfg, app); err != nil {
		t.Fatalf("applyReviewStartup() error = %v", err)
	} else if goal != "" {
		t.Fatalf("headless review goal = %q, want empty for interactive startup", goal)
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

func TestApplyReviewStartupHeadlessUsesPreparedPrompt(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"updated\")\n}\n")
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	app := appConfig{
		focus:      protocol.FocusReview,
		headless:   true,
		reviewMode: reviewtask.ModeDefault,
	}

	goal, err := applyReviewStartup(&cfg, app)
	if err != nil {
		t.Fatalf("applyReviewStartup() error = %v", err)
	}
	if goal == "" {
		t.Fatal("headless review goal is empty")
	}
	if !strings.Contains(goal, "Diff:") {
		t.Fatalf("headless review goal missing diff prompt:\n%s", goal)
	}
	if cfg.StartupGoal != "" {
		t.Fatalf("StartupGoal = %q, want empty for headless startup", cfg.StartupGoal)
	}
}

func TestApplyReviewStartupHeadlessRejectsNoDiff(t *testing.T) {
	repo := newGitRepo(t)
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	app := appConfig{
		focus:      protocol.FocusReview,
		headless:   true,
		reviewMode: reviewtask.ModeDefault,
	}

	goal, err := applyReviewStartup(&cfg, app)
	if err == nil {
		t.Fatal("applyReviewStartup() error = nil, want no-diff failure")
	}
	if goal != "" {
		t.Fatalf("headless review goal = %q, want empty on failure", goal)
	}
	if !strings.Contains(err.Error(), "no git diff was detected") {
		t.Fatalf("error = %v, want no-diff failure", err)
	}
}

func TestApplyReviewStartupHeadlessRejectsNonGit(t *testing.T) {
	dir := t.TempDir()
	cfg := badger.DefaultConfig()
	cfg.Root = dir
	app := appConfig{
		focus:      protocol.FocusReview,
		headless:   true,
		reviewMode: reviewtask.ModeDefault,
	}

	goal, err := applyReviewStartup(&cfg, app)
	if err == nil {
		t.Fatal("applyReviewStartup() error = nil, want non-git failure")
	}
	if goal != "" {
		t.Fatalf("headless review goal = %q, want empty on failure", goal)
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("error = %v, want non-git failure", err)
	}
}

func TestApplyReviewStartupUsesFallbackPromptWhenNoDiff(t *testing.T) {
	repo := newGitRepo(t)
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	app := appConfig{focus: protocol.FocusReview}

	if _, err := applyReviewStartup(&cfg, app); err != nil {
		t.Fatalf("applyReviewStartup() error = %v", err)
	}
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
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

func TestPrintUsageIncludesReviewEntrypoint(t *testing.T) {
	out := captureStdout(t, func() {
		printUsage(appConfig{})
	})

	for _, want := range []string{
		"badger badge",
		"Show GitHub stargazer scoreboard",
		"badger review [--staged | --branch <ref> | --commit <sha>] [extra focus text]",
		"`badger review` preloads an editable review prompt from the current git diff.",
		"manual fallback prompt in the editor",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("printUsage output missing %q:\n%s", want, out)
		}
	}
}

func TestRunBadgeDeclineDoesNotFetch(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	originalFetch := fetchStargazersFunc
	terminalInteractiveFunc = func() bool { return true }
	fetchCalled := false
	fetchStargazersFunc = func() ([]string, int, error) {
		fetchCalled = true
		return nil, 0, nil
	}
	defer func() {
		terminalInteractiveFunc = originalTerminalInteractive
		fetchStargazersFunc = originalFetch
	}()

	var out bytes.Buffer
	if err := runBadge(strings.NewReader("n\n"), &out, io.Discard); err != nil {
		t.Fatalf("runBadge() error = %v", err)
	}
	if fetchCalled {
		t.Fatal("fetchStargazersFunc was called on decline")
	}
	if got := out.String(); !strings.Contains(got, "👍 No problem! Run 'badger badge' anytime to see the scoreboard.") {
		t.Fatalf("output missing decline message:\n%s", got)
	}
}

func TestRunBadgeSuccessRender(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	originalFetch := fetchStargazersFunc
	terminalInteractiveFunc = func() bool { return true }
	fetchStargazersFunc = func() ([]string, int, error) {
		return []string{"user33", "user34", "user35", "user36", "user37", "user38", "user39", "user40", "user41", "user42"}, 42, nil
	}
	defer func() {
		terminalInteractiveFunc = originalTerminalInteractive
		fetchStargazersFunc = originalFetch
	}()

	var out bytes.Buffer
	if err := runBadge(strings.NewReader("y\n"), &out, io.Discard); err != nil {
		t.Fatalf("runBadge() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"⭐ TOTAL STARS: 42",
		"🌟 Recent supporters (last 10):",
		"@user33",
		"@user42",
		"[S]tar the repo in browser",
		"[Enter] exit",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunBadgeGazillionRender(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	originalFetch := fetchStargazersFunc
	terminalInteractiveFunc = func() bool { return true }
	fetchStargazersFunc = func() ([]string, int, error) {
		return []string{"user091", "user092", "user093", "user094", "user095", "user096", "user097", "user098", "user099", "user100"}, 100, nil
	}
	defer func() {
		terminalInteractiveFunc = originalTerminalInteractive
		fetchStargazersFunc = originalFetch
	}()

	var out bytes.Buffer
	if err := runBadge(strings.NewReader("y\n"), &out, io.Discard); err != nil {
		t.Fatalf("runBadge() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"A GAZILLION BADGERS have starred this repo!",
		"Results may be cached",
		"@user100",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestRunBadgeNetworkErrorRender(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	originalFetch := fetchStargazersFunc
	terminalInteractiveFunc = func() bool { return true }
	fetchStargazersFunc = func() ([]string, int, error) {
		return nil, 0, errors.New("Could not fetch data: timeout")
	}
	defer func() {
		terminalInteractiveFunc = originalTerminalInteractive
		fetchStargazersFunc = originalFetch
	}()

	var out bytes.Buffer
	if err := runBadge(strings.NewReader("y\n"), &out, io.Discard); err != nil {
		t.Fatalf("runBadge() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "❌ Could not fetch data: timeout") {
		t.Fatalf("output missing network error:\n%s", got)
	}
}

func TestRunBadgeRateLimitRender(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	originalFetch := fetchStargazersFunc
	terminalInteractiveFunc = func() bool { return true }
	fetchStargazersFunc = func() ([]string, int, error) {
		return nil, 0, github.ErrRateLimit
	}
	defer func() {
		terminalInteractiveFunc = originalTerminalInteractive
		fetchStargazersFunc = originalFetch
	}()

	var out bytes.Buffer
	if err := runBadge(strings.NewReader("y\n"), &out, io.Discard); err != nil {
		t.Fatalf("runBadge() error = %v", err)
	}
	if got := out.String(); !strings.Contains(got, "⚠️ GitHub API rate limit hit. Try again in an hour.") {
		t.Fatalf("output missing rate-limit error:\n%s", got)
	}
}

func TestRunBadgeRejectsNonInteractive(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	terminalInteractiveFunc = func() bool { return false }
	defer func() { terminalInteractiveFunc = originalTerminalInteractive }()

	if err := runBadge(strings.NewReader(""), io.Discard, io.Discard); err == nil {
		t.Fatal("runBadge() error = nil, want interactive-terminal rejection")
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
