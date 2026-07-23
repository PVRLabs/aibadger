package main

import (
	"bytes"
	"io"
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
		{name: "followup", args: []string{"followup"}, focus: protocol.FocusFollowup},
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

	applyDesignStartup(&cfg, app)
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.Startup.Goal != protocol.DefaultDesignPrompt {
		t.Fatalf("Startup.Goal = %q, want %q", cfg.Startup.Goal, protocol.DefaultDesignPrompt)
	}
	if cfg.Startup.Status.Severity != "success" {
		t.Fatalf("Startup.Status.Severity = %q, want %q", cfg.Startup.Status.Severity, "success")
	}
	if cfg.Startup.Status.Text == "" {
		t.Fatal("Startup.Status.Text is empty")
	}
	if !strings.Contains(cfg.Startup.Status.Text, "Design") {
		t.Fatalf("Startup.Status.Text = %q, want message mentioning Design", cfg.Startup.Status.Text)
	}
}

func TestApplyDesignStartupHeadless(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	app := appConfig{
		focus:    protocol.FocusDesign,
		headless: true,
	}

	applyDesignStartup(&cfg, app)
	if cfg.Startup.Goal == "" {
		t.Fatal("headless design startup goal is empty")
	}
	if !strings.Contains(cfg.Startup.Goal, "Design") {
		t.Fatalf("headless design startup goal missing design template:\n%s", cfg.Startup.Goal)
	}
}

func TestApplyFollowupStartupInteractive(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	app := appConfig{
		focus: protocol.FocusFollowup,
	}

	applyFollowupStartup(&cfg, app)
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.Startup.Goal != protocol.DefaultFollowupPrompt {
		t.Fatalf("Startup.Goal = %q, want %q", cfg.Startup.Goal, protocol.DefaultFollowupPrompt)
	}
	if cfg.Startup.Status.Severity != "success" {
		t.Fatalf("Startup.Status.Severity = %q, want %q", cfg.Startup.Status.Severity, "success")
	}
	if cfg.Startup.Status.Text == "" {
		t.Fatal("Startup.Status.Text is empty")
	}
	if !strings.Contains(cfg.Startup.Status.Text, "Follow-up") {
		t.Fatalf("Startup.Status.Text = %q, want message mentioning Follow-up", cfg.Startup.Status.Text)
	}
}

func TestApplyFollowupStartupHeadless(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	app := appConfig{
		focus:    protocol.FocusFollowup,
		headless: true,
	}

	applyFollowupStartup(&cfg, app)
	if cfg.Startup.Goal == "" {
		t.Fatal("headless follow-up startup goal is empty")
	}
	if !strings.Contains(cfg.Startup.Goal, "Follow-up") {
		t.Fatalf("headless follow-up startup goal missing follow-up template:\n%s", cfg.Startup.Goal)
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
	if cfg.Startup.Goal == "" {
		t.Fatal("Startup.Goal is empty")
	}
	if strings.Contains(cfg.Startup.Goal, "Diff:") {
		t.Fatalf("Startup.Goal unexpectedly contains raw diff text:\n%s", cfg.Startup.Goal)
	}
	if len(cfg.Startup.Attachments) != 1 {
		t.Fatalf("Startup.Attachments length = %d, want 1", len(cfg.Startup.Attachments))
	}
	if cfg.Startup.Attachments[0].Type != "git diff" {
		t.Fatalf("Startup.Attachments[0].Type = %q, want git diff", cfg.Startup.Attachments[0].Type)
	}
	if cfg.Startup.Attachments[0].Text == "" {
		t.Fatal("Startup.Attachments[0].Text is empty")
	}
	if cfg.Startup.Attachments[0].FilesChanged == 0 {
		t.Fatal("Startup.Attachments[0].FilesChanged is zero")
	}
	if cfg.Startup.Status.Severity != "success" {
		t.Fatalf("Startup.Status.Severity = %q, want %q", cfg.Startup.Status.Severity, "success")
	}
	if cfg.Startup.Status.Text == "" {
		t.Fatal("Startup.Status.Text is empty")
	}
}

func TestRunHeadlessReviewWritesPreparedGoal(t *testing.T) {
	repo := newGitRepo(t)
	writeFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"updated\")\n}\n")
	cfg := badger.DefaultConfig()
	cfg.Root = repo
	var stdout bytes.Buffer

	if err := runHeadlessReview(cfg, appConfig{reviewMode: reviewtask.ModeDefault}, &stdout, io.Discard); err != nil {
		t.Fatalf("runHeadlessReview() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Diff:") {
		t.Fatalf("runHeadlessReview() output missing diff:\n%s", stdout.String())
	}
	if !strings.HasSuffix(stdout.String(), "\n") || strings.HasSuffix(stdout.String(), "\n\n") {
		t.Fatalf("runHeadlessReview() output should have exactly one trailing newline: %q", stdout.String())
	}
}

func TestRunHeadlessReviewPropagatesErrors(t *testing.T) {
	cfg := badger.DefaultConfig()
	cfg.Root = t.TempDir()
	var stdout bytes.Buffer

	err := runHeadlessReview(cfg, appConfig{reviewMode: reviewtask.ModeDefault}, &stdout, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("runHeadlessReview() error = %v, want non-git error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("runHeadlessReview() stdout = %q, want empty", stdout.String())
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
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.Startup.Status.Severity != "warning" {
		t.Fatalf("Startup.Status.Severity = %q, want %q", cfg.Startup.Status.Severity, "warning")
	}
	if cfg.Startup.Goal == "" {
		t.Fatal("Startup.Goal is empty")
	}
	if strings.Contains(cfg.Startup.Goal, "Diff:") {
		t.Fatalf("Startup.Goal unexpectedly contains raw diff text:\n%s", cfg.Startup.Goal)
	}
	if len(cfg.Startup.Attachments) != 0 {
		t.Fatalf("Startup.Attachments length = %d, want 0", len(cfg.Startup.Attachments))
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

func TestPrintUsageIncludesPublicEntrypoints(t *testing.T) {
	out := captureStdout(t, func() {
		printUsage(appConfig{})
	})

	for _, want := range []string{
		"badger badge",
		"Launch the TUI with /badge preloaded",
		"badger review [--staged | --branch <ref> | --commit <sha>] [extra focus text]",
		"`badger review` preloads an editable review prompt from the current Git working tree.",
		"relevant Git-untracked paths",
		"badger api topology --root <project>",
		"badger api prompt --root <project> --focus <code|design> --input <goal-file>",
		"badger api extract --root <project> --input <selector-file> --goal-file <goal-file>",
		"The api commands are non-interactive and write directly usable prompt text to stdout.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("printUsage output missing %q:\n%s", want, out)
		}
	}
	for _, hidden := range []string{"api scan", "api goal", "api extraction", "api write-plan"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("printUsage output exposed certification-only operation %q:\n%s", hidden, out)
		}
	}
}

func TestApplyBadgeStartupInteractive(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	terminalInteractiveFunc = func() bool { return true }
	defer func() { terminalInteractiveFunc = originalTerminalInteractive }()

	cfg := badger.DefaultConfig()
	app := appConfig{}

	if err := applyBadgeStartup(&app, &cfg); err != nil {
		t.Fatalf("applyBadgeStartup() error = %v", err)
	}
	if !cfg.SkipOnboarding {
		t.Fatal("SkipOnboarding = false, want true")
	}
	if cfg.Startup.Goal != badgeStartupGoal {
		t.Fatalf("Startup.Goal = %q, want %q", cfg.Startup.Goal, badgeStartupGoal)
	}
	if cfg.Startup.Status.Text != "" {
		t.Fatalf("Startup.Status.Text = %q, want empty", cfg.Startup.Status.Text)
	}
}

func TestApplyBadgeStartupRejectsHeadless(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	terminalInteractiveFunc = func() bool { return true }
	defer func() { terminalInteractiveFunc = originalTerminalInteractive }()

	cfg := badger.DefaultConfig()
	app := appConfig{headless: true}

	if err := applyBadgeStartup(&app, &cfg); err == nil {
		t.Fatal("applyBadgeStartup() error = nil, want headless rejection")
	}
}

func TestApplyBadgeStartupRejectsNonInteractive(t *testing.T) {
	originalTerminalInteractive := terminalInteractiveFunc
	terminalInteractiveFunc = func() bool { return false }
	defer func() { terminalInteractiveFunc = originalTerminalInteractive }()

	cfg := badger.DefaultConfig()
	app := appConfig{}

	if err := applyBadgeStartup(&app, &cfg); err == nil {
		t.Fatal("applyBadgeStartup() error = nil, want interactive-terminal rejection")
	}
}

func TestLoadConfigRejectsRetiredGenericHeadlessFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--step", "topology"},
		{"--input", "commands.txt"},
		{"--truncate-topology"},
	} {
		cfg := loadConfig(args)
		if cfg.parseErr == nil || !strings.Contains(cfg.parseErr.Error(), "unknown flag") {
			t.Fatalf("loadConfig(%v) parseErr = %v, want unknown flag", args, cfg.parseErr)
		}
	}
}

func TestValidateHeadlessModeOnlyAllowsReview(t *testing.T) {
	if err := validateHeadlessMode(appConfig{headless: true}); err == nil {
		t.Fatal("validateHeadlessMode() error = nil for generic --headless")
	}
	if err := validateHeadlessMode(appConfig{headless: true, focus: protocol.FocusReview}); err != nil {
		t.Fatalf("validateHeadlessMode() review error = %v", err)
	}
}

func TestUsedDevOnlyFlagsTracksReviewHeadlessOnly(t *testing.T) {
	got := usedDevOnlyFlags([]string{"review", "--headless", "--step=topology"})
	if len(got) != 1 || got[0] != "--headless" {
		t.Fatalf("usedDevOnlyFlags() = %v, want [--headless]", got)
	}

	got = usedDevOnlyFlags([]string{"--", "--headless"})
	if len(got) != 0 {
		t.Fatalf("usedDevOnlyFlags() after option terminator = %v, want none", got)
	}
}

func TestParseAPIConfig(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    apiConfig
		wantErr string
	}{
		{
			name: "scan with root",
			args: []string{"scan", "--root", "/project"},
			want: apiConfig{operation: "scan", root: "/project"},
		},
		{
			name: "goal with input equals syntax",
			args: []string{"goal", "--root=/project", "--input=goal.txt"},
			want: apiConfig{operation: "goal", root: "/project", inputPath: "goal.txt"},
		},
		{
			name: "topology",
			args: []string{"topology", "--root", "/project"},
			want: apiConfig{operation: "topology", root: "/project"},
		},
		{
			name: "design prompt",
			args: []string{"prompt", "--focus=design", "--root", "/project", "--input", "goal.txt"},
			want: apiConfig{operation: "prompt", root: "/project", inputPath: "goal.txt", focus: protocol.FocusDesign},
		},
		{
			name: "extract",
			args: []string{"extract", "--root", "/project", "--input", "selectors.txt", "--goal-file", "goal.txt"},
			want: apiConfig{operation: "extract", root: "/project", inputPath: "selectors.txt", goalFilePath: "goal.txt"},
		},
		{
			name:    "missing operation",
			wantErr: "api operation is required",
		},
		{
			name:    "missing root",
			args:    []string{"scan"},
			wantErr: "api scan requires --root <project>",
		},
		{
			name:    "unknown operation",
			args:    []string{"unknown"},
			wantErr: "unknown api operation: unknown",
		},
		{
			name:    "missing input",
			args:    []string{"write-plan", "--root", "/project"},
			wantErr: "api write-plan requires --input <file>",
		},
		{
			name:    "scan rejects input",
			args:    []string{"scan", "--root", "/project", "--input", "ignored.txt"},
			wantErr: "api scan does not accept --input",
		},
		{
			name:    "prompt requires focus",
			args:    []string{"prompt", "--root", "/project", "--input", "goal.txt"},
			wantErr: "api prompt requires --focus <code|design>",
		},
		{
			name:    "prompt rejects unsupported focus",
			args:    []string{"prompt", "--root", "/project", "--input", "goal.txt", "--focus", "review"},
			wantErr: `api prompt supports --focus <code|design>; got "review"`,
		},
		{
			name:    "topology rejects focus",
			args:    []string{"topology", "--root", "/project", "--focus", "design"},
			wantErr: "api topology does not accept --focus",
		},
		{
			name:    "extract requires goal file",
			args:    []string{"extract", "--root", "/project", "--input", "selectors.txt"},
			wantErr: "api extract requires --goal-file <file>",
		},
		{
			name:    "extract rejects focus",
			args:    []string{"extract", "--root", "/project", "--input", "selectors.txt", "--goal-file", "goal.txt", "--focus", "code"},
			wantErr: "api extract does not accept --focus",
		},
		{
			name:    "prompt rejects goal file",
			args:    []string{"prompt", "--root", "/project", "--input", "goal.txt", "--focus", "code", "--goal-file", "other.txt"},
			wantErr: "api prompt does not accept --goal-file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAPIConfig(tt.args)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("parseAPIConfig() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAPIConfig() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("parseAPIConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRunAPIGoalUsesInputFile(t *testing.T) {
	root := t.TempDir()
	inputPath := filepath.Join(root, "goal.txt")
	if err := os.WriteFile(inputPath, []byte("inspect this project\n"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var output bytes.Buffer
	var diagnostics bytes.Buffer
	if err := runAPI([]string{"goal", "--root", root, "--input", inputPath}, &output, &diagnostics); err != nil {
		t.Fatalf("runAPI() error = %v", err)
	}
	if !strings.Contains(output.String(), "Dev goal: inspect this project") {
		t.Fatalf("runAPI() output = %q, want goal output", output.String())
	}
	if diagnostics.Len() != 0 {
		t.Fatalf("runAPI() diagnostics = %q, want empty", diagnostics.String())
	}
}
