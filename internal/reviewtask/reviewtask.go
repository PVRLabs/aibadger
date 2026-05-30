package reviewtask

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

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
	// FailureNoDiff means git was available but the selected diff was empty.
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
	Mode                  Mode
	Ref                   string
	ExtraFocus            string
	Instruction           string
	Diff                  string
	FilesChanged          int
	Additions             int
	Deletions             int
	Prompt                string
	FallbackPrompt        string
	FailureClassification FailureClassification
}

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
		return "Loaded review prompt from the current git diff. Edit it before submitting.", "success"
	case FailureNoDiff:
		return "No git diff was detected. The prompt is editable.", "warning"
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
	ctx.Attachments = []startup.Attachment{{
		Type:         "git diff",
		Source:       "git diff",
		Text:         t.Diff,
		SizeBytes:    int64(len(t.Diff)),
		Lines:        strings.Count(strings.TrimRight(t.Diff, "\n"), "\n") + 1,
		FilesChanged: t.FilesChanged,
		Additions:    t.Additions,
		Deletions:    t.Deletions,
	}}
	return ctx
}

// HeadlessGoal returns the generated prompt that should seed headless review
// startup. Non-diff fallback cases are treated as failures so headless review
// exits with a clear error instead of silently continuing with a manual prompt.
func (t Task) HeadlessGoal() (string, error) {
	switch t.FailureClassification {
	case FailureNone:
		return t.Prompt, nil
	case FailureNoDiff:
		return "", errors.New("review prompt could not be prepared: no git diff was detected")
	case FailureNotGit:
		return "", errors.New("review prompt could not be prepared: this directory is not a git repository")
	default:
		return "", errors.New("review prompt could not be prepared")
	}
}

// Build prepares a review prompt from the requested diff mode.
func Build(root string, opts Options) (Task, error) {
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
	if strings.TrimSpace(diff) == "" {
		task.FailureClassification = FailureNoDiff
		task.Diff = ""
		task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
		task.Prompt = buildFallbackPromptWithReason(task.ExtraFocus, "No git diff was detected.")
		return task, nil
	}

	task.Diff = diff
	task.FilesChanged = filesChanged
	task.Additions = additions
	task.Deletions = deletions
	task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
	task.Prompt = buildReviewPrompt(task.ExtraFocus, diff)
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
		return resolveDiffAndStats(root, []string{"diff", "--no-ext-diff", "--unified=3", "HEAD"}, []string{"diff", "--no-ext-diff", "--numstat", "HEAD"})
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
	root = strings.TrimSpace(root)
	if root == "" {
		root = "."
	}

	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func buildReviewPrompt(extraFocus, diff string) string {
	lines := []string{buildReviewInstruction(extraFocus)}
	lines = append(lines, "", "Diff:", diff)
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
