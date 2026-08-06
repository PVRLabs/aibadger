package reviewtask

import (
	"fmt"
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
	want := []string{"added file.txt:added", "app.go:modified", "delete.txt:deleted"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(set.UntrackedPaths, []string{"untracked[1].txt"}) || set.UntrackedOmitted != 0 {
		t.Fatalf("untracked = %v omitted=%d", set.UntrackedPaths, set.UntrackedOmitted)
	}
	for _, change := range set.Changes {
		if change.Patch == "" || !strings.Contains(change.Patch, "diff --git") {
			t.Fatalf("%s has incomplete patch: %q", change.Path, change.Patch)
		}
	}
}

func TestAssembleChangeSetUsesStructuredMetadataWithoutGit(t *testing.T) {
	root := t.TempDir()
	metadata := []changeMetadata{
		{path: "z.go", kind: ChangeModified},
		{path: "a.go", kind: ChangeAdded},
		{path: "gone.go", kind: ChangeDeleted, binary: true},
	}
	patches := map[string]string{"z.go": "z\n", "a.go": "a\n", "gone.go": "binary\n"}
	set, err := assembleChangeSet(root, Options{Mode: ModeDefault}, nil, metadata, []string{"scratch.txt"}, 2,
		func(_ string, _ []string, item changeMetadata) (string, bool, error) {
			return patches[item.path], item.binary, nil
		})
	if err != nil {
		t.Fatalf("assembleChangeSet() error = %v", err)
	}
	if got := []string{set.Changes[0].Path, set.Changes[1].Path, set.Changes[2].Path}; !reflect.DeepEqual(got, []string{"a.go", "gone.go", "z.go"}) {
		t.Fatalf("change ordering = %#v", got)
	}
	if !set.Changes[1].Binary || set.UntrackedOmitted != 2 || !reflect.DeepEqual(set.UntrackedPaths, []string{"scratch.txt"}) {
		t.Fatalf("assembled classifications = %#v", set)
	}
}

func TestAssembleChangeSetAppliesLiteralSelectionAndRejectsMissing(t *testing.T) {
	root := t.TempDir()
	metadata := []changeMetadata{{path: "one.go", kind: ChangeModified}, {path: "two.go", kind: ChangeModified}, {path: "new.txt", untracked: true}}
	buildPatch := func(_ string, _ []string, item changeMetadata) (string, bool, error) { return item.path, false, nil }
	set, err := assembleChangeSet(root, Options{Mode: ModeDefault, SelectedPaths: []string{"two.go", "two.go", "new.txt"}}, nil, metadata, nil, 0, buildPatch)
	if err != nil {
		t.Fatalf("selected assembleChangeSet() error = %v", err)
	}
	if len(set.Changes) != 1 || set.Changes[0].Path != "two.go" || !reflect.DeepEqual(set.UntrackedPaths, []string{"new.txt"}) {
		t.Fatalf("selected set = %#v", set)
	}
	if _, err := assembleChangeSet(root, Options{Mode: ModeDefault, SelectedPaths: []string{"missing.go"}}, nil, metadata, nil, 0, buildPatch); err == nil || !strings.Contains(err.Error(), "not a current change") {
		t.Fatalf("missing selection error = %v", err)
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
	if got, want := changeSummaries(set.Changes), []string{"app.go:modified"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("changes = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(set.UntrackedPaths, []string{"literal[1].txt"}) {
		t.Fatalf("selected untracked paths = %v", set.UntrackedPaths)
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
	if len(set.Changes) != 0 || !reflect.DeepEqual(set.UntrackedPaths, []string{"first.go"}) {
		t.Fatalf("unborn set = %+v", set)
	}
}

func TestBuildChangeSetRepositoryWideRanksCapsAndFiltersUntrackedPaths(t *testing.T) {
	repo := newGitRepo(t)
	for i := 0; i < 30; i++ {
		writeTrackedFile(t, repo, fmt.Sprintf("src/file_%02d.go", i), "package src\n")
	}
	writeTrackedFile(t, repo, "build/generated.go", "package build\n")
	writeTrackedFile(t, repo, ".env", "TOKEN=private\n")

	patchCalls := 0
	set, err := buildChangeSet(repo, Options{Mode: ModeDefault}, func(root string, args []string, item changeMetadata) (string, bool, error) {
		patchCalls++
		return buildChangePatch(root, args, item)
	}, discoverUntrackedFiles)
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 0 || len(set.UntrackedPaths) != maxUntrackedReviewFiles || set.UntrackedOmitted != 5 {
		t.Fatalf("set = %+v", set)
	}
	if patchCalls != 0 {
		t.Fatalf("per-entry patch calls = %d, want 0 for repository-wide untracked paths", patchCalls)
	}
	for _, path := range set.UntrackedPaths {
		if path == ".env" || strings.HasPrefix(path, "build/") {
			t.Fatalf("filtered path surfaced: %q", path)
		}
	}
}

func TestBuildChangeSetSelectedUntrackedBypassesFilteringWithoutSynthesizingPatch(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "build/literal[1].go", "package generated\n")

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"build/literal[1].go"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 0 || !reflect.DeepEqual(set.UntrackedPaths, []string{"build/literal[1].go"}) {
		t.Fatalf("set = %+v", set)
	}
}

func TestBuildChangeSetSelectedUntrackedBinaryIsPathOnly(t *testing.T) {
	repo := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "new.bin"), []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"new.bin"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 0 || !reflect.DeepEqual(set.UntrackedPaths, []string{"new.bin"}) {
		t.Fatalf("set = %+v", set)
	}
}

func TestBuildChangeSetSelectedEmptyUntrackedFileIsPathOnly(t *testing.T) {
	repo := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(repo, "empty.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	set, err := BuildChangeSet(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"empty.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Changes) != 0 || !reflect.DeepEqual(set.UntrackedPaths, []string{"empty.txt"}) {
		t.Fatalf("set = %+v", set)
	}
}

func TestChangePatchGitProcessCostIsBoundedByChangeKind(t *testing.T) {
	baseArgs := []string{"diff", "--no-ext-diff", "--binary", "HEAD"}
	for _, tc := range []struct {
		name  string
		item  changeMetadata
		calls int
	}{
		{name: "tracked", item: changeMetadata{path: "app.go", kind: ChangeModified}, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			_, _, err := buildChangePatchWithRunner("/repo", baseArgs, tc.item, func(_ string, args ...string) (string, error) {
				calls++
				return "diff --git", nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.calls {
				t.Fatalf("Git process calls = %d, want %d", calls, tc.calls)
			}
		})
	}
}

func TestUntrackedPatchConstructionIsRejectedWithoutGitProcess(t *testing.T) {
	calls := 0
	_, _, err := buildChangePatchWithRunner("/repo", nil, changeMetadata{path: "new.go", untracked: true}, func(string, ...string) (string, error) {
		calls++
		return "", nil
	})
	if err == nil || calls != 0 {
		t.Fatalf("error=%v calls=%d, want rejection before Git", err, calls)
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
