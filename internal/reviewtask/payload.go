package reviewtask

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

const (
	maxInitialReviewPayloadBytes = 512 * 1024
	maxInitialReviewFileBytes    = 64 * 1024
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
	return buildInitialReviewPayload(root, set, strings.TrimSpace(opts.ExtraFocus), defaultReviewPayloadLimits, readStableReviewFile), nil
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
	out.WriteString("Review the authoritative Git changes below. Report actionable findings now, ordered by severity, with file and line references. If there are no findings, say so clearly. Request FILE:, PREFIX:, or NEAR: context only when unchanged referenced or validating context is genuinely necessary.\n")
	if guidance != "" {
		out.WriteString("\nReview guidance:\n")
		out.WriteString(guidance)
		out.WriteByte('\n')
	}
	if len(files) > 0 {
		out.WriteString("\nTracked file context status:\n")
		for _, file := range files {
			fmt.Fprintf(&out, "%s\t%s\n", file.Path, file.Status)
		}
	}
	if len(set.Changes) > 0 {
		out.WriteString("\nAuthoritative tracked Git diff:\n```diff\n")
		for i, change := range set.Changes {
			if i > 0 {
				out.WriteString("\n\n")
			}
			out.WriteString(change.Patch)
		}
		out.WriteString("\n```\n")
	}
	if len(set.UntrackedPaths) > 0 {
		out.WriteString("\nGit-untracked reference paths (contents not included):\n")
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
		out.WriteString("\nCurrent working-tree supporting file: ")
		out.WriteString(file.Path)
		out.WriteString("\n```text\n")
		out.WriteString(file.Content)
		if !strings.HasSuffix(file.Content, "\n") {
			out.WriteByte('\n')
		}
		out.WriteString("```\n")
	}
	return out.String()
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
