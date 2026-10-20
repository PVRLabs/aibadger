package reviewtask

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/scanner"
	"github.com/PVRLabs/aibadger/internal/testutil"
)

var (
	committedRepo     string
	unbornRepo        string
	committedOnce     sync.Once
	unbornOnce        sync.Once
	parallelRepoTests sync.Map
	gitTestProcesses  atomic.Int64
)

func TestMain(m *testing.M) {
	code := m.Run()
	if os.Getenv("BADGER_REPORT_GIT_TEST_PROCESSES") != "" {
		fmt.Fprintf(os.Stderr, "reviewtask test Git subprocesses: %d\n", gitTestProcesses.Load())
	}
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
	gitTestProcesses.Add(1)
	// Template repositories are copied concurrently by parallel tests. Disable
	// auto maintenance so transient .git/objects locks cannot disappear while
	// os.CopyFS walks the shared template.
	cmd := exec.Command("git", append([]string{"-c", "gc.auto=0", "-c", "maintenance.auto=false", "-C", dir}, args...)...)
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

func TestDefaultReviewGoalContract(t *testing.T) {
	instruction := buildReviewInstruction("")
	if instruction != defaultReviewGoal {
		t.Fatalf("buildReviewInstruction(\"\") = %q, want defaultReviewGoal %q", instruction, defaultReviewGoal)
	}
	for _, want := range []string{
		"concrete bugs, edge cases, regressions, maintainability problems, and unintended behavior changes",
		"Report concise findings, or clearly state that no issues were found.",
		"Include a brief, directional recommendation for addressing each finding when useful.",
		"Do not provide detailed patches or implementation code unless explicitly requested.",
	} {
		if !strings.Contains(instruction, want) {
			t.Fatalf("prepared review instruction missing %q:\n%s", want, instruction)
		}
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

func newGitRepo(t *testing.T) string {
	t.Helper()
	testutil.RequireIntegration(t)
	if _, loaded := parallelRepoTests.LoadOrStore(t.Name(), struct{}{}); !loaded {
		t.Parallel()
	}
	ensureCommittedTemplate()
	return copyRepositoryTemplate(t, committedRepo, "committed")
}

func newUnbornGitRepo(t *testing.T) string {
	t.Helper()
	testutil.RequireIntegration(t)
	if _, loaded := parallelRepoTests.LoadOrStore(t.Name(), struct{}{}); !loaded {
		t.Parallel()
	}
	ensureUnbornTemplate()
	return copyRepositoryTemplate(t, unbornRepo, "unborn")
}

func copyRepositoryTemplate(t *testing.T, template, label string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(template)); err != nil {
		t.Fatalf("copy %s repository template: %v", label, err)
	}
	return dst
}

func TestRepositoryTemplatesAreIsolated(t *testing.T) {
	first := newGitRepo(t)
	second := copyRepositoryTemplate(t, committedRepo, "committed")
	writeTrackedFile(t, first, "only-first.go", "package main\n")
	if _, err := os.Stat(filepath.Join(second, "only-first.go")); !os.IsNotExist(err) {
		t.Fatalf("second template copy sees first copy mutation: stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(committedRepo, "only-first.go")); !os.IsNotExist(err) {
		t.Fatalf("committed template sees copy mutation: stat error = %v", err)
	}
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
	testutil.RequireIntegration(t)
	gitTestProcesses.Add(1)

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

func TestRankUntrackedFilesProjectFilePriorityBeforeModificationRecency(t *testing.T) {
	repo := t.TempDir()
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "scratch.log", "aux\n")
	writeUntrackedFile(t, repo, "internal/api/new_client.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "scratch.log"), base.Add(2*time.Hour))
	setFileModTime(t, filepath.Join(repo, "internal/api/new_client.go"), base)

	paths, _ := rankUntrackedFiles(repo, []string{"scratch.log", "internal/api/new_client.go"})

	if paths[0] != "internal/api/new_client.go" {
		t.Fatalf("UntrackedFiles = %v, want project file first", paths)
	}
}

func TestRankUntrackedFilesModificationRecencyWithinSamePriority(t *testing.T) {
	repo := t.TempDir()
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "internal/api/older.go", "package api\n")
	writeUntrackedFile(t, repo, "internal/api/newer.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/older.go"), base)
	setFileModTime(t, filepath.Join(repo, "internal/api/newer.go"), base.Add(time.Hour))

	paths, _ := rankUntrackedFiles(repo, []string{"internal/api/older.go", "internal/api/newer.go"})

	if paths[0] != "internal/api/newer.go" {
		t.Fatalf("UntrackedFiles = %v, want newer file first", paths)
	}
}

func TestRankUntrackedFilesPathTieBreaksWhenPriorityAndTimesMatch(t *testing.T) {
	repo := t.TempDir()
	base := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	writeUntrackedFile(t, repo, "internal/api/b.go", "package api\n")
	writeUntrackedFile(t, repo, "internal/api/a.go", "package api\n")
	setFileModTime(t, filepath.Join(repo, "internal/api/b.go"), base)
	setFileModTime(t, filepath.Join(repo, "internal/api/a.go"), base)

	paths, _ := rankUntrackedFiles(repo, []string{"internal/api/b.go", "internal/api/a.go"})

	if paths[0] != "internal/api/a.go" || paths[1] != "internal/api/b.go" {
		t.Fatalf("UntrackedFiles = %v, want lexical tie-break", paths)
	}
}

func TestBuildChangeSetDoesNotTreatInvalidHeadAsUnborn(t *testing.T) {
	repo := newUnbornGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, ".git", "refs", "heads", "main"), []byte("not-an-object\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(invalid HEAD ref) error = %v", err)
	}

	_, err := BuildChangeSet(repo, Options{Mode: ModeDefault})

	if err == nil {
		t.Fatal("BuildChangeSet() error = nil, want invalid HEAD error")
	}
	if !strings.Contains(err.Error(), "git rev-parse --verify HEAD") {
		t.Fatalf("BuildChangeSet() error = %v, want original HEAD resolution error", err)
	}
}
