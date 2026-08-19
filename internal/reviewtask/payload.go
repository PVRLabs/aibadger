package reviewtask

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/scanner"
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
	contextPending           ContextStatus = "pending-full-file-read"
)

// PayloadFailure is a typed non-Git failure from initial payload generation.
type PayloadFailure string

const (
	PayloadFailureNone              PayloadFailure = ""
	PayloadFailureNoChanges         PayloadFailure = "no_changes"
	PayloadFailureMandatoryOverflow PayloadFailure = "mandatory_overflow"
	PayloadFailureTopologyScan      PayloadFailure = "topology_scan"
)

// FileContext records one changed path's explicit payload disposition.
type FileContext struct {
	Path           string
	Status         ContextStatus
	Content        string
	Untracked      bool
	suppressStatus bool
}

// InitialReviewPayload is the fully rendered, bounded initial review request.
type InitialReviewPayload struct {
	ChangeSet ChangeSet
	Guidance  string
	Files     []FileContext
	// MaxFileBytes is the effective per-file limit used when rendering status
	// reasons. It keeps interactive delivery byte-for-byte equivalent to the
	// initial review payload that produced it.
	MaxFileBytes int
	Prompt       string
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
	return buildReviewPayloadFromChangeSet(root, set, opts, true)
}

func buildReviewPayloadFromChangeSet(root string, set ChangeSet, opts Options, includeReviewInstructions bool) InitialReviewResult {
	limits := defaultReviewPayloadLimits
	if opts.MaxPayloadBytes > 0 {
		limits.maxPayloadBytes = opts.MaxPayloadBytes
	}
	if opts.MaxFileBytes > 0 {
		limits.maxFileBytes = opts.MaxFileBytes
	}
	return buildInitialReviewPayloadWithTopology(root, set, strings.TrimSpace(opts.ExtraFocus), limits, opts.IncludeTopology, opts.MaxFilesPerDirectory, includeReviewInstructions, readStableReviewFile)
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
	case PayloadFailureTopologyScan:
		return startup.Context{}, errors.New("review prompt could not be prepared: could not scan project topology")
	case PayloadFailureNone:
		return initialPayloadStartupContext(result.Payload), nil
	default:
		return startup.Context{}, fmt.Errorf("review prompt could not be prepared: unknown payload outcome %q", result.Failure)
	}
}

func buildInteractivePayloadFromChangeSet(root string, set ChangeSet, opts Options, maxPromptBytes int, explicitPayloadLimit bool) InitialReviewResult {
	result := buildReviewPayloadFromChangeSet(root, set, opts, false)
	if result.Failure != PayloadFailureMandatoryOverflow || explicitPayloadLimit {
		return result
	}
	maxTaskBytes := maximumInteractivePayloadBudget(maxPromptBytes)
	if maxTaskBytes <= opts.MaxPayloadBytes {
		return result
	}
	opts.MaxPayloadBytes = maxTaskBytes
	return buildReviewPayloadFromChangeSet(root, set, opts, false)
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
	contextPrompt := renderReviewContext(payload.ChangeSet, payload.Files, payload.MaxFileBytes)
	additions, deletions := reviewPatchStats(payload.ChangeSet.Changes)
	return startup.Context{
		Goal: buildReviewInstruction(payload.Guidance),
		Attachments: []startup.Attachment{{
			Type:           "review context",
			Source:         "review context",
			Text:           contextPrompt,
			SizeBytes:      int64(len(contextPrompt)),
			Lines:          countReviewTextLines(contextPrompt),
			FilesChanged:   len(payload.ChangeSet.Changes) + len(payload.ChangeSet.UntrackedPaths),
			Additions:      additions,
			Deletions:      deletions,
			SensitivePaths: sensitiveReviewPaths(payload.ChangeSet),
		}},
		Status: startup.Status{
			Text:     "Loaded Git changes and supporting review context. Add optional guidance before submitting.",
			Severity: "success",
		},
	}
}

func sensitiveReviewPaths(set ChangeSet) []string {
	paths := sensitiveTrackedPaths(set.Changes)
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		seen[path] = struct{}{}
	}
	for _, path := range set.UntrackedPaths {
		if !promptpolicy.IsSensitivePath(path) {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func sensitiveTrackedPaths(changes []Change) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, change := range changes {
		for _, path := range []string{change.Path, change.PreviousPath} {
			if path == "" || !promptpolicy.IsSensitivePath(path) {
				continue
			}
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
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
	return buildInitialReviewPayloadWithTopology(root, set, guidance, limits, false, 0, true, readFile)
}

func buildInitialReviewPayloadWithTopology(root string, set ChangeSet, guidance string, limits reviewPayloadLimits, includeTopology bool, maxFilesPerDirectory int, includeReviewInstructions bool, readFile stableFileReader) InitialReviewResult {
	if len(set.Changes) == 0 && len(set.UntrackedPaths) == 0 {
		return InitialReviewResult{Failure: PayloadFailureNoChanges}
	}
	files := make([]FileContext, 0, len(set.Changes)+len(set.UntrackedPaths))
	for _, change := range set.Changes {
		files = append(files, FileContext{Path: change.Path, Status: initialContextStatus(change)})
	}
	for _, path := range set.UntrackedPaths {
		status := contextPending
		if promptpolicy.IsSensitivePath(path) {
			status = ContextSensitive
		}
		files = append(files, FileContext{Path: path, Status: status, Untracked: true})
	}
	if !suppressFileStatusesToFit(set, guidance, files, limits, "", includeReviewInstructions) {
		return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
	}
	topology := ""
	if includeTopology {
		// Reserve the complete mandatory review request before rendering topology.
		// This guarantees topology can never consume or hide the authoritative diff.
		mandatory := renderReviewPayload(set, guidance, files, limits.maxFileBytes, "", includeReviewInstructions)
		// renderInitialReviewPrompt inserts one separator newline between the
		// topology block and the review instruction. Reserve it alongside the
		// mandatory review payload so an exactly-filled topology remains valid.
		topologyBudget := limits.maxPayloadBytes - len(mandatory) - 1
		if topologyBudget < 1 {
			return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
		}
		s := scanner.NewScanner(root)
		s.MaxFilesPerDirectory = maxFilesPerDirectory
		project, err := s.Scan()
		if err != nil {
			return InitialReviewResult{Failure: PayloadFailureTopologyScan}
		}
		formatter := protocol.NewFormatter()
		formatter.MaxTopologyPromptBytes = topologyBudget
		topology = formatter.GenerateTopology(project)
	}

	// Pending optional files render neither a limitation status nor a content
	// block. This is the smallest truthful provisional prompt, so it cannot
	// reject a review merely because a status may disappear after a successful
	// read.
	if len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)) > limits.maxPayloadBytes {
		return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
	}

	for i := range files {
		if files[i].Status != contextPending {
			continue
		}
		content, outcome := readFile(filepath.Join(root, filepath.FromSlash(files[i].Path)), limits.maxFileBytes)
		switch outcome {
		case stableFileOversized:
			files[i].Status = ContextOversized
		case stableFileUnavailable:
			files[i].Status = ContextUnavailable
		case stableFileChanged:
			files[i].Status = ContextChangedDuringRead
		case stableFileOK:
			if files[i].Untracked && !isReviewText(content) {
				files[i].Status = ContextBinary
			} else {
				files[i].Status = ContextIncluded
				files[i].Content = string(content)
			}
		}

		if len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)) <= limits.maxPayloadBytes {
			continue
		}
		if files[i].Status == ContextIncluded {
			files[i].Status = ContextBudget
			files[i].Content = ""
		}
		if len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)) > limits.maxPayloadBytes {
			files[i].suppressStatus = true
		}
		for j := i + 1; j < len(files); j++ {
			if files[j].Status == contextPending {
				files[j].Status = ContextBudget
				if len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)) > limits.maxPayloadBytes {
					files[j].suppressStatus = true
				}
			}
		}
		break
	}

	// Any pending entries remain only after budget exhaustion and were not read.
	for i := range files {
		if files[i].Status == contextPending {
			files[i].Status = ContextBudget
		}
	}
	prompt := renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)
	for len(prompt) > limits.maxPayloadBytes {
		removed := false
		for i := len(files) - 1; i >= 0; i-- {
			if files[i].Status != ContextIncluded {
				continue
			}
			files[i].Status = ContextBudget
			files[i].Content = ""
			if len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)) > limits.maxPayloadBytes {
				files[i].suppressStatus = true
			}
			removed = true
			break
		}
		if !removed {
			return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
		}
		prompt = renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions)
	}
	if len(prompt) > limits.maxPayloadBytes {
		return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow}
	}
	return InitialReviewResult{Payload: InitialReviewPayload{ChangeSet: set, Guidance: guidance, Files: files, MaxFileBytes: limits.maxFileBytes, Prompt: prompt}}
}

func suppressFileStatusesToFit(set ChangeSet, guidance string, files []FileContext, limits reviewPayloadLimits, topology string, includeReviewInstructions bool) bool {
	renderedBytes := len(renderReviewPayload(set, guidance, files, limits.maxFileBytes, topology, includeReviewInstructions))
	if renderedBytes <= limits.maxPayloadBytes {
		return true
	}
	statusBytes := len(renderFileContextStatus(files, limits.maxFileBytes))
	// When the complete status block disappears, renderReviewContext also
	// trims the leading separator newline before the mandatory diff/path body.
	const statusBodySeparatorBytes = 1
	if statusBytes == 0 || renderedBytes-statusBytes-statusBodySeparatorBytes > limits.maxPayloadBytes {
		return false
	}

	activeStatuses := 0
	for _, file := range files {
		if _, ok := renderFileContextStatusLine(file, limits.maxFileBytes); ok {
			activeStatuses++
		}
	}
	removedBytes := 0
	for i := len(files) - 1; i >= 0; i-- {
		line, ok := renderFileContextStatusLine(files[i], limits.maxFileBytes)
		if !ok {
			continue
		}
		files[i].suppressStatus = true
		removedBytes += len(line) + 1 // renderFileContextStatus appends a newline per line.
		activeStatuses--
		if activeStatuses == 0 {
			removedBytes += len("[FILE CONTEXT STATUS]\n") + statusBodySeparatorBytes
		}
		if renderedBytes-removedBytes <= limits.maxPayloadBytes {
			return true
		}
	}
	return false
}

func initialContextStatus(change Change) ContextStatus {
	switch {
	case change.Binary:
		return ContextBinary
	case isSensitiveChange(change):
		return ContextSensitive
	case change.Kind == ChangeAdded:
		return ContextAddedPatch
	case change.Kind == ChangeDeleted:
		return ContextDeleted
	case change.Kind == ChangeModified || change.Kind == ChangeRenamed:
		return contextPending
	default:
		return ContextUnavailable
	}
}

func isSensitiveChange(change Change) bool {
	return promptpolicy.IsSensitivePath(change.Path) || promptpolicy.IsSensitivePath(change.PreviousPath)
}

func renderInitialReviewPrompt(set ChangeSet, guidance string, files []FileContext, maxFileBytes int, topologyParts ...string) string {
	topology := ""
	if len(topologyParts) > 0 {
		topology = topologyParts[0]
	}
	return renderReviewPayload(set, guidance, files, maxFileBytes, topology, true)
}

func renderReviewPayload(set ChangeSet, guidance string, files []FileContext, maxFileBytes int, topology string, includeReviewInstructions bool) string {
	var out strings.Builder
	if topology != "" {
		out.WriteString(topology)
		out.WriteByte('\n')
	}
	if includeReviewInstructions {
		reviewInstructions := protocol.InstructionsForFocus(protocol.FocusReview)
		out.WriteString(fmt.Sprintf(reviewInstructions.SchemaAConstraint, buildReviewInstruction(guidance)))
		out.WriteByte('\n')
	}
	out.WriteString(renderReviewContext(set, files, maxFileBytes))
	return out.String()
}

func renderReviewContext(set ChangeSet, files []FileContext, maxFileBytes int) string {
	var out strings.Builder
	if status := renderFileContextStatus(files, maxFileBytes); status != "" {
		out.WriteString(status)
	}
	if len(set.Changes) > 0 {
		var diff strings.Builder
		for i, change := range set.Changes {
			if i > 0 {
				diff.WriteString("\n\n")
			}
			diff.WriteString(change.Patch)
		}
		out.WriteString("\nDiff:\n")
		out.WriteString(renderLiteralFence("diff", diff.String()))
		out.WriteByte('\n')
	}
	if len(set.UntrackedPaths) > 0 {
		out.WriteString("\n[REVIEW CONTEXT: GIT-UNTRACKED FILES]\n")
		out.WriteString("Note: These paths are untracked working-tree additions. Eligible complete contents are labeled separately below; omitted files remain path-only.\n\n")
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
		if file.Untracked {
			out.WriteString("\n[REVIEW CONTEXT: UNTRACKED WORKING-TREE ADDITION]\nPath: ")
		} else {
			out.WriteString("\n[REVIEW CONTEXT: CURRENT WORKING-TREE FILE]\nPath: ")
		}
		out.WriteString(escapeReviewPath(file.Path))
		out.WriteByte('\n')
		out.WriteString(renderLiteralFence("text", file.Content))
		out.WriteByte('\n')
	}
	return strings.TrimPrefix(out.String(), "\n")
}

func isReviewText(content []byte) bool {
	return !strings.ContainsRune(string(content), '\x00') && utf8.Valid(content)
}

func renderLiteralFence(language, content string) string {
	maxRun, run := 0, 0
	for _, r := range content {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		run = 0
	}
	fence := strings.Repeat("`", max(3, maxRun+1))
	ending := "\n"
	if strings.HasSuffix(content, "\n") {
		ending = ""
	}
	return fence + language + "\n" + content + ending + fence
}

// renderFileContextStatus is the AI-facing vocabulary shared with the VS Code
// direct-review payload. Internal ContextStatus values remain intentionally
// separate so control flow and typed results do not depend on prompt wording.
func renderFileContextStatus(files []FileContext, maxFileBytes int) string {
	if maxFileBytes <= 0 {
		maxFileBytes = maxInitialReviewFileBytes
	}
	lines := make([]string, 0, len(files)+1)
	for _, file := range files {
		line, ok := renderFileContextStatusLine(file, maxFileBytes)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return "[FILE CONTEXT STATUS]\n" + strings.Join(lines, "\n") + "\n"
}

func renderFileContextStatusLine(file FileContext, maxFileBytes int) (string, bool) {
	if file.suppressStatus {
		return "", false
	}
	reason, ok := fileContextStatusReason(file.Status, maxFileBytes)
	if !ok {
		return "", false
	}
	disposition := "diff only"
	if file.Untracked {
		disposition = "path only"
	}
	return fmt.Sprintf("- %s — %s: %s", escapeReviewPath(file.Path), disposition, reason), true
}

func fileContextStatusReason(status ContextStatus, maxFileBytes int) (string, bool) {
	kib := maxFileBytes / 1024
	limit := fmt.Sprintf("%d bytes", maxFileBytes)
	if maxFileBytes%1024 == 0 {
		limit = fmt.Sprintf("%d KiB", kib)
	}
	switch status {
	case ContextIncluded, contextPending:
		return "", false
	case ContextAddedPatch:
		return "tracked newly added file already complete in patch", true
	case ContextDeleted:
		return "deleted", true
	case ContextBinary:
		return "binary file", true
	case ContextSensitive:
		return "sensitive file excluded from full-file context", true
	case ContextOversized:
		return fmt.Sprintf("file exceeds %s full-file limit", limit), true
	case ContextUnavailable, ContextChangedDuringRead:
		return "full file unavailable", true
	case ContextBudget:
		return "total review-context budget reached", true
	default:
		return "full file unavailable", true
	}
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
