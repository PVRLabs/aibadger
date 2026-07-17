package reviewtask

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/scanner"
)

var (
	committedRepo string
	unbornRepo    string
	committedOnce sync.Once
	unbornOnce    sync.Once
)

func TestMain(m *testing.M) {
	code := m.Run()
	if committedRepo != "" {
		if err := os.RemoveAll(committedRepo); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove committed template %s: %v\n", committedRepo, err)
		}
	}
	if unbornRepo != "" {
		if err := os.RemoveAll(unbornRepo); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to remove unborn template %s: %v\n", unbornRepo, err)
		}
	}
	os.Exit(code)
}

func ensureCommittedTemplate() {
	committedOnce.Do(func() {
		committedRepo = createCommittedTemplate()
	})
}

func ensureUnbornTemplate() {
	unbornOnce.Do(func() {
		unbornRepo = createUnbornTemplate()
	})
}

func createCommittedTemplate() (dir string) {
	dir, err := os.MkdirTemp("", "badger-review-committed-*")
	if err != nil {
		panic(fmt.Sprintf("MkdirTemp: %v", err))
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	runTemplateGit(dir, "init", "--template=")
	runTemplateGit(dir, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "app.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"base\")\n}\n"), 0o644); err != nil {
		panic(fmt.Sprintf("WriteFile app.go: %v", err))
	}
	runTemplateGit(dir, "add", "app.go")
	runTemplateGit(dir, "commit", "-m", "initial commit")
	ok = true
	return dir
}

func createUnbornTemplate() (dir string) {
	dir, err := os.MkdirTemp("", "badger-review-unborn-*")
	if err != nil {
		panic(fmt.Sprintf("MkdirTemp: %v", err))
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(dir)
		}
	}()
	runTemplateGit(dir, "init", "--template=")
	runTemplateGit(dir, "checkout", "-b", "main")
	ok = true
	return dir
}

func runTemplateGit(dir string, args ...string) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Badger Test",
		"GIT_AUTHOR_EMAIL=badger@example.com",
		"GIT_COMMITTER_NAME=Badger Test",
		"GIT_COMMITTER_EMAIL=badger@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out)))
	}
}

func TestBuildDefaultTrackedChangesOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"updated\")\n}\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault, ExtraFocus: "Pay attention to logging."})

	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if !task.HasReviewableChanges() {
		t.Fatal("HasReviewableChanges() = false, want true")
	}
	if task.Diff == "" {
		t.Fatal("Diff is empty")
	}
	if len(task.UntrackedFiles) != 0 {
		t.Fatalf("UntrackedFiles = %v, want none", task.UntrackedFiles)
	}
	if task.UntrackedOmitted != 0 {
		t.Fatalf("UntrackedOmitted = %d, want 0", task.UntrackedOmitted)
	}
	if task.FilesChanged == 0 || task.Additions == 0 {
		t.Fatalf("tracked diff stats not populated: files=%d additions=%d deletions=%d", task.FilesChanged, task.Additions, task.Deletions)
	}
	if !strings.Contains(task.Prompt, "Diff:") {
		t.Fatalf("Prompt missing diff heading:\n%s", task.Prompt)
	}
	if strings.Contains(task.Prompt, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]") {
		t.Fatalf("Prompt unexpectedly included untracked section:\n%s", task.Prompt)
	}
	if !strings.Contains(task.Prompt, "println(\"updated\")") {
		t.Fatalf("Prompt missing tracked diff body:\n%s", task.Prompt)
	}
	ctx := task.StartupContext()
	if ctx.Goal != task.Instruction {
		t.Fatalf("StartupContext.Goal = %q, want instruction %q", ctx.Goal, task.Instruction)
	}
	if len(ctx.Attachments) != 1 {
		t.Fatalf("StartupContext.Attachments length = %d, want 1", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Type != "git diff" {
		t.Fatalf("attachment type = %q, want git diff", ctx.Attachments[0].Type)
	}
}

func TestBuildDefaultTrackedDiffSurvivesUntrackedDiscoveryFailure(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() { println(\"tracked\") }\n")
	discoveryErr := errors.New("untracked discovery failed")

	task, err := build(repo, Options{Mode: ModeDefault}, func(string) ([]string, int, error) {
		return []string{"partial.go"}, 3, discoveryErr
	})

	if err != nil {
		t.Fatalf("build() error = %v, want tracked review to continue", err)
	}
	if task.Diff == "" || task.FailureClassification != FailureNone {
		t.Fatalf("task = %+v, want successful tracked review", task)
	}
	if !task.UntrackedDiscoveryFailed {
		t.Fatal("UntrackedDiscoveryFailed = false, want true")
	}
	if len(task.UntrackedFiles) != 0 || task.UntrackedOmitted != 0 {
		t.Fatalf("partial discovery results survived error: files=%v omitted=%d", task.UntrackedFiles, task.UntrackedOmitted)
	}
	status, severity := task.StartupStatus()
	if severity != "warning" || !strings.Contains(status, "Git-untracked files could not be listed") {
		t.Fatalf("StartupStatus() = (%q, %q), want degraded-discovery warning", status, severity)
	}
	goal, err := task.HeadlessGoal()
	if err != nil {
		t.Fatalf("HeadlessGoal() error = %v", err)
	}
	if !strings.Contains(goal, "Warning:") || !strings.Contains(goal, "review context may be incomplete") {
		t.Fatalf("HeadlessGoal() missing degraded-discovery warning:\n%s", goal)
	}
}

func TestBuildDefaultWithoutTrackedDiffReturnsUntrackedDiscoveryFailure(t *testing.T) {
	repo := newGitRepo(t)
	discoveryErr := errors.New("untracked discovery failed")

	_, err := build(repo, Options{Mode: ModeDefault}, func(string) ([]string, int, error) {
		return nil, 0, discoveryErr
	})

	if !errors.Is(err, discoveryErr) {
		t.Fatalf("build() error = %v, want %v", err, discoveryErr)
	}
}

func TestBuildDefaultTrackedDiffReportsInspectionOnlyOmissions(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() { println(\"tracked\") }\n")

	task, err := build(repo, Options{Mode: ModeDefault}, func(string) ([]string, int, error) {
		return nil, 1, nil
	})

	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want success", task.FailureClassification)
	}
	if !strings.Contains(task.Prompt, "1 Git-untracked file omitted.") {
		t.Fatalf("Prompt missing omission-only section:\n%s", task.Prompt)
	}
	ctx := task.StartupContext()
	if len(ctx.Attachments) != 2 || !strings.Contains(ctx.Attachments[1].Text, "1 Git-untracked file omitted.") {
		t.Fatalf("StartupContext attachments = %+v, want diff and omission-only context", ctx.Attachments)
	}
}

func TestBuildDefaultInspectionOnlyOmissionsExplainNoReviewableChanges(t *testing.T) {
	repo := newGitRepo(t)

	task, err := build(repo, Options{Mode: ModeDefault}, func(string) ([]string, int, error) {
		return nil, 2, nil
	})

	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	for label, text := range map[string]string{
		"Prompt": task.Prompt,
		"Status": func() string { text, _ := task.StartupStatus(); return text }(),
	} {
		if !strings.Contains(text, "2 review-eligible Git-untracked files could not be inspected") {
			t.Fatalf("%s missing inspection failure detail:\n%s", label, text)
		}
	}
	if _, err := task.HeadlessGoal(); err == nil || !strings.Contains(err.Error(), "2 review-eligible Git-untracked files could not be inspected") {
		t.Fatalf("HeadlessGoal() error = %v, want inspection failure detail", err)
	}
}

func TestBuildDefaultTrackedAndUntrackedTogether(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"tracked\")\n}\n")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n\nconst value = 1\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/new_client.go"), time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if task.Diff == "" {
		t.Fatal("Diff is empty")
	}
	if len(task.UntrackedFiles) != 1 || task.UntrackedFiles[0] != "internal/api/new_client.go" {
		t.Fatalf("UntrackedFiles = %v, want [internal/api/new_client.go]", task.UntrackedFiles)
	}
	if task.UntrackedOmitted != 0 {
		t.Fatalf("UntrackedOmitted = %d, want 0", task.UntrackedOmitted)
	}
	if !strings.Contains(task.Prompt, "Diff:") || !strings.Contains(task.Prompt, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]") {
		t.Fatalf("Prompt missing tracked and untracked sections:\n%s", task.Prompt)
	}

	ctx := task.StartupContext()
	if len(ctx.Attachments) != 2 {
		t.Fatalf("StartupContext.Attachments length = %d, want 2", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Type != "git diff" {
		t.Fatalf("first attachment type = %q, want git diff", ctx.Attachments[0].Type)
	}
	if ctx.Attachments[1].Source != "Git-untracked files" {
		t.Fatalf("second attachment source = %q, want Git-untracked files", ctx.Attachments[1].Source)
	}
	if strings.Contains(ctx.Attachments[1].Text, "const value = 1") {
		t.Fatalf("untracked attachment leaked file contents:\n%s", ctx.Attachments[1].Text)
	}
}

func TestBuildDefaultUntrackedOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n\nconst value = 1\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/new_client.go"), time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC))

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if task.Diff != "" {
		t.Fatalf("Diff = %q, want empty", task.Diff)
	}
	if !task.HasReviewableChanges() {
		t.Fatal("HasReviewableChanges() = false, want true")
	}
	if !strings.Contains(task.Prompt, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]") {
		t.Fatalf("Prompt missing untracked heading:\n%s", task.Prompt)
	}
	if strings.Contains(task.Prompt, "Diff:") {
		t.Fatalf("Prompt unexpectedly included diff heading:\n%s", task.Prompt)
	}
	if !strings.Contains(task.Prompt, "- internal/api/new_client.go") {
		t.Fatalf("Prompt missing untracked path:\n%s", task.Prompt)
	}
	if _, err := task.HeadlessGoal(); err != nil {
		t.Fatalf("HeadlessGoal() error = %v, want nil", err)
	}

	ctx := task.StartupContext()
	if len(ctx.Attachments) != 1 {
		t.Fatalf("StartupContext.Attachments length = %d, want 1", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Type != "text" {
		t.Fatalf("attachment type = %q, want text", ctx.Attachments[0].Type)
	}
	if ctx.Attachments[0].Source != "Git-untracked files" {
		t.Fatalf("attachment source = %q, want Git-untracked files", ctx.Attachments[0].Source)
	}
	if strings.Contains(ctx.Attachments[0].Text, "const value = 1") {
		t.Fatalf("untracked attachment leaked file contents:\n%s", ctx.Attachments[0].Text)
	}
}

func TestBuildDefaultUnbornRepositoryWithStagedAndUnstagedChanges(t *testing.T) {
	repo := newUnbornGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() { println(\"staged\") }\n")
	runGitCmd(t, repo, "add", "app.go")
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() { println(\"working tree\") }\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if !strings.Contains(task.Diff, "println(\"working tree\")") {
		t.Fatalf("Diff missing current working-tree content:\n%s", task.Diff)
	}
	if task.FilesChanged != 1 || task.Additions == 0 {
		t.Fatalf("diff stats = files:%d additions:%d deletions:%d, want one added file", task.FilesChanged, task.Additions, task.Deletions)
	}
	if len(task.UntrackedFiles) != 0 {
		t.Fatalf("UntrackedFiles = %v, want none for staged path", task.UntrackedFiles)
	}
}

func TestBuildDefaultEmptyUnbornRepositoryHasNoReviewableChanges(t *testing.T) {
	repo := newUnbornGitRepo(t)

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	if task.HasReviewableChanges() {
		t.Fatal("HasReviewableChanges() = true, want false")
	}
	if !strings.Contains(task.Prompt, "No reviewable changes were detected.") {
		t.Fatalf("Prompt missing no-change explanation:\n%s", task.Prompt)
	}
}

func TestBuildDefaultDoesNotTreatInvalidHeadAsUnborn(t *testing.T) {
	repo := newUnbornGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".git", "refs", "heads", "main"), []byte("not-an-object\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(invalid HEAD ref) error = %v", err)
	}

	_, err := Build(repo, Options{Mode: ModeDefault})

	if err == nil {
		t.Fatal("Build() error = nil, want invalid HEAD error")
	}
	if !strings.Contains(err.Error(), "git rev-parse --verify HEAD") {
		t.Fatalf("Build() error = %v, want original HEAD resolution error", err)
	}
}

func TestBuildDefaultEscapesUntrackedPathControlCharacters(t *testing.T) {
	repo := newGitRepo(t)
	rel := filepath.Join("notes", "line\n- injected\rDiff:\tvalue.go")
	writeUntrackedFile(t, repo, rel, "package notes\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	normalized := filepath.ToSlash(rel)
	if len(task.UntrackedFiles) != 1 || task.UntrackedFiles[0] != normalized {
		t.Fatalf("UntrackedFiles = %q, want raw normalized path %q", task.UntrackedFiles, normalized)
	}
	if strings.Contains(task.Prompt, normalized) {
		t.Fatalf("Prompt included raw path control characters:\n%q", task.Prompt)
	}
	escaped := "notes/line\\n- injected\\rDiff:\\tvalue.go"
	if !strings.Contains(task.Prompt, escaped) {
		t.Fatalf("Prompt missing escaped path %q:\n%q", escaped, task.Prompt)
	}
	ctx := task.StartupContext()
	if len(ctx.Attachments) != 1 || ctx.Attachments[0].Lines != 2 {
		t.Fatalf("StartupContext attachments = %+v, want one two-line untracked attachment", ctx.Attachments)
	}
	if strings.Contains(ctx.Attachments[0].Text, normalized) {
		t.Fatalf("attachment included raw path control characters:\n%q", ctx.Attachments[0].Text)
	}
}

func TestBuildDefaultNoTrackedOrUntrackedChanges(t *testing.T) {
	repo := newGitRepo(t)

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	if task.HasReviewableChanges() {
		t.Fatal("HasReviewableChanges() = true, want false")
	}
	if task.Prompt == "" || !strings.Contains(task.Prompt, "No reviewable changes were detected.") {
		t.Fatalf("Prompt missing no-diff reason:\n%s", task.Prompt)
	}
	if _, err := task.HeadlessGoal(); err == nil {
		t.Fatal("HeadlessGoal() error = nil, want error")
	}
	if len(task.StartupContext().Attachments) != 0 {
		t.Fatalf("StartupContext.Attachments = %+v, want none", task.StartupContext().Attachments)
	}
}

func TestBuildNonGitFallback(t *testing.T) {
	task := buildTask(t, t.TempDir(), Options{Mode: ModeDefault, ExtraFocus: "Watch nil guards."})

	if task.FailureClassification != FailureNotGit {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNotGit)
	}
	if !strings.Contains(task.Prompt, "This directory is not a git repository.") {
		t.Fatalf("Prompt missing not-git reason:\n%s", task.Prompt)
	}
	if !strings.Contains(task.FallbackPrompt, "Paste the diff below or replace this text with the change you want reviewed.") {
		t.Fatalf("FallbackPrompt missing fallback guidance:\n%s", task.FallbackPrompt)
	}
	if !strings.Contains(task.Instruction, "Watch nil guards.") || !strings.Contains(task.Prompt, "Watch nil guards.") || !strings.Contains(task.FallbackPrompt, "Watch nil guards.") {
		t.Fatalf("extra focus text missing from generated prompts:\nInstruction:\n%s\nPrompt:\n%s\nFallback:\n%s", task.Instruction, task.Prompt, task.FallbackPrompt)
	}
}

func TestBuildInvalidOptions(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{name: "default rejects ref", opts: Options{Mode: ModeDefault, Ref: "main"}},
		{name: "staged rejects ref", opts: Options{Mode: ModeStaged, Ref: "main"}},
		{name: "branch requires ref", opts: Options{Mode: ModeBranch}},
		{name: "commit requires ref", opts: Options{Mode: ModeCommit}},
		{name: "unknown mode rejected", opts: Options{Mode: Mode(99)}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build(t.TempDir(), tc.opts); err == nil {
				t.Fatal("Build() error = nil, want error")
			}
		})
	}
}

func TestBuildExtraFocusTextInInstructionPromptAndFallback(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"focus\")\n}\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault, ExtraFocus: "Check error handling and nil guards."})

	for _, want := range []string{"Additional focus:", "Check error handling and nil guards."} {
		if !strings.Contains(task.Instruction, want) {
			t.Fatalf("Instruction missing %q:\n%s", want, task.Instruction)
		}
		if !strings.Contains(task.Prompt, want) {
			t.Fatalf("Prompt missing %q:\n%s", want, task.Prompt)
		}
		if !strings.Contains(task.FallbackPrompt, want) {
			t.Fatalf("FallbackPrompt missing %q:\n%s", want, task.FallbackPrompt)
		}
	}
}

func TestBuildNoReviewableChangesReasonOnlyInFailurePrompt(t *testing.T) {
	repo := newGitRepo(t)

	task := buildTask(t, repo, Options{Mode: ModeDefault, ExtraFocus: "Check error handling."})

	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	if !strings.Contains(task.Prompt, "No reviewable changes were detected.") {
		t.Fatalf("Prompt missing no-diff reason:\n%s", task.Prompt)
	}
	if strings.Contains(task.FallbackPrompt, "No reviewable changes were detected.") {
		t.Fatalf("FallbackPrompt unexpectedly included no-diff reason:\n%s", task.FallbackPrompt)
	}
	if !strings.Contains(task.Prompt, "Check error handling.") || !strings.Contains(task.FallbackPrompt, "Check error handling.") {
		t.Fatalf("extra focus text missing from failure prompts:\nPrompt:\n%s\nFallback:\n%s", task.Prompt, task.FallbackPrompt)
	}
}

func TestCountReviewTextLines(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "empty", text: "", want: 0},
		{name: "one line", text: "one line", want: 1},
		{name: "two lf lines", text: "one\ntwo", want: 2},
		{name: "two crlf lines", text: "one\r\ntwo", want: 2},
		{name: "trailing lf", text: "one\n", want: 1},
		{name: "trailing crlf", text: "one\r\n", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countReviewTextLines(tt.text); got != tt.want {
				t.Fatalf("countReviewTextLines(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestBuildIgnoredUntrackedFiles(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, ".gitignore", "ignored.tmp\n")
	runGitCmd(t, repo, "add", ".gitignore")
	runGitCmd(t, repo, "commit", "-m", "add gitignore")
	writeUntrackedFile(t, repo, "ignored.tmp", "ignored\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != 0 {
		t.Fatalf("UntrackedFiles = %v, want ignored file to be excluded", task.UntrackedFiles)
	}
	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
}

func TestBuildExcludedGeneratedOrTemporaryFiles(t *testing.T) {
	repo := newGitRepo(t)
	for _, rel := range []string{
		"go.sum",
		".DS_Store",
		"build/tmp.txt",
	} {
		writeUntrackedFile(t, repo, rel, "noise\n")
	}

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != 0 {
		t.Fatalf("UntrackedFiles = %v, want all generated/temporary files excluded", task.UntrackedFiles)
	}
	if task.FailureClassification != FailureNoDiff {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNoDiff)
	}
	status, _ := task.StartupStatus()
	if !strings.Contains(status, "No reviewable changes were detected") {
		t.Fatalf("StartupStatus() = %q, want reviewable-changes wording", status)
	}
}

func TestRankUntrackedFilesCountsInspectionFailuresAsOmitted(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "internal/present.go", "package internal\n")

	paths, omitted := rankUntrackedFiles(repo, []string{
		"internal/present.go",
		"internal/missing.go",
		"build/filtered.go",
	})

	if len(paths) != 1 || paths[0] != "internal/present.go" {
		t.Fatalf("paths = %v, want existing reviewable path", paths)
	}
	if omitted != 1 {
		t.Fatalf("omitted = %d, want missing reviewable path only", omitted)
	}
}

func TestBuildExactly25RelevantUntrackedFiles(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < maxUntrackedReviewFiles; i++ {
		rel := filepath.Join("internal", "api", fmt.Sprintf("file%02d.go", i))
		writeUntrackedFile(t, repo, rel, "package api\n")
		setFileModTime(t, filepath.Join(repo, rel), base.Add(time.Duration(i)*time.Minute))
	}

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != maxUntrackedReviewFiles {
		t.Fatalf("len(UntrackedFiles) = %d, want %d", len(task.UntrackedFiles), maxUntrackedReviewFiles)
	}
	if task.UntrackedOmitted != 0 {
		t.Fatalf("UntrackedOmitted = %d, want 0", task.UntrackedOmitted)
	}
}

func TestBuildMoreThan25RelevantUntrackedFiles(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < maxUntrackedReviewFiles+2; i++ {
		rel := filepath.Join("internal", "api", fmt.Sprintf("file%02d.go", i))
		writeUntrackedFile(t, repo, rel, "package api\n")
		setFileModTime(t, filepath.Join(repo, rel), base.Add(time.Duration(i)*time.Minute))
	}

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != maxUntrackedReviewFiles {
		t.Fatalf("len(UntrackedFiles) = %d, want %d", len(task.UntrackedFiles), maxUntrackedReviewFiles)
	}
	if task.UntrackedOmitted != 2 {
		t.Fatalf("UntrackedOmitted = %d, want 2", task.UntrackedOmitted)
	}
	if got := formatUntrackedSection(task.UntrackedFiles, task.UntrackedOmitted); !strings.Contains(got, "2 additional Git-untracked files omitted.") {
		t.Fatalf("formatted section missing plural omitted count:\n%s", got)
	}
}

func TestBuildOmittedCountExcludesFilteredUntrackedFiles(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < maxUntrackedReviewFiles+2; i++ {
		rel := filepath.Join("internal", "api", fmt.Sprintf("file%02d.go", i))
		writeUntrackedFile(t, repo, rel, "package api\n")
		setFileModTime(t, filepath.Join(repo, rel), base.Add(time.Duration(i)*time.Minute))
	}
	for _, rel := range []string{"build/generated.go", "go.sum", ".env"} {
		writeUntrackedFile(t, repo, rel, "filtered\n")
	}

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != maxUntrackedReviewFiles {
		t.Fatalf("len(UntrackedFiles) = %d, want %d", len(task.UntrackedFiles), maxUntrackedReviewFiles)
	}
	if task.UntrackedOmitted != 2 {
		t.Fatalf("UntrackedOmitted = %d, want only 2 review-eligible files omitted", task.UntrackedOmitted)
	}
}

func TestBuildIncludesDanglingUntrackedSymlinkPath(t *testing.T) {
	repo := newGitRepo(t)
	linkPath := filepath.Join(repo, "internal", "api", "current.go")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(linkPath), err)
	}
	if err := os.Symlink("missing.go", linkPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if len(task.UntrackedFiles) != 1 || task.UntrackedFiles[0] != "internal/api/current.go" {
		t.Fatalf("UntrackedFiles = %v, want dangling symlink path", task.UntrackedFiles)
	}
}

func TestUntrackedOmittedFormattingSingularAndPlural(t *testing.T) {
	if got := formatUntrackedSection([]string{"internal/api/new_client.go"}, 1); !strings.Contains(got, "1 additional Git-untracked file omitted.") {
		t.Fatalf("singular omitted formatting = %q", got)
	}
	if got := formatUntrackedSection([]string{"internal/api/new_client.go"}, 2); !strings.Contains(got, "2 additional Git-untracked files omitted.") {
		t.Fatalf("plural omitted formatting = %q", got)
	}
	if got := formatUntrackedSection(nil, 1); !strings.Contains(got, "1 Git-untracked file omitted.") {
		t.Fatalf("omission-only singular formatting = %q", got)
	}
	if got := formatUntrackedSection(nil, 2); !strings.Contains(got, "2 Git-untracked files omitted.") {
		t.Fatalf("omission-only plural formatting = %q", got)
	}
}

func TestFormatUntrackedSectionEscapesPathControlCharacters(t *testing.T) {
	path := "notes/line\n- injected\rDiff:\tvalue.go"

	got := formatUntrackedSection([]string{path}, 0)

	if strings.Contains(got, path) {
		t.Fatalf("formatUntrackedSection() included raw control characters:\n%q", got)
	}
	want := "[REVIEW CONTEXT: GIT-UNTRACKED FILES]\n- notes/line\\n- injected\\rDiff:\\tvalue.go"
	if got != want {
		t.Fatalf("formatUntrackedSection() = %q, want %q", got, want)
	}
	if lines := countReviewTextLines(got); lines != 2 {
		t.Fatalf("countReviewTextLines() = %d, want heading and one path line", lines)
	}
}

func TestBuildDeterministicOrdering(t *testing.T) {
	t.Run("priority before recency", func(t *testing.T) {
		repo := newGitRepo(t)
		base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		writeUntrackedFile(t, repo, "scratch.log", "aux\n")
		writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")
		setFileModTime(t, filepath.Join(repo, "scratch.log"), base.Add(2*time.Hour))
		setFileModTime(t, filepath.Join(repo, "internal/api/new_client.go"), base)

		task := buildTask(t, repo, Options{Mode: ModeDefault})

		if len(task.UntrackedFiles) != 2 {
			t.Fatalf("len(UntrackedFiles) = %d, want 2", len(task.UntrackedFiles))
		}
		if task.UntrackedFiles[0] != "internal/api/new_client.go" {
			t.Fatalf("UntrackedFiles[0] = %q, want project file first", task.UntrackedFiles[0])
		}
	})

	t.Run("recency within same priority", func(t *testing.T) {
		repo := newGitRepo(t)
		base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		writeUntrackedFile(t, repo, "internal/api/older.go", "package api\n")
		writeUntrackedFile(t, repo, "internal/api/newer.go", "package api\n")
		setFileModTime(t, filepath.Join(repo, "internal/api/older.go"), base)
		setFileModTime(t, filepath.Join(repo, "internal/api/newer.go"), base.Add(time.Hour))

		task := buildTask(t, repo, Options{Mode: ModeDefault})

		if task.UntrackedFiles[0] != "internal/api/newer.go" {
			t.Fatalf("UntrackedFiles[0] = %q, want newer file first", task.UntrackedFiles[0])
		}
	})

	t.Run("path tie-breaking", func(t *testing.T) {
		repo := newGitRepo(t)
		base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
		writeUntrackedFile(t, repo, "internal/api/b.go", "package api\n")
		writeUntrackedFile(t, repo, "internal/api/a.go", "package api\n")
		setFileModTime(t, filepath.Join(repo, "internal/api/b.go"), base)
		setFileModTime(t, filepath.Join(repo, "internal/api/a.go"), base)

		task := buildTask(t, repo, Options{Mode: ModeDefault})

		if task.UntrackedFiles[0] != "internal/api/a.go" || task.UntrackedFiles[1] != "internal/api/b.go" {
			t.Fatalf("UntrackedFiles = %v, want lexical tie-break", task.UntrackedFiles)
		}
	})
}

func TestBuildInteractiveContextWithDiffAttachmentOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"interactive\")\n}\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	ctx := task.StartupContext()

	if len(ctx.Attachments) != 1 {
		t.Fatalf("StartupContext.Attachments length = %d, want 1", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Type != "git diff" {
		t.Fatalf("attachment type = %q, want git diff", ctx.Attachments[0].Type)
	}
	if ctx.Attachments[0].FilesChanged != task.FilesChanged || ctx.Attachments[0].Additions != task.Additions || ctx.Attachments[0].Deletions != task.Deletions {
		t.Fatalf("diff stats not preserved: %+v vs %+v", ctx.Attachments[0], task)
	}
}

func TestBuildInteractiveContextWithDiffAndUntrackedAttachments(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"interactive\")\n}\n")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	ctx := task.StartupContext()

	if len(ctx.Attachments) != 2 {
		t.Fatalf("StartupContext.Attachments length = %d, want 2", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Type != "git diff" || ctx.Attachments[1].Source != "Git-untracked files" {
		t.Fatalf("attachment order incorrect: %+v", ctx.Attachments)
	}
}

func TestBuildInteractiveContextWithUntrackedAttachmentOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	ctx := task.StartupContext()

	if len(ctx.Attachments) != 1 {
		t.Fatalf("StartupContext.Attachments length = %d, want 1", len(ctx.Attachments))
	}
	if ctx.Attachments[0].Source != "Git-untracked files" {
		t.Fatalf("attachment source = %q, want Git-untracked files", ctx.Attachments[0].Source)
	}
	if strings.Contains(ctx.Attachments[0].Text, "package api") {
		t.Fatalf("untracked attachment leaked file contents:\n%s", ctx.Attachments[0].Text)
	}
}

func TestReviewPathPriorityPrefersProjectFiles(t *testing.T) {
	repo := newGitRepo(t)
	project := filepath.Join(repo, "internal/api/new_client.go")
	aux := filepath.Join(repo, "scratch.log")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")
	writeUntrackedFile(t, repo, "scratch.log", "aux\n")

	if got, ok := scannerPriorityForTest(t, repo, project); !ok || got != 1 {
		t.Fatalf("project priority = (%d,%v), want (1,true)", got, ok)
	}
	if got, ok := scannerPriorityForTest(t, repo, aux); !ok || got != 0 {
		t.Fatalf("aux priority = (%d,%v), want (0,true)", got, ok)
	}
}

func TestBuildUntrackedAttachmentContainsPathsButNotContents(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\nconst secret = \"hidden\"\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	ctx := task.StartupContext()

	if len(ctx.Attachments) != 1 {
		t.Fatalf("StartupContext.Attachments length = %d, want 1", len(ctx.Attachments))
	}
	if !strings.Contains(ctx.Attachments[0].Text, "internal/api/new_client.go") {
		t.Fatalf("attachment missing path:\n%s", ctx.Attachments[0].Text)
	}
	if strings.Contains(ctx.Attachments[0].Text, "hidden") {
		t.Fatalf("attachment leaked file contents:\n%s", ctx.Attachments[0].Text)
	}
}

func TestBuildSuccessStatusUsesNeutralWorkingTreeCopy(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	text, severity := task.StartupStatus()

	if severity != "success" {
		t.Fatalf("severity = %q, want success", severity)
	}
	if !strings.Contains(text, "current Git working tree") {
		t.Fatalf("status text = %q, want working-tree copy", text)
	}
}

func TestBuildDefaultProjectFilePriorityBeforeModificationRecency(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "scratch.log", "aux\n")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "scratch.log"), base.Add(2*time.Hour))
	setFileModTime(t, filepath.Join(repo, "internal/api/new_client.go"), base)

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.UntrackedFiles[0] != "internal/api/new_client.go" {
		t.Fatalf("UntrackedFiles = %v, want project file first", task.UntrackedFiles)
	}
}

func TestBuildDefaultModificationRecencyWithinSamePriority(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "internal/api/older.go", "package api\n")
	writeUntrackedFile(t, repo, "internal/api/newer.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/older.go"), base)
	setFileModTime(t, filepath.Join(repo, "internal/api/newer.go"), base.Add(time.Hour))

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.UntrackedFiles[0] != "internal/api/newer.go" {
		t.Fatalf("UntrackedFiles = %v, want newer file first", task.UntrackedFiles)
	}
}

func TestBuildDefaultPathTieBreaksWhenPriorityAndTimesMatch(t *testing.T) {
	repo := newGitRepo(t)
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "internal/api/b.go", "package api\n")
	writeUntrackedFile(t, repo, "internal/api/a.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/b.go"), base)
	setFileModTime(t, filepath.Join(repo, "internal/api/a.go"), base)

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.UntrackedFiles[0] != "internal/api/a.go" || task.UntrackedFiles[1] != "internal/api/b.go" {
		t.Fatalf("UntrackedFiles = %v, want lexical tie-break", task.UntrackedFiles)
	}
}

func TestBuildDefaultUntrackedAndTrackedDiffStatsStayTrackedOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n\nfunc main() {\n\tprintln(\"tracked\")\n}\n")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if task.FilesChanged == 0 {
		t.Fatal("FilesChanged is zero")
	}
	if strings.Contains(task.Prompt, "files changed") {
		t.Fatalf("Prompt unexpectedly included diff stats in untracked section:\n%s", task.Prompt)
	}
}

func TestBuildDefaultUntrackedSectionFormattingIsEmptyWhenNoFiles(t *testing.T) {
	if got := formatUntrackedSection(nil, 0); got != "" {
		t.Fatalf("formatUntrackedSection(nil, 0) = %q, want empty", got)
	}
}

func TestBuildDefaultUntrackedOnlyHasReviewableChanges(t *testing.T) {
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, "README.md", "# local change\n")

	task := buildTask(t, repo, Options{Mode: ModeDefault})

	if !task.HasReviewableChanges() {
		t.Fatal("HasReviewableChanges() = false, want true")
	}
}

func TestBuildUntrackedGoTestFileReviewEligibility(t *testing.T) {
	const (
		path     = "internal/model/topology_test.go"
		contents = "package model\n\nconst hiddenTopologyValue = 42\n"
	)
	repo := newGitRepo(t)
	writeUntrackedFile(t, repo, path, contents)

	task := buildTask(t, repo, Options{Mode: ModeDefault})
	if task.FailureClassification != FailureNone {
		t.Fatalf("FailureClassification = %q, want %q", task.FailureClassification, FailureNone)
	}
	if len(task.UntrackedFiles) != 1 || task.UntrackedFiles[0] != path {
		t.Fatalf("UntrackedFiles = %v, want [%q]", task.UntrackedFiles, path)
	}
	untrackedSection := formatUntrackedSection(task.UntrackedFiles, task.UntrackedOmitted)
	if !strings.Contains(untrackedSection, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]\n- "+path) {
		t.Fatalf("untracked section missing path:\n%s", untrackedSection)
	}
	if strings.Contains(untrackedSection, "hiddenTopologyValue") || strings.Contains(task.Prompt, "hiddenTopologyValue") {
		t.Fatalf("untracked review context leaked file contents:\n%s", task.Prompt)
	}

	initialCommit := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	for _, tt := range []struct {
		name string
		opts Options
	}{
		{name: "staged", opts: Options{Mode: ModeStaged}},
		{name: "branch", opts: Options{Mode: ModeBranch, Ref: "main"}},
		{name: "commit", opts: Options{Mode: ModeCommit, Ref: initialCommit}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			narrowTask := buildTask(t, repo, tt.opts)
			if len(narrowTask.UntrackedFiles) != 0 {
				t.Fatalf("UntrackedFiles = %v, want none", narrowTask.UntrackedFiles)
			}
			if strings.Contains(narrowTask.Prompt, path) {
				t.Fatalf("Prompt unexpectedly includes untracked path:\n%s", narrowTask.Prompt)
			}
		})
	}
}

func buildTask(t *testing.T, repo string, opts Options) Task {
	t.Helper()

	task, err := Build(repo, opts)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	return task
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	ensureCommittedTemplate()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(committedRepo)); err != nil {
		t.Fatalf("copy committed template: %v", err)
	}
	return dst
}

func newUnbornGitRepo(t *testing.T) string {
	t.Helper()
	ensureUnbornTemplate()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(unbornRepo)); err != nil {
		t.Fatalf("copy unborn template: %v", err)
	}
	return dst
}

func writeTrackedFile(t *testing.T, dir, path, contents string) {
	t.Helper()

	fullPath := filepath.Join(dir, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s) error = %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", fullPath, err)
	}
}

func writeUntrackedFile(t *testing.T, dir, path, contents string) {
	t.Helper()

	writeTrackedFile(t, dir, path, contents)
}

func commitTrackedFile(t *testing.T, dir, path, contents, message string) string {
	t.Helper()

	writeTrackedFile(t, dir, path, contents)
	runGitCmd(t, dir, "add", path)
	runGitCmd(t, dir, "commit", "-m", message)
	return strings.TrimSpace(runGitCmd(t, dir, "rev-parse", "HEAD"))
}

func setFileModTime(t *testing.T, path string, modTime time.Time) {
	t.Helper()

	if err := os.Chtimes(path, modTime, modTime); err != nil {
		t.Fatalf("Chtimes(%s) error = %v", path, err)
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

func scannerPriorityForTest(t *testing.T, repo, path string) (int, bool) {
	t.Helper()

	return scanner.ReviewPathPriority(repo, path)
}
