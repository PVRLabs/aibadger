package reviewtask

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/startup"
)

const (
	maxInitialReviewPayloadBytes    = 512 * 1024
	maxInitialReviewFileBytes       = 64 * 1024
	minimumInteractiveTopologyBytes = 40 * 1024
)

// ContextStatus explains how one changed file is represented in an initial
// review payload.
type ContextStatus string

const (
	ContextIncluded          ContextStatus = "complete-file-included"
	ContextAddedPatch        ContextStatus = "diff-only-complete-in-patch"
	ContextDeleted           ContextStatus = "diff-only-deleted"
	ContextBinary            ContextStatus = "diff-only-binary"
	ContextSensitive         ContextStatus = "diff-only-sensitive"
	ContextOversized         ContextStatus = "diff-only-oversized"
	ContextUnavailable       ContextStatus = "diff-only-unavailable"
	ContextChangedDuringRead ContextStatus = "diff-only-changed-during-read"
	ContextBudget            ContextStatus = "diff-only-budget"
)

// PayloadFailure is a typed non-Git failure from initial payload generation.
type PayloadFailure string

const (
	PayloadFailureNone              PayloadFailure = ""
	PayloadFailureNoChanges         PayloadFailure = "no_changes"
	PayloadFailureMandatoryOverflow PayloadFailure = "mandatory_overflow"
)

// FileContext records one changed path's explicit payload disposition.
type FileContext struct {
	Path    string
	Status  ContextStatus
	Content string
}

// InitialReviewPayload is the fully rendered, bounded initial review request.
type InitialReviewPayload struct {
	ChangeSet ChangeSet
	Guidance  string
	Files     []FileContext
	Prompt    string
}

// InitialReviewResult returns either a complete payload or a typed failure.
// Typed failures never contain a partial prompt.
type InitialReviewResult struct {
	Payload InitialReviewPayload
	Failure PayloadFailure
}

type reviewPayloadLimits struct {
	maxPayloadBytes int
	maxFileBytes    int
}

var defaultReviewPayloadLimits = reviewPayloadLimits{
	maxPayloadBytes: maxInitialReviewPayloadBytes,
	maxFileBytes:    maxInitialReviewFileBytes,
}

// BuildInitialReviewPayload inspects the repository once and renders a bounded
// initial request. It does not mutate the repository or invoke UI behavior.
func BuildInitialReviewPayload(root string, opts Options) (InitialReviewResult, error) {
	set, err := BuildChangeSet(root, opts)
	if err != nil {
		return InitialReviewResult{}, err
	}
	return buildInitialReviewPayloadFromChangeSet(root, set, opts), nil
}

func buildInitialReviewPayloadFromChangeSet(root string, set ChangeSet, opts Options) InitialReviewResult {
	limits := defaultReviewPayloadLimits
	if opts.MaxPayloadBytes > 0 {
		limits.maxPayloadBytes = opts.MaxPayloadBytes
	}
	if opts.MaxFileBytes > 0 {
		limits.maxFileBytes = opts.MaxFileBytes
	}
	return buildInitialReviewPayload(root, set, strings.TrimSpace(opts.ExtraFocus), limits, readStableReviewFile)
}

// BuildInteractiveContext prepares review context for editable TUI
// startup. The generated context stays byte-for-byte intact in one raw
// attachment; text entered in Goal is additional editable review guidance.
// Clean and non-Git roots preserve the historical editable fallback.
func BuildInteractiveContext(root string, opts Options) (startup.Context, error) {
	maxPromptBytes := opts.MaxPromptBytes
	if maxPromptBytes == 0 {
		maxPromptBytes = defaults.MaxTopologyPromptBytes
	}
	explicitPayloadLimit := opts.MaxPayloadBytes > 0
	if opts.MaxPayloadBytes <= 0 {
		opts.MaxPayloadBytes = InteractivePayloadBudget(maxPromptBytes)
	}
	set, err := BuildChangeSet(root, opts)
	if err != nil {
		legacy, legacyErr := Build(root, opts)
		if legacyErr == nil && legacy.FailureClassification == FailureNotGit {
			return legacy.StartupContext(), nil
		}
		return startup.Context{}, err
	}
	result := buildInteractivePayloadFromChangeSet(root, set, opts, maxPromptBytes, explicitPayloadLimit)
	switch result.Failure {
	case PayloadFailureNoChanges:
		legacy, err := Build(root, opts)
		if err != nil {
			return startup.Context{}, err
		}
		return legacy.StartupContext(), nil
	case PayloadFailureMandatoryOverflow:
		return startup.Context{}, errors.New("review prompt could not be prepared: mandatory review context exceeds the payload limit")
	case PayloadFailureNone:
		return initialPayloadStartupContext(result.Payload), nil
	default:
		return startup.Context{}, fmt.Errorf("review prompt could not be prepared: unknown payload outcome %q", result.Failure)
	}
}

func buildInteractivePayloadFromChangeSet(root string, set ChangeSet, opts Options, maxPromptBytes int, explicitPayloadLimit bool) InitialReviewResult {
	result := buildInitialReviewPayloadFromChangeSet(root, set, opts)
	if result.Failure != PayloadFailureMandatoryOverflow || explicitPayloadLimit {
		return result
	}
	maxTaskBytes := maximumInteractivePayloadBudget(maxPromptBytes)
	if maxTaskBytes <= opts.MaxPayloadBytes {
		return result
	}
	opts.MaxPayloadBytes = maxTaskBytes
	return buildInitialReviewPayloadFromChangeSet(root, set, opts)
}

// InteractivePayloadBudget reserves room in Prompt 1 for the real Review
// framing and a useful topology slice. Non-positive Prompt 1 limits retain the
// standalone review-context limit because the formatter is unbounded.
func InteractivePayloadBudget(maxPromptBytes int) int {
	if maxPromptBytes <= 0 {
		return maxInitialReviewPayloadBytes
	}
	budget := maximumInteractivePayloadBudget(maxPromptBytes)
	if budget > minimumInteractiveTopologyBytes {
		budget -= minimumInteractiveTopologyBytes
	}
	return budget
}

func maximumInteractivePayloadBudget(maxPromptBytes int) int {
	if maxPromptBytes <= 0 {
		return maxInitialReviewPayloadBytes
	}
	budget := maxPromptBytes - protocol.SchemaAMinimumOverheadBytes(protocol.FocusReview)
	if budget < 1 {
		return 1
	}
	if budget > maxInitialReviewPayloadBytes {
		return maxInitialReviewPayloadBytes
	}
	return budget
}

func initialPayloadStartupContext(payload InitialReviewPayload) startup.Context {
	contextPrompt := renderReviewContext(payload.ChangeSet, payload.Files)
	additions, deletions := reviewPatchStats(payload.ChangeSet.Changes)
	return startup.Context{
		Goal: buildReviewInstruction(payload.Guidance),
		Attachments: []startup.Attachment{{
			Type:         "review context",
			Source:       "review context",
			Text:         contextPrompt,
			SizeBytes:    int64(len(contextPrompt)),
			Lines:        countReviewTextLines(contextPrompt),
			FilesChanged: len(payload.ChangeSet.Changes) + len(payload.ChangeSet.UntrackedPaths),
			Additions:    additions,
			Deletions:    deletions,
		}},
		Status: startup.Status{
			Text:     "Loaded Git changes and supporting review context. Add optional guidance before submitting.",
			Severity: "success",
		},
	}
}

func reviewPatchStats(changes []Change) (additions, deletions int) {
	for _, change := range changes {
		for _, line := range strings.Split(change.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
				additions++
			case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
				deletions++
			}
		}
	}
	return additions, deletions
}

type stableFileOutcome int

const (
	stableFileOK stableFileOutcome = iota
	stableFileOversized
	stableFileUnavailable
	stableFileChanged
)

type stableFileReader func(string, int) ([]byte, stableFileOutcome)

func buildInitialReviewPayload(root string, set ChangeSet, guidance string, limits reviewPayloadLimits, readFile stableFileReader) InitialReviewResult {
	if len(set.Changes) == 0 && len(set.UntrackedPaths) == 0 {
		return InitialReviewResult{Failure: PayloadFailureNoChanges}
	}

	files := make([]FileContext, len(set.Changes))
	eligible := make([]bool, len(set.Changes))
	for i, change := range set.Changes {
		files[i] = FileContext{Path: change.Path, Status: initialContextStatus(change)}
		eligible[i] = files[i].Status == ContextBudget
	}

	// If even the context known without optional reads cannot fit, return before
	// touching candidate files. ContextBudget is the conservative pending status.
	if len(renderInitialReviewPrompt(set, guidance, files)) > limits.maxPayloadBytes {
		return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
	}

	budgetExhausted := false
	for i, change := range set.Changes {
		if !eligible[i] || budgetExhausted {
			continue
		}
		content, outcome := readFile(filepath.Join(root, filepath.FromSlash(change.Path)), limits.maxFileBytes)
		switch outcome {
		case stableFileOversized:
			files[i].Status = ContextOversized
		case stableFileUnavailable:
			files[i].Status = ContextUnavailable
		case stableFileChanged:
			files[i].Status = ContextChangedDuringRead
		case stableFileOK:
			candidate := cloneFileContexts(files)
			candidate[i].Status = ContextIncluded
			candidate[i].Content = string(content)
			if len(renderInitialReviewPrompt(set, guidance, candidate)) > limits.maxPayloadBytes {
				files[i].Status = ContextBudget
				budgetExhausted = true
				continue
			}
			files = candidate
		}
	}

	prompt := renderInitialReviewPrompt(set, guidance, files)
	if len(prompt) > limits.maxPayloadBytes {
		return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
	}
	return InitialReviewResult{Payload: InitialReviewPayload{ChangeSet: set, Guidance: guidance, Files: files, Prompt: prompt}}
}

func initialContextStatus(change Change) ContextStatus {
	switch {
	case change.Binary:
		return ContextBinary
	case promptpolicy.IsSensitivePath(change.Path):
		return ContextSensitive
	case change.Kind == ChangeAdded:
		return ContextAddedPatch
	case change.Kind == ChangeDeleted:
		return ContextDeleted
	case change.Kind == ChangeModified || change.Kind == ChangeRenamed:
		return ContextBudget // pending eligibility; also the default budget result
	default:
		return ContextUnavailable
	}
}

func renderInitialReviewPrompt(set ChangeSet, guidance string, files []FileContext) string {
	var out strings.Builder
	out.WriteString(buildReviewInstruction(guidance))
	out.WriteByte('\n')
	out.WriteString(renderReviewContext(set, files))
	return out.String()
}

func renderReviewContext(set ChangeSet, files []FileContext) string {
	var out strings.Builder
	if len(files) > 0 {
		out.WriteString("[REVIEW CONTEXT: TRACKED FILE STATUS]\n")
		for _, file := range files {
			fmt.Fprintf(&out, "%s\t%s\n", file.Path, file.Status)
		}
	}
	if len(set.Changes) > 0 {
		out.WriteString("\nDiff:\n```diff\n")
		for i, change := range set.Changes {
			if i > 0 {
				out.WriteString("\n\n")
			}
			out.WriteString(change.Patch)
		}
		out.WriteString("\n```\n")
	}
	if len(set.UntrackedPaths) > 0 {
		out.WriteString("\n[REVIEW CONTEXT: GIT-UNTRACKED FILES]\n")
		out.WriteString("Note: Untracked files are provided for reference and are not necessarily missing from the commit. Their contents are not included.\n\n")
		for _, path := range set.UntrackedPaths {
			out.WriteString("- ")
			out.WriteString(escapeReviewPath(path))
			out.WriteByte('\n')
		}
		if set.UntrackedOmitted > 0 {
			fmt.Fprintf(&out, "%d additional relevant Git-untracked paths omitted.\n", set.UntrackedOmitted)
		}
	}
	for _, file := range files {
		if file.Status != ContextIncluded {
			continue
		}
		out.WriteString("\n[REVIEW CONTEXT: CURRENT WORKING-TREE FILE]\nPath: ")
		out.WriteString(file.Path)
		out.WriteString("\n```text\n")
		out.WriteString(file.Content)
		if !strings.HasSuffix(file.Content, "\n") {
			out.WriteByte('\n')
		}
		out.WriteString("```\n")
	}
	return strings.TrimPrefix(out.String(), "\n")
}

func cloneFileContexts(files []FileContext) []FileContext {
	result := make([]FileContext, len(files))
	copy(result, files)
	return result
}

func readStableReviewFile(path string, maxBytes int) ([]byte, stableFileOutcome) {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() {
		return nil, stableFileUnavailable
	}
	if before.Size() > int64(maxBytes) {
		return nil, stableFileOversized
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, stableFileUnavailable
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(before, opened) || before.Size() != opened.Size() || before.ModTime() != opened.ModTime() {
		file.Close()
		return nil, stableFileChanged
	}
	content, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	afterOpen, statErr := file.Stat()
	closeErr := file.Close()
	afterPath, pathErr := os.Lstat(path)
	if len(content) > maxBytes {
		return nil, stableFileOversized
	}
	if readErr != nil || statErr != nil || closeErr != nil || pathErr != nil {
		return nil, stableFileUnavailable
	}
	if !os.SameFile(opened, afterOpen) || !os.SameFile(opened, afterPath) ||
		opened.Size() != afterOpen.Size() || opened.ModTime() != afterOpen.ModTime() ||
		opened.Size() != afterPath.Size() || opened.ModTime() != afterPath.ModTime() {
		return nil, stableFileChanged
	}
	return content, stableFileOK
}
