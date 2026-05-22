package reviewtask

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDefaultDiffPrompt(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"updated\")\n}\n")

	task, err := Build(repo, Options{Mode: ModeDefault, ExtraFocus: "Pay attention to logging."})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if task.Diff == "" {
		t.Fatal("Diff is empty")
	}
	for _, want := range []string{
		"Review my current change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing.",
		"Additional focus:",
		"Pay attention to logging.",
		"Diff:",
		"println(\"updated\")",
	} {
		if !strings.Contains(task.Prompt, want) {
			t.Fatalf("Prompt missing %q:\n%s", want, task.Prompt)
		}
	}
	if !strings.Contains(task.FallbackPrompt, "Paste the diff below or replace this text with the change you want reviewed.") {
		t.Fatalf("FallbackPrompt missing manual fallback guidance:\n%s", task.FallbackPrompt)
	}
	if strings.Contains(task.FallbackPrompt, "No git diff was detected.") {
		t.Fatalf("FallbackPrompt unexpectedly included a failure reason:\n%s", task.FallbackPrompt)
	}
}

func TestBuildStagedDiffPrompt(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"staged\")\n}\n")
	runGitCmd(t, repo, "add", "app.go")

	task, err := Build(repo, Options{Mode: ModeStaged})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if !strings.Contains(task.Prompt, "staged") {
		t.Fatalf("Prompt missing staged diff:\n%s", task.Prompt)
	}
}

func TestBuildBranchDiffPrompt(t *testing.T) {
	repo := newGitRepo(t)
	runGitCmd(t, repo, "checkout", "-b", "feature")
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"branch\")\n}\n")
	runGitCmd(t, repo, "commit", "-am", "feature change")

	task, err := Build(repo, Options{Mode: ModeBranch, Ref: "main"})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if !strings.Contains(task.Prompt, "branch") {
		t.Fatalf("Prompt missing branch diff:\n%s", task.Prompt)
	}
	if !strings.Contains(task.Prompt, "Diff:") {
		t.Fatalf("Prompt missing diff heading:\n%s", task.Prompt)
	}
}

func TestBuildCommitDiffPrompt(t *testing.T) {
	repo := newGitRepo(t)
	commit := commitTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"commit\")\n}\n", "commit change")

	task, err := Build(repo, Options{Mode: ModeCommit, Ref: commit})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if !strings.Contains(task.Prompt, "commit") {
		t.Fatalf("Prompt missing commit diff:\n%s", task.Prompt)
	}
}

func TestBuildExtraFocusText(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"focus\")\n}\n")

	task, err := Build(repo, Options{
		Mode:       ModeDefault,
		ExtraFocus: "Check error handling and nil guards.",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, want := range []string{
		"Additional focus:",
		"Check error handling and nil guards.",
	} {
		if !strings.Contains(task.Prompt, want) {
			t.Fatalf("Prompt missing %q:\n%s", want, task.Prompt)
		}
		if !strings.Contains(task.FallbackPrompt, want) {
			t.Fatalf("FallbackPrompt missing %q:\n%s", want, task.FallbackPrompt)
		}
	}
}

func TestBuildNoDiffFallback(t *testing.T) {
	repo := newGitRepo(t)

	task, err := Build(repo, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	if !strings.Contains(task.Prompt, "No git diff was detected.") {
		t.Fatalf("Prompt missing no-diff reason:\n%s", task.Prompt)
	}
	if !strings.Contains(task.FallbackPrompt, "Paste the diff below or replace this text with the change you want reviewed.") {
		t.Fatalf("FallbackPrompt missing fallback guidance:\n%s", task.FallbackPrompt)
	}
	if strings.Contains(task.FallbackPrompt, "No git diff was detected.") {
		t.Fatalf("FallbackPrompt unexpectedly contained no-diff reason:\n%s", task.FallbackPrompt)
	}
}

func TestBuildNonGitFallback(t *testing.T) {
	dir := t.TempDir()

	task, err := Build(dir, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if task.FailureClassification != FailureNotGit {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNotGit)
	}
	if !strings.Contains(task.Prompt, "This directory is not a git repository.") {
		t.Fatalf("Prompt missing not-git reason:\n%s", task.Prompt)
	}
	if !strings.Contains(task.FallbackPrompt, "Paste the diff below or replace this text with the change you want reviewed.") {
		t.Fatalf("FallbackPrompt missing fallback guidance:\n%s", task.FallbackPrompt)
	}
}

func TestBuildInvalidOptions(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{
			name: "default with ref",
			opts: Options{Mode: ModeDefault, Ref: "main"},
		},
		{
			name: "staged with ref",
			opts: Options{Mode: ModeStaged, Ref: "main"},
		},
		{
			name: "branch without ref",
			opts: Options{Mode: ModeBranch},
		},
		{
			name: "commit without ref",
			opts: Options{Mode: ModeCommit},
		},
		{
			name: "unknown mode",
			opts: Options{Mode: Mode(99)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Build(t.TempDir(), tc.opts)
			if err == nil {
				t.Fatal("Build() error = nil, want error")
			}
		})
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "checkout", "-b", "main")
	runGitCmd(t, dir, "config", "user.name", "Badger Test")
	runGitCmd(t, dir, "config", "user.email", "badger@example.com")
	commitTrackedFile(t, dir, "app.go", "package main\n\nfunc main() {\n\tprintln(\"base\")\n}\n", "initial commit")
	return dir
}

func writeTrackedFile(t *testing.T, dir, path, contents string) {
	t.Helper()

	fullPath := filepath.Join(dir, path)
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", fullPath, err)
	}
}

func commitTrackedFile(t *testing.T, dir, path, contents, message string) string {
	t.Helper()

	writeTrackedFile(t, dir, path, contents)
	runGitCmd(t, dir, "add", path)
	runGitCmd(t, dir, "commit", "-m", message)
	return strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
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
