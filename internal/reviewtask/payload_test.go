package reviewtask

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestBuildInitialReviewPayloadIncludesEligibleCurrentFilesAndStatuses(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// current supporting content\n")
	writeTrackedFile(t, repo, "added.go", "package added\n")
	writeTrackedFile(t, repo, ".env", "TOKEN=secret\n")
	runGitCmd(t, repo, "add", "added.go")

	result, err := BuildInitialReviewPayload(repo, Options{Mode: ModeDefault, ExtraFocus: "  Check concurrency.  "})
	if err != nil {
		t.Fatalf("BuildInitialReviewPayload() error = %v", err)
	}
	if result.Failure != PayloadFailureNone {
		t.Fatalf("Failure = %q", result.Failure)
	}
	statuses := payloadStatuses(result.Payload.Files)
	if statuses["app.go"] != ContextIncluded || statuses["added.go"] != ContextAddedPatch {
		t.Fatalf("statuses = %v", statuses)
	}
	if _, ok := statuses[".env"]; ok {
		t.Fatalf("repository-wide untracked file received tracked status: %v", statuses)
	}
	if !strings.Contains(result.Payload.Prompt, "Check concurrency.") || !strings.Contains(result.Payload.Prompt, "current supporting content") {
		t.Fatalf("prompt missing guidance or supporting content:\n%s", result.Payload.Prompt)
	}
	for _, want := range []string{
		"[TASK]",
		"[CONSTRAINT]",
		"If additional unchanged context is genuinely necessary",
		"FILE:<path>",
		"PREFIX:<path>#<literal prefix from the start of the target line>",
		"NEAR:<path>#<literal string from a nearby unique line or comment>",
	} {
		if !strings.Contains(result.Payload.Prompt, want) {
			t.Fatalf("prompt missing shared review instruction %q:\n%s", want, result.Payload.Prompt)
		}
	}
	if strings.Count(result.Payload.Prompt, "package added") != 1 {
		t.Fatalf("added file was duplicated:\n%s", result.Payload.Prompt)
	}
	if strings.Contains(result.Payload.Prompt, "Path: .env") {
		t.Fatalf("sensitive file was duplicated as supporting context:\n%s", result.Payload.Prompt)
	}
}

func TestBuildInteractiveContextIncludesTrackedDiffStats(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// one\n// two\n")

	ctx, err := BuildInteractiveContext(repo, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatal(err)
	}
	if len(ctx.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(ctx.Attachments))
	}
	attachment := ctx.Attachments[0]
	if attachment.FilesChanged != 1 || attachment.Additions != 2 || attachment.Deletions != 4 {
		t.Fatalf("stats = files:%d +%d/-%d, want files:1 +2/-4", attachment.FilesChanged, attachment.Additions, attachment.Deletions)
	}
}

func TestBuildInitialReviewPayloadRendersUntrackedPathsWithoutContents(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "notes/new.go", "package notes\nconst hidden = 42\n")

	result, err := BuildInitialReviewPayload(repo, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure != PayloadFailureNone || len(result.Payload.ChangeSet.Changes) != 0 {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(result.Payload.Prompt, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]") || !strings.Contains(result.Payload.Prompt, "not necessarily missing from the commit") || !strings.Contains(result.Payload.Prompt, "notes/new.go") {
		t.Fatalf("prompt missing untracked path section:\n%s", result.Payload.Prompt)
	}
	if strings.Contains(result.Payload.Prompt, "const hidden") || strings.Contains(result.Payload.Prompt, "Diff:") {
		t.Fatalf("untracked contents or synthetic diff leaked:\n%s", result.Payload.Prompt)
	}
}

func TestBuildInitialReviewPayloadSelectedUntrackedIsPathOnly(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "scratch/new.go", "package scratch\nconst selected = true\n")

	result, err := BuildInitialReviewPayload(repo, Options{Mode: ModeDefault, SelectedPaths: []string{"scratch/new.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure != PayloadFailureNone || !strings.Contains(result.Payload.Prompt, "scratch/new.go") {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(result.Payload.Prompt, "const selected = true") || len(result.Payload.ChangeSet.Changes) != 0 {
		t.Fatalf("selected untracked contents became authoritative changes: %+v", result.Payload)
	}
}

func TestPayloadUntrackedOmissionTextParticipatesInMandatoryBudget(t *testing.T) {
	set := ChangeSet{Mode: ModeDefault, UntrackedPaths: []string{"new.go"}, UntrackedOmitted: 7}
	baseline := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: 4096, maxFileBytes: 100}, readStableReviewFile)
	if baseline.Failure != PayloadFailureNone || !strings.Contains(baseline.Payload.Prompt, "7 additional relevant Git-untracked paths omitted") {
		t.Fatalf("baseline = %+v", baseline)
	}
	exact := len(baseline.Payload.Prompt)
	if got := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: exact, maxFileBytes: 100}, readStableReviewFile); got.Failure != PayloadFailureNone {
		t.Fatalf("exact boundary = %+v", got)
	}
	if got := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: exact - 1, maxFileBytes: 100}, readStableReviewFile); got.Failure != PayloadFailureMandatoryOverflow {
		t.Fatalf("overflow boundary = %+v", got)
	}
}

func TestBuildInitialReviewPayloadUsesWorkingTreeSupportingContentForStagedReview(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// staged image\n")
	runGitCmd(t, repo, "add", "app.go")
	writeTrackedFile(t, repo, "app.go", "package main\n// later worktree supporting image\n")

	result, err := BuildInitialReviewPayload(repo, Options{Mode: ModeStaged})
	if err != nil {
		t.Fatal(err)
	}
	prompt := result.Payload.Prompt
	if !strings.Contains(prompt, "staged image") || !strings.Contains(prompt, "later worktree supporting image") {
		t.Fatalf("prompt did not distinguish authoritative diff and current support:\n%s", prompt)
	}
}

func TestBuildInitialReviewPayloadUsesCurrentCheckoutForCommitReview(t *testing.T) {
	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// reviewed commit\n")
	runGitCmd(t, repo, "add", "app.go")
	runGitCmd(t, repo, "commit", "-m", "reviewed")
	commit := strings.TrimSpace(runGitCmd(t, repo, "rev-parse", "HEAD"))
	writeTrackedFile(t, repo, "app.go", "package main\n// current checkout support\n")

	result, err := BuildInitialReviewPayload(repo, Options{Mode: ModeCommit, Ref: commit})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Payload.Prompt, "reviewed commit") || !strings.Contains(result.Payload.Prompt, "current checkout support") {
		t.Fatalf("prompt = %s", result.Payload.Prompt)
	}
}

func TestPayloadFileLimitUsesUTF8BytesAndReadsAtMostOneExtraByte(t *testing.T) {
	set := oneModifiedChangeSet("app.go", "diff --git a/app.go b/app.go\n")
	readLimit := -1
	reader := func(_ string, limit int) ([]byte, stableFileOutcome) {
		readLimit = limit
		return []byte("ééé"), stableFileOversized
	}
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: 4096, maxFileBytes: 5}, reader)
	if readLimit != 5 || result.Payload.Files[0].Status != ContextOversized {
		t.Fatalf("readLimit=%d result=%+v", readLimit, result)
	}
}

func TestPayloadExactTotalBoundaryAndFirstBudgetFailure(t *testing.T) {
	set := ChangeSet{Mode: ModeDefault, Changes: []Change{
		{Path: "a.go", Kind: ChangeModified, Patch: "diff --git a/a.go b/a.go"},
		{Path: "b.go", Kind: ChangeModified, Patch: "diff --git a/b.go b/b.go"},
	}}
	reader := func(path string, _ int) ([]byte, stableFileOutcome) {
		return []byte(strings.Repeat(filepath.Base(path), 20)), stableFileOK
	}
	baseline := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: 10000, maxFileBytes: 100}, reader)
	exact := len(baseline.Payload.Prompt)
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: exact, maxFileBytes: 100}, reader)
	if result.Failure != PayloadFailureNone || len(result.Payload.Prompt) != exact {
		t.Fatalf("exact result = %+v", result)
	}

	oneOnly := cloneFileContexts(baseline.Payload.Files)
	oneOnly[1] = FileContext{Path: "b.go", Status: ContextBudget}
	limit := len(renderInitialReviewPrompt(set, "", oneOnly, 200))
	result = buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: limit, maxFileBytes: 200}, reader)
	if result.Payload.Files[0].Status != ContextIncluded || result.Payload.Files[1].Status != ContextBudget {
		t.Fatalf("budget statuses = %+v", result.Payload.Files)
	}
}

func TestPayloadPendingFilesDoNotCauseFalseMandatoryOverflow(t *testing.T) {
	set := ChangeSet{Mode: ModeDefault}
	for i := 0; i < 24; i++ {
		path := strings.Repeat("nested/", 4) + fmt.Sprintf("file-%02d.go", i)
		set.Changes = append(set.Changes, Change{Path: path, Kind: ChangeModified, Patch: "p"})
	}
	finalFiles := make([]FileContext, len(set.Changes))
	oldProvisionalFiles := make([]FileContext, len(set.Changes))
	for i, change := range set.Changes {
		finalFiles[i] = FileContext{Path: change.Path, Status: ContextUnavailable}
		oldProvisionalFiles[i] = FileContext{Path: change.Path, Status: ContextBudget}
	}
	finalSize := len(renderInitialReviewPrompt(set, "", finalFiles, 100))
	oldProvisionalSize := len(renderInitialReviewPrompt(set, "", oldProvisionalFiles, 100))
	if oldProvisionalSize <= finalSize {
		t.Fatalf("old provisional size = %d, final size = %d; fixture does not prove regression", oldProvisionalSize, finalSize)
	}
	reads := 0
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: finalSize, maxFileBytes: 100}, func(string, int) ([]byte, stableFileOutcome) {
		reads++
		return nil, stableFileUnavailable
	})
	if result.Failure != PayloadFailureNone || len(result.Payload.Prompt) != finalSize || reads != len(set.Changes) {
		t.Fatalf("result=%+v reads=%d, want successful %d-byte prompt after %d reads", result, reads, finalSize, len(set.Changes))
	}
}

func TestPayloadStopsReadingAfterFirstBudgetFailure(t *testing.T) {
	set := ChangeSet{Mode: ModeDefault, Changes: []Change{
		{Path: "a.go", Kind: ChangeModified, Patch: "p1"},
		{Path: "b.go", Kind: ChangeModified, Patch: "p2"},
		{Path: "c.go", Kind: ChangeModified, Patch: "p3"},
	}}
	content := strings.Repeat("x", 256)
	desired := []FileContext{
		{Path: "a.go", Status: ContextIncluded, Content: content},
		{Path: "b.go", Status: ContextBudget},
		{Path: "c.go", Status: ContextBudget},
	}
	limit := len(renderInitialReviewPrompt(set, "", desired, 512))
	reads := 0
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: limit, maxFileBytes: 512}, func(string, int) ([]byte, stableFileOutcome) {
		reads++
		return []byte(content), stableFileOK
	})
	if result.Failure != PayloadFailureNone || reads != 2 {
		t.Fatalf("result=%+v reads=%d, want success after first two reads", result, reads)
	}
	if got := payloadStatuses(result.Payload.Files); got["a.go"] != ContextIncluded || got["b.go"] != ContextBudget || got["c.go"] != ContextBudget {
		t.Fatalf("statuses = %v", got)
	}
}

func TestPayloadMandatoryOverflowDoesNotReadOptionalFiles(t *testing.T) {
	set := oneModifiedChangeSet("app.go", strings.Repeat("x", 200))
	reads := 0
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: 10, maxFileBytes: 100}, func(string, int) ([]byte, stableFileOutcome) {
		reads++
		return []byte("content"), stableFileOK
	})
	if result.Failure != PayloadFailureMandatoryOverflow || reads != 0 || result.Payload.Prompt != "" {
		t.Fatalf("result=%+v reads=%d", result, reads)
	}
}

func TestPayloadOptionalReadOutcomesAreExplicit(t *testing.T) {
	set := ChangeSet{Changes: []Change{
		{Path: "changed.go", Kind: ChangeModified, Patch: "p1"},
		{Path: "missing.go", Kind: ChangeModified, Patch: "p2"},
		{Path: "large.go", Kind: ChangeModified, Patch: "p3"},
	}}
	reader := func(path string, _ int) ([]byte, stableFileOutcome) {
		switch filepath.Base(path) {
		case "changed.go":
			return nil, stableFileChanged
		case "missing.go":
			return nil, stableFileUnavailable
		default:
			return nil, stableFileOversized
		}
	}
	result := buildInitialReviewPayload("/unused", set, "", reviewPayloadLimits{maxPayloadBytes: 4096, maxFileBytes: 100}, reader)
	statuses := payloadStatuses(result.Payload.Files)
	if statuses["changed.go"] != ContextChangedDuringRead || statuses["missing.go"] != ContextUnavailable || statuses["large.go"] != ContextOversized {
		t.Fatalf("statuses = %v", statuses)
	}
}

func TestRenderFileContextStatusUsesSharedVocabularyAndStableOrder(t *testing.T) {
	files := []FileContext{
		{Path: "included.go", Status: ContextIncluded},
		{Path: "deleted.go", Status: ContextDeleted},
		{Path: "new.go", Status: ContextAddedPatch},
		{Path: "image.png", Status: ContextBinary},
		{Path: ".env", Status: ContextSensitive},
		{Path: "large.go", Status: ContextOversized},
		{Path: "missing.go", Status: ContextUnavailable},
		{Path: "changed.go", Status: ContextChangedDuringRead},
		{Path: "budget.go", Status: ContextBudget},
	}
	want := strings.Join([]string{
		"[FILE CONTEXT STATUS]",
		`- deleted.go — diff only: deleted`,
		`- new.go — diff only: tracked newly added file already complete in patch`,
		`- image.png — diff only: binary file`,
		`- .env — diff only: sensitive file excluded from full-file context`,
		`- large.go — diff only: file exceeds 64 KiB full-file limit`,
		`- missing.go — diff only: full file unavailable`,
		`- changed.go — diff only: full file unavailable`,
		`- budget.go — diff only: total review-context budget reached`,
		"",
	}, "\n")
	if got := renderFileContextStatus(files, 64*1024); got != want {
		t.Fatalf("rendered status = %q, want %q", got, want)
	}
	if got := renderFileContextStatus([]FileContext{{Path: "ok.go", Status: ContextIncluded}}, 64*1024); got != "" {
		t.Fatalf("included-only status = %q, want empty", got)
	}
	if got := renderFileContextStatus([]FileContext{{Path: "unknown.go", Status: ContextStatus("internal-new-status")}}, 64*1024); strings.Contains(got, "internal-new-status") || !strings.Contains(got, "full file unavailable") {
		t.Fatalf("unknown internal status leaked into prompt: %q", got)
	}
}

func TestRenderFileContextStatusUsesEffectiveLimitAndEscapesPath(t *testing.T) {
	files := []FileContext{{Path: "dir/line\nname.go", Status: ContextOversized}}
	got := renderFileContextStatus(files, 1000)
	want := "[FILE CONTEXT STATUS]\n- dir/line\\nname.go — diff only: file exceeds 1000 bytes full-file limit\n"
	if got != want {
		t.Fatalf("rendered status = %q, want %q", got, want)
	}
}

func TestReadStableReviewFileRejectsNonRegularAndOversized(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("missing", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if _, got := readStableReviewFile(filepath.Join(dir, "link"), 10); got != stableFileUnavailable {
		t.Fatalf("symlink outcome = %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "large"), []byte("123456"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, got := readStableReviewFile(filepath.Join(dir, "large"), 5); got != stableFileOversized {
		t.Fatalf("large outcome = %v", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "exact"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if content, got := readStableReviewFile(filepath.Join(dir, "exact"), 5); got != stableFileOK || string(content) != "12345" {
		t.Fatalf("exact = %q, %v", content, got)
	}
	if err := os.WriteFile(filepath.Join(dir, "utf8"), []byte("ééé"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, got := readStableReviewFile(filepath.Join(dir, "utf8"), 5); got != stableFileOversized {
		t.Fatalf("UTF-8 byte boundary outcome = %v", got)
	}
}

func TestBuildInitialReviewPayloadNoChangesIsTyped(t *testing.T) {
	result, err := BuildInitialReviewPayload(newGitRepo(t), Options{Mode: ModeDefault})
	if err != nil {
		t.Fatal(err)
	}
	if result.Failure != PayloadFailureNoChanges || result.Payload.Prompt != "" {
		t.Fatalf("result = %+v", result)
	}
}

func TestBuildInteractiveContextKeepsFallbackAndRejectsMandatoryOverflow(t *testing.T) {
	clean := newGitRepo(t)
	ctx, err := BuildInteractiveContext(clean, Options{Mode: ModeDefault})
	if err != nil {
		t.Fatalf("BuildInteractiveContext(clean) error = %v", err)
	}
	if len(ctx.Attachments) != 0 || !strings.Contains(ctx.Goal, "Paste the diff below") || ctx.Status.Severity != "warning" {
		t.Fatalf("clean startup context = %#v, want editable warning fallback", ctx)
	}

	repo := newGitRepo(t)
	writeTrackedFile(t, repo, "app.go", "package main\n// changed\n")
	if _, err := BuildInteractiveContext(repo, Options{Mode: ModeDefault, MaxPayloadBytes: 1}); err == nil || !strings.Contains(err.Error(), "mandatory review context") {
		t.Fatalf("BuildInteractiveContext(overflow) error = %v, want mandatory overflow", err)
	}
}

func TestInteractivePayloadBudgetReservesReviewFramingAndTopology(t *testing.T) {
	got := InteractivePayloadBudget(512 * 1024)
	want := 512*1024 - protocol.SchemaAMinimumOverheadBytes(protocol.FocusReview) - 40*1024
	if got != want {
		t.Fatalf("InteractivePayloadBudget() = %d, want %d", got, want)
	}
	if got := InteractivePayloadBudget(-1); got != maxInitialReviewPayloadBytes {
		t.Fatalf("unbounded InteractivePayloadBudget() = %d, want %d", got, maxInitialReviewPayloadBytes)
	}
	minimum := protocol.SchemaAMinimumOverheadBytes(protocol.FocusReview)
	if got := InteractivePayloadBudget(minimum + 8*1024); got != 8*1024 {
		t.Fatalf("small InteractivePayloadBudget() = %d, want %d", got, 8*1024)
	}
}

func TestInteractivePayloadRetriesWithoutTopologyReserve(t *testing.T) {
	const maxPromptBytes = 64 * 1024
	preferred := InteractivePayloadBudget(maxPromptBytes)
	maximum := maximumInteractivePayloadBudget(maxPromptBytes)
	if preferred >= maximum {
		t.Fatalf("preferred budget = %d, maximum = %d, want fallback capacity", preferred, maximum)
	}
	set := oneModifiedChangeSet("app.go", strings.Repeat("+changed\n", 4*1024))
	opts := Options{Mode: ModeDefault, MaxPayloadBytes: preferred}

	initial := buildInitialReviewPayloadFromChangeSet(t.TempDir(), set, opts)
	if initial.Failure != PayloadFailureMandatoryOverflow {
		t.Fatalf("preferred result = %q, want mandatory overflow", initial.Failure)
	}
	got := buildInteractivePayloadFromChangeSet(t.TempDir(), set, opts, maxPromptBytes, false)
	if got.Failure != PayloadFailureNone {
		t.Fatalf("fallback result = %q, want success", got.Failure)
	}
	if strings.Contains(got.Payload.Prompt, "[TASK]") || strings.Contains(got.Payload.Prompt, "[CONSTRAINT]") {
		t.Fatalf("interactive payload redundantly included Schema-A framing:\n%s", got.Payload.Prompt)
	}
	if len(got.Payload.Prompt) > maximum {
		t.Fatalf("fallback payload = %d bytes, want <= %d", len(got.Payload.Prompt), maximum)
	}
}

func oneModifiedChangeSet(path, patch string) ChangeSet {
	return ChangeSet{Mode: ModeDefault, Changes: []Change{{Path: path, Kind: ChangeModified, Patch: patch}}}
}

func payloadStatuses(files []FileContext) map[string]ContextStatus {
	result := make(map[string]ContextStatus, len(files))
	for _, file := range files {
		result[file.Path] = file.Status
	}
	return result
}
