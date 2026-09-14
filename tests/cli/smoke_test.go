package cli_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	smokeEnv        = "BADGER_CLI_SMOKE"
	buildTimeout    = 2 * time.Minute
	commandTimeout  = 20 * time.Second
	goalMarker      = "SMOKE_GOAL_RENDER_GREETING"
	trackedMarker   = "SMOKE_TRACKED_GREETING"
	untrackedMarker = "SMOKE_UNTRACKED_NOTE"
)

type harness struct {
	t          *testing.T
	repoRoot   string
	fixture    string
	badgerPath string
	env        []string
}

func TestCLISmoke(t *testing.T) {
	if os.Getenv(smokeEnv) != "1" {
		t.Skip("set " + smokeEnv + "=1 to run CLI smoke tests")
	}

	h := newHarness(t)

	t.Run("version and help", h.testVersionAndHelp)
	t.Run("topology", h.testTopology)
	t.Run("prompt and extract", h.testPromptAndExtract)
	t.Run("default review context", h.testReviewContext)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	repoRoot := repositoryRoot(t)
	workDir := t.TempDir()
	runtimeHome := filepath.Join(workDir, "home")
	for _, dir := range []string{
		runtimeHome,
		filepath.Join(workDir, "config"),
		filepath.Join(workDir, "cache"),
		filepath.Join(workDir, "data"),
		filepath.Join(workDir, "tmp"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create isolated runtime directory %q: %v", dir, err)
		}
	}

	h := &harness{
		t:          t,
		repoRoot:   repoRoot,
		fixture:    filepath.Join(workDir, "smoke-fixture"),
		badgerPath: filepath.Join(workDir, executableName("badger")),
		env: append(os.Environ(),
			"HOME="+runtimeHome,
			"XDG_CONFIG_HOME="+filepath.Join(workDir, "config"),
			"XDG_CACHE_HOME="+filepath.Join(workDir, "cache"),
			"XDG_DATA_HOME="+filepath.Join(workDir, "data"),
			"TMPDIR="+filepath.Join(workDir, "tmp"),
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL="+os.DevNull,
		),
	}
	h.buildBadger()
	h.createFixture()
	return h
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve repository root: runtime caller unavailable")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolve repository root from %q: %v", sourceFile, err)
	}
	return root
}

func (h *harness) buildBadger() {
	h.t.Helper()
	run(h.t, buildTimeout, h.repoRoot, os.Environ(), "build badger executable", "go", "build", "-o", h.badgerPath, "./cmd/badger")
}

func (h *harness) createFixture() {
	h.t.Helper()
	if err := os.MkdirAll(h.fixture, 0o755); err != nil {
		h.t.Fatalf("create fixture repository: %v", err)
	}
	h.writeFixture("go.mod", "module example.com/smokefixture\n\ngo 1.27\n")
	h.writeFixture("main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(greeting())\n}\n\nfunc greeting() string {\n\treturn \"baseline greeting\"\n}\n")
	h.writeFixture("README.md", "# CLI Smoke Fixture\n\nA minimal Go Modules project.\n")

	h.git("initialize fixture", "init", "--quiet")
	h.git("configure fixture author name", "config", "user.name", "Badger CLI Smoke")
	h.git("configure fixture author email", "config", "user.email", "smoke@example.invalid")
	h.git("stage fixture baseline", "add", "go.mod", "main.go", "README.md")
	h.git("commit fixture baseline", "commit", "--quiet", "-m", "baseline")

	h.writeFixture("main.go", "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(greeting())\n}\n\nfunc greeting() string {\n\treturn \""+trackedMarker+"\"\n}\n")
	h.writeFixture("smoke-note.txt", untrackedMarker+"\n")
}

func (h *harness) writeFixture(name, content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.fixture, name), []byte(content), 0o644); err != nil {
		h.t.Fatalf("write fixture file %q: %v", name, err)
	}
}

func (h *harness) git(operation string, args ...string) string {
	h.t.Helper()
	stdout, _ := run(h.t, commandTimeout, h.fixture, h.env, operation, "git", args...)
	return stdout
}

func (h *harness) badger(t *testing.T, operation string, args ...string) string {
	t.Helper()
	stdout, stderr := run(t, commandTimeout, h.repoRoot, h.env, operation, h.badgerPath, args...)
	if stderr != "" {
		t.Fatalf("%s unexpectedly wrote to stderr:\n%s", operation, stderr)
	}
	return stdout
}

func run(t *testing.T, timeout time.Duration, dir string, env []string, operation, name string, args ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		timeoutDetail := ""
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			timeoutDetail = fmt.Sprintf("\nTIMEOUT: exceeded %s", timeout)
		}
		t.Fatalf("%s failed: %v%s\nCOMMAND: %s %s\nSTDOUT:\n%s\nSTDERR:\n%s", operation, err, timeoutDetail, name, strings.Join(args, " "), stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String()
}

func (h *harness) testVersionAndHelp(t *testing.T) {
	version := h.badger(t, "print version", "--version")
	if !regexp.MustCompile(`^badger v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?\n$`).MatchString(version) {
		t.Fatalf("version output does not match the stable format: %q", version)
	}

	help := h.badger(t, "print help", "--help")
	assertContains(t, "help output", help,
		"badger api topology --root <project>",
		"badger api prompt --root <project> --focus <code|design>",
		"badger api extract --root <project> [--focus <code|design>]",
		"badger api review-context --root <repository>",
	)
}

func (h *harness) testTopology(t *testing.T) {
	output := h.badger(t, "generate topology", "api", "topology", "--root", h.fixture)
	assertContains(t, "topology output", output,
		"[PROJECT TOPOLOGY]",
		"Languages: Go",
		"Stack: Go Modules",
		"[SOURCE TREE]",
		"go.mod",
		"main.go",
	)
}

func (h *harness) testPromptAndExtract(t *testing.T) {
	inputDir := t.TempDir()
	goalFile := filepath.Join(inputDir, "goal.txt")
	selectorFile := filepath.Join(inputDir, "selectors.txt")
	if err := os.WriteFile(goalFile, []byte(goalMarker+"\n"), 0o644); err != nil {
		t.Fatalf("write goal file: %v", err)
	}
	if err := os.WriteFile(selectorFile, []byte("FILE:main.go\n"), 0o644); err != nil {
		t.Fatalf("write selector file: %v", err)
	}

	prompt := h.badger(t, "generate code prompt", "api", "prompt", "--root", h.fixture, "--focus", "code", "--input", goalFile)
	assertContains(t, "prompt output", prompt,
		"[PROJECT TOPOLOGY]",
		"Stack: Go Modules",
		"main.go",
		"[TASK]",
		goalMarker,
		"[CONSTRAINT]",
		"FILE:",
	)

	extracted := h.badger(t, "extract selected source", "api", "extract", "--root", h.fixture, "--focus", "code", "--input", selectorFile, "--goal-file", goalFile)
	assertContains(t, "extract output", extracted,
		"[PROJECT TOPOLOGY]",
		"[TASK]",
		goalMarker,
		"[OUTPUT CONSTRAINT]",
		"[CONTEXT]",
		"main.go",
		trackedMarker,
	)
}

func (h *harness) testReviewContext(t *testing.T) {
	output := h.badger(t, "generate default review context", "api", "review-context", "--root", h.fixture)
	assertContains(t, "review-context output", output,
		"main.go",
		trackedMarker,
		"smoke-note.txt",
		untrackedMarker,
	)
}

func assertContains(t *testing.T, label, got string, markers ...string) {
	t.Helper()
	for _, marker := range markers {
		if !strings.Contains(got, marker) {
			t.Errorf("%s missing %q\nOUTPUT:\n%s", label, marker, got)
		}
	}
}

func executableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}
