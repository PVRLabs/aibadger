package reviewtask

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildChangeSetDefaultClassifiesAndOrdersChanges(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// modified\n")
	writeTrackedFile(t, repo, "delete.txt", "delete me\n")
	runGitCmd(t, repo, "add", "delete.txt")
	runGitCmd(t, repo, "commit", "-m", "add deletion target")
	if err := os.Remove(filepath.Join(repo, "delete.txt")); err != nil {
		t.Fatal(err)
	}
	writeTrackedFile(t, repo, "added file.txt", "added\n")
	runGitCmd(t, repo, "add", "added file.txt")
	writeTrackedFile(t, repo, "untracked[1].txt", "untracked\n")

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("BuildChangeSet() error = %v", err)
	}
	got := changeSummaries(set.Changes)
	want := []string{"added file.txt:added", "app.go:modified", "delete.txt:deleted", "untracked[1].txt:untracked"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	for _, change := range set.Changes {
		if change.Patch == "" || !strings.Contains(change.Patch, "diff --git") {
			t.Fatalf("%s has incomplete patch: %q", change.Path, change.Patch)
		}
	}
}

func TestBuildChangeSetRenameAndBinary(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "old name.txt", "rename body\n")
	writeTrackedFile(t, repo, "image.dat", "text first\n")
	runGitCmd(t, repo, "add", "old name.txt", "image.dat")
	runGitCmd(t, repo, "commit", "-m", "fixtures")
	runGitCmd(t, repo, "mv", "old name.txt", "new name.txt")
	if err := os.WriteFile(filepath.Join(repo, "image.dat"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("BuildChangeSet() error = %v", err)
	}
	if len(set.Changes) != 2 {
		t.Fatalf("changes = %+v, want 2", set.Changes)
	}
	if got := set.Changes[1]; got.Kind != ChangeRenamed || got.Path != "new name.txt" || got.PreviousPath != "old name.txt" {
		t.Fatalf("rename = %+v", got)
	}
	if got := set.Changes[0]; got.Path != "image.dat" || !got.Binary {
		t.Fatalf("binary = %+v", got)
	}
}

func TestBuildChangeSetSelectedPathsAreLiteralDeduplicatedAndSorted(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// changed\n")
	writeTrackedFile(t, repo, "literal[1].txt", "literal\n")
	writeTrackedFile(t, repo, "other.txt", "other\n")

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"literal[1].txt", "app.go", "literal[1].txt"}})
	if err != nil {
		t.Fatalf("BuildChangeSet() error = %v", err)
	}
	if got, want := changeSummaries(set.Changes), []string{"app.go:modified", "literal[1].txt:untracked"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

func TestBuildChangeSetSelectedDeletedPath(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "gone.txt", "gone\n")
	runGitCmd(t, repo, "add", "gone.txt")
	runGitCmd(t, repo, "commit", "-m", "add gone")
	if err := os.Remove(filepath.Join(repo, "gone.txt")); err != nil {
		t.Fatal(err)
	}

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"gone.txt"}})
	if err != nil {
		t.Fatalf("BuildChangeSet() error = %v", err)
	}
	if len(set.Changes) != 1 || set.Changes[0].Kind != ChangeDeleted {
		t.Fatalf("changes = %+v", set.Changes)
	}
}

func TestBuildChangeSetRejectsInvalidAndStaleSelections(t *testing.T) {
	repo := newGitRepo(t)
	for _, tc := range []struct {
		name  string
		paths []string
	}{
		{"empty", []string{""}}, {"root", []string{"."}}, {"absolute", []string{filepath.Join(repo, "app.go")}},
		{"traversal", []string{"../app.go"}}, {"unchanged", []string{"app.go"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: tc.paths}); err == nil {
				t.Fatal("BuildChangeSet() error = nil")
			}
		})
	}
	if _, err := BuildChangeSet(repo, Options{Mode: ModeStaged, SelectedPaths: []string{"app.go"}}); err == nil {
		t.Fatal("staged selected paths error = nil")
	}
}

func TestBuildChangeSetSupportsModesAndUnbornHead(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// staged\n")
	runGitCmd(t, repo, "add", "app.go")
	staged, err := BuildChangeSet(repo, Options{Mode: ModeStaged})
	if err != nil || len(staged.Changes) != 1 {
		t.Fatalf("staged = %+v, err = %v", staged, err)
	}

	unborn := newUnbornGitRepo(t)
	writeTrackedFile(t, unborn, "first.go", "package first\n")
	set, err := BuildChangeSet(unborn, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("unborn BuildChangeSet() error = %v", err)
	}
	if len(set.Changes) != 1 || set.Changes[0].Kind != ChangeUntracked {
		t.Fatalf("unborn changes = %+v", set.Changes)
	}
}

func TestBuildChangeSetBranchAndCommitUseAuthoritativeRevisions(t *testing.T) {
	repo := newGitRepo(t)
	runGitCmd(t, repo, "checkout", "-b", "feature")
	writeTrackedFile(t, repo, "app.go", "package main\n// feature commit\n")
	runGitCmd(t, repo, "add", "app.go")
	runGitCmd(t, repo, "commit", "-m", "feature")
	commit := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	writeTrackedFile(t, repo, "app.go", "package main\n// dirty worktree\n")

	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"branch", Options{Mode: ModeBranch, Ref: "main"}},
		{"commit", Options{Mode: ModeCommit, Ref: commit}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set, err := BuildChangeSet(repo, tc.opts)
			if err != nil {
				t.Fatalf("BuildChangeSet() error = %v", err)
			}
			if len(set.Changes) != 1 || !strings.Contains(set.Changes[0].Patch, "feature commit") || strings.Contains(set.Changes[0].Patch, "dirty worktree") {
				t.Fatalf("changes = %+v", set.Changes)
			}
		})
	}
}

func TestLegacyBuildDoesNotSilentlyIgnoreSelectedPaths(t *testing.T) {
	repo := newGitRepo(t)
	if _, err := Build(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"app.go"}}); err == nil {
		t.Fatal("Build() error = nil")
	}
}

func TestBuildChangeSetRejectsNestedRepositoryDirectory(t *testing.T) {
	repo := newGitRepo(t)
	if _, err := BuildChangeSet(filepath.Join(repo, "internal"), Options{Mode: ModeDefault}); err == nil {
		t.Fatal("BuildChangeSet() error = nil")
	}
}

func changeSummaries(changes []Change) []string {
	result := make([]string, len(changes))
	for i, change := range changes {
		result[i] = change.Path + ":" + string(change.Kind)
	}
	return result
}
