package reviewtask

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/scanner"
	"github.com/PVRLabs/aibadger/internal/startup"
)

// Mode selects how the review diff should be resolved.
type Mode int

const (
	// ModeDefault reviews the current worktree against HEAD.
	ModeDefault Mode = iota
	// ModeStaged reviews staged changes only.
	ModeStaged
	// ModeBranch reviews the current branch against a merge-base with Ref.
	ModeBranch
	// ModeCommit reviews a single commit identified by Ref.
	ModeCommit
)

// String returns the canonical label for the review mode.
func (m Mode) String() string {
	switch m {
	case ModeDefault:
		return "default"
	case ModeStaged:
		return "staged"
	case ModeBranch:
		return "branch"
	case ModeCommit:
		return "commit"
	default:
		return fmt.Sprintf("mode(%d)", int(m))
	}
}

// FailureClassification identifies a non-fatal resolution outcome.
type FailureClassification string

const (
	// FailureNone means a diff was resolved successfully.
	FailureNone FailureClassification = ""
	// FailureNoDiff means git was available but no reviewable changes were found.
	FailureNoDiff FailureClassification = "no_diff"
	// FailureNotGit means the root is not backed by a git repository.
	FailureNotGit FailureClassification = "not_git"
)

// Options configures review task generation.
type Options struct {
	Mode       Mode
	Ref        string
	ExtraFocus string
}

// Task is the editable review prompt payload prepared for the caller.
type Task struct {
	Mode                     Mode
	Ref                      string
	ExtraFocus               string
	Instruction              string
	Diff                     string
	FilesChanged             int
	Additions                int
	Deletions                int
	UntrackedFiles           []string
	UntrackedOmitted         int
	UntrackedDiscoveryFailed bool
	Prompt                   string
	FallbackPrompt           string
	FailureClassification    FailureClassification
}

const maxUntrackedReviewFiles = 25

// StartupPrompt returns the editable prompt text appropriate for interactive
// startup. When a diff was resolved, that is the review prompt. Otherwise it is
// the manual fallback prompt.
func (t Task) StartupPrompt() string {
	if t.FailureClassification == FailureNone {
		return t.Prompt
	}
	return t.FallbackPrompt
}

// StartupStatus returns the user-facing status text and severity for
// interactive startup.
func (t Task) StartupStatus() (text, severity string) {
	switch t.FailureClassification {
	case FailureNone:
		if t.UntrackedDiscoveryFailed {
			return "Loaded tracked review context, but Git-untracked files could not be listed. Edit it before submitting.", "warning"
		}
		if t.Mode == ModeDefault {
			return "Loaded review context from the current Git working tree. Edit it before submitting.", "success"
		}
		return "Loaded review context. Edit it before submitting.", "success"
	case FailureNoDiff:
		if t.UntrackedOmitted > 0 {
			return fmt.Sprintf("No reviewable changes were available; %s could not be inspected. The prompt is editable.", untrackedFileCount(t.UntrackedOmitted)), "warning"
		}
		return "No reviewable changes were detected. The prompt is editable.", "warning"
	case FailureNotGit:
		return "This directory is not a git repository. The prompt is editable.", "warning"
	default:
		return "Loaded review prompt. Edit it before submitting.", "neutral"
	}
}

// StartupContext returns the interactive startup payload for the task.
func (t Task) StartupContext() startup.Context {
	text, severity := t.StartupStatus()
	ctx := startup.Context{
		Goal: t.StartupPrompt(),
		Status: startup.Status{
			Text:     text,
			Severity: severity,
		},
	}
	if t.FailureClassification != FailureNone {
		return ctx
	}
	ctx.Goal = t.Instruction
	if t.Diff != "" {
		ctx.Attachments = append(ctx.Attachments, startup.Attachment{
			Type:         "git diff",
			Source:       "git diff",
			Text:         t.Diff,
			SizeBytes:    int64(len(t.Diff)),
			Lines:        countReviewTextLines(t.Diff),
			FilesChanged: t.FilesChanged,
			Additions:    t.Additions,
			Deletions:    t.Deletions,
		})
	}
	if untrackedSection := formatUntrackedSection(t.UntrackedFiles, t.UntrackedOmitted); untrackedSection != "" {
		ctx.Attachments = append(ctx.Attachments, startup.Attachment{
			Type:      "text",
			Source:    "Git-untracked files",
			Text:      untrackedSection,
			SizeBytes: int64(len(untrackedSection)),
			Lines:     countReviewTextLines(untrackedSection),
		})
	}
	return ctx
}

// HasReviewableChanges reports whether the task contains tracked diff content
// or relevant untracked files suitable for review.
func (t Task) HasReviewableChanges() bool {
	return strings.TrimSpace(t.Diff) != "" || len(t.UntrackedFiles) > 0
}

// HeadlessGoal returns the generated prompt that should seed headless review
// startup. Non-diff fallback cases are treated as failures so headless review
// exits with a clear error instead of silently continuing with a manual prompt.
func (t Task) HeadlessGoal() (string, error) {
	switch t.FailureClassification {
	case FailureNone:
		return t.Prompt, nil
	case FailureNoDiff:
		if t.UntrackedOmitted > 0 {
			return "", fmt.Errorf("review prompt could not be prepared: no reviewable changes were available; %s could not be inspected", untrackedFileCount(t.UntrackedOmitted))
		}
		return "", errors.New("review prompt could not be prepared: no reviewable changes were detected")
	case FailureNotGit:
		return "", errors.New("review prompt could not be prepared: this directory is not a git repository")
	default:
		return "", errors.New("review prompt could not be prepared")
	}
}

// Build prepares a review prompt from the requested diff mode.
func Build(root string, opts Options) (Task, error) {
	return build(root, opts, discoverUntrackedFiles)
}

type untrackedDiscoverer func(string) ([]string, int, error)

func build(root string, opts Options, discoverUntracked untrackedDiscoverer) (Task, error) {
	if err := validateOptions(opts); err != nil {
		return Task{}, err
	}

	task := Task{
		Mode:        opts.Mode,
		Ref:         strings.TrimSpace(opts.Ref),
		ExtraFocus:  strings.TrimSpace(opts.ExtraFocus),
		Instruction: buildReviewInstruction(strings.TrimSpace(opts.ExtraFocus)),
	}

	if !isGitRepository(root) {
		task.FailureClassification = FailureNotGit
		task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
		task.Prompt = buildFallbackPromptWithReason(task.ExtraFocus, "This directory is not a git repository.")
		return task, nil
	}

	diff, filesChanged, additions, deletions, err := resolveDiff(root, opts.Mode, task.Ref)
	if err != nil {
		return Task{}, err
	}
	diff = strings.TrimRight(diff, "\n")
	task.Diff = diff
	task.FilesChanged = filesChanged
	task.Additions = additions
	task.Deletions = deletions

	if opts.Mode == ModeDefault {
		task.UntrackedFiles, task.UntrackedOmitted, err = discoverUntracked(root)
		if err != nil {
			// Discovery is all-or-nothing: never surface partial results with an
			// incomplete-context warning.
			task.UntrackedFiles = nil
			task.UntrackedOmitted = 0
			if strings.TrimSpace(task.Diff) == "" {
				return Task{}, err
			}
			task.UntrackedDiscoveryFailed = true
		}
	}

	if !task.HasReviewableChanges() {
		task.FailureClassification = FailureNoDiff
		task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
		reason := "No reviewable changes were detected."
		if task.UntrackedOmitted > 0 {
			reason = fmt.Sprintf("No reviewable changes were available; %s could not be inspected.", untrackedFileCount(task.UntrackedOmitted))
		}
		task.Prompt = buildFallbackPromptWithReason(task.ExtraFocus, reason)
		return task, nil
	}

	task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
	task.Prompt = buildReviewPrompt(task.ExtraFocus, task.Diff, formatUntrackedSection(task.UntrackedFiles, task.UntrackedOmitted))
	if task.UntrackedDiscoveryFailed {
		task.Prompt += "\n\nWarning:\nGit-untracked files could not be listed; review context may be incomplete."
	}
	return task, nil
}

func validateOptions(opts Options) error {
	switch opts.Mode {
	case ModeDefault, ModeStaged:
		if strings.TrimSpace(opts.Ref) != "" {
			return fmt.Errorf("review mode %s does not accept a ref", opts.Mode)
		}
	case ModeBranch, ModeCommit:
		if strings.TrimSpace(opts.Ref) == "" {
			return fmt.Errorf("review mode %s requires a ref", opts.Mode)
		}
	default:
		return fmt.Errorf("unknown review mode %d", opts.Mode)
	}
	return nil
}

func resolveDiff(root string, mode Mode, ref string) (string, int, int, int, error) {
	switch mode {
	case ModeDefault:
		base, err := defaultDiffBase(root)
		if err != nil {
			return "", 0, 0, 0, err
		}
		return resolveDiffAndStats(root, []string{"diff", "--no-ext-diff", "--unified=3", base}, []string{"diff", "--no-ext-diff", "--numstat", base})
	case ModeStaged:
		return resolveDiffAndStats(root, []string{"diff", "--no-ext-diff", "--unified=3", "--cached"}, []string{"diff", "--no-ext-diff", "--numstat", "--cached"})
	case ModeBranch:
		base, err := runGit(root, "merge-base", "HEAD", ref)
		if err != nil {
			return "", 0, 0, 0, err
		}
		base = strings.TrimSpace(base)
		return resolveDiffAndStats(root, []string{"diff", "--no-ext-diff", "--unified=3", base, "HEAD"}, []string{"diff", "--no-ext-diff", "--numstat", base, "HEAD"})
	case ModeCommit:
		return resolveDiffAndStats(root, []string{"show", "--no-ext-diff", "--format=", "--unified=3", ref}, []string{"show", "--no-ext-diff", "--format=", "--numstat", ref})
	default:
		return "", 0, 0, 0, fmt.Errorf("unknown review mode %d", mode)
	}
}

func defaultDiffBase(root string) (string, error) {
	_, headErr := runGit(root, "rev-parse", "--verify", "HEAD")
	if headErr == nil {
		return "HEAD", nil
	}
	if !hasUnbornHead(root) {
		return "", headErr
	}

	// Derive the empty-tree ID through Git instead of assuming SHA-1. This also
	// supports repositories initialized with SHA-256 object IDs.
	emptyTree, err := runGitWithInput(root, "", "hash-object", "-t", "tree", "--stdin")
	if err != nil {
		return "", err
	}
	emptyTree = strings.TrimSpace(emptyTree)
	if emptyTree == "" {
		return "", errors.New("git returned an empty object ID for the empty tree")
	}
	return emptyTree, nil
}

func hasUnbornHead(root string) bool {
	headRef, err := runGit(root, "symbolic-ref", "-q", "HEAD")
	if err != nil {
		return false
	}
	headRef = strings.TrimSpace(headRef)
	if headRef == "" {
		return false
	}

	_, err = runGit(root, "show-ref", "--verify", "--quiet", headRef)
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == 1
}

func resolveDiffAndStats(root string, diffArgs, numstatArgs []string) (string, int, int, int, error) {
	diff, err := runGit(root, diffArgs...)
	if err != nil {
		return "", 0, 0, 0, err
	}
	numstat, err := runGit(root, numstatArgs...)
	if err != nil {
		return "", 0, 0, 0, err
	}
	filesChanged, additions, deletions := parseNumstat(numstat)
	return diff, filesChanged, additions, deletions, nil
}

type reviewUntrackedFile struct {
	path     string
	priority int
	modTime  time.Time
}

func discoverUntrackedFiles(root string) ([]string, int, error) {
	output, err := runGit(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, 0, err
	}

	paths, omitted := rankUntrackedFiles(root, strings.Split(output, "\x00"))
	return paths, omitted, nil
}

func rankUntrackedFiles(root string, entries []string) ([]string, int) {
	files := make([]reviewUntrackedFile, 0, len(entries))
	omitted := 0
	for _, rel := range entries {
		if rel == "" {
			continue
		}
		normalized := normalizeReviewPath(rel)
		absPath := filepath.Join(root, filepath.FromSlash(normalized))
		priority, ok := scanner.ReviewPathPriority(root, absPath)
		if !ok {
			continue
		}
		info, statErr := os.Lstat(absPath)
		if statErr != nil {
			omitted++
			continue
		}
		files = append(files, reviewUntrackedFile{
			path:     normalized,
			priority: priority,
			modTime:  info.ModTime(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		if files[i].priority != files[j].priority {
			return files[i].priority > files[j].priority
		}
		if !files[i].modTime.Equal(files[j].modTime) {
			return files[i].modTime.After(files[j].modTime)
		}
		return files[i].path < files[j].path
	})

	if len(files) > maxUntrackedReviewFiles {
		omitted += len(files) - maxUntrackedReviewFiles
		files = files[:maxUntrackedReviewFiles]
	}

	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.path)
	}
	return paths, omitted
}

func countReviewTextLines(text string) int {
	if text == "" {
		return 0
	}
	text = strings.TrimRight(text, "\r\n")
	if text == "" {
		return 0
	}
	lines := 1
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			lines++
		case '\r':
			lines++
			if i+1 < len(text) && text[i+1] == '\n' {
				i++
			}
		}
	}
	return lines
}

func normalizeReviewPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func formatUntrackedSection(paths []string, omitted int) string {
	if len(paths) == 0 && omitted == 0 {
		return ""
	}

	lines := make([]string, 0, len(paths)+3)
	lines = append(lines, "[REVIEW CONTEXT: GIT-UNTRACKED FILES]")
	for _, path := range paths {
		lines = append(lines, "- "+escapeReviewPath(path))
	}
	if omitted > 0 {
		lines = append(lines, "")
		if len(paths) == 0 && omitted == 1 {
			lines = append(lines, "1 Git-untracked file omitted.")
		} else if len(paths) == 0 {
			lines = append(lines, fmt.Sprintf("%d Git-untracked files omitted.", omitted))
		} else if omitted == 1 {
			lines = append(lines, "1 additional Git-untracked file omitted.")
		} else {
			lines = append(lines, fmt.Sprintf("%d additional Git-untracked files omitted.", omitted))
		}
	}
	return strings.Join(lines, "\n")
}

func untrackedFileCount(count int) string {
	if count == 1 {
		return "1 review-eligible Git-untracked file"
	}
	return fmt.Sprintf("%d review-eligible Git-untracked files", count)
}

func escapeReviewPath(path string) string {
	quoted := strconv.QuoteToGraphic(path)
	return quoted[1 : len(quoted)-1]
}

func parseNumstat(output string) (filesChanged, additions, deletions int) {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		filesChanged++
		if fields[0] != "-" {
			if n, err := parsePositiveInt(fields[0]); err == nil {
				additions += n
			}
		}
		if fields[1] != "-" {
			if n, err := parsePositiveInt(fields[1]); err == nil {
				deletions += n
			}
		}
	}
	return filesChanged, additions, deletions
}

func parsePositiveInt(s string) (int, error) {
	var n int
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid integer %q", s)
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

func isGitRepository(root string) bool {
	_, err := runGit(root, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

func runGit(root string, args ...string) (string, error) {
	return runGitWithInput(root, "", args...)
}

func runGitWithInput(root, input string, args ...string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}

	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Stdin = strings.NewReader(input)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func buildReviewPrompt(extraFocus, diff, untrackedSection string) string {
	lines := []string{buildReviewInstruction(extraFocus)}
	if diff != "" {
		lines = append(lines, "", "Diff:", diff)
	}
	if untrackedSection != "" {
		lines = append(lines, "", untrackedSection)
	}
	return strings.Join(lines, "\n")
}

func buildReviewInstruction(extraFocus string) string {
	lines := []string{defaultReviewGoal}
	if extraFocus != "" {
		lines = append(lines, "", "Additional focus:", extraFocus)
	}
	return strings.Join(lines, "\n")
}

func buildFallbackPrompt(extraFocus string) string {
	lines := []string{defaultReviewGoal}
	if extraFocus != "" {
		lines = append(lines, "", "Additional focus:", extraFocus)
	}
	lines = append(lines, "", "Paste the diff below or replace this text with the change you want reviewed.")
	return strings.Join(lines, "\n")
}

func buildFallbackPromptWithReason(extraFocus, reason string) string {
	lines := []string{defaultReviewGoal}
	if extraFocus != "" {
		lines = append(lines, "", "Additional focus:", extraFocus)
	}
	if reason != "" {
		lines = append(lines, "", reason)
	}
	lines = append(lines, "", "Paste the diff below or replace this text with the change you want reviewed.")
	return strings.Join(lines, "\n")
}

var defaultReviewGoal = "Review the following change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing."
