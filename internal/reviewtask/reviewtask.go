package reviewtask

import (
	"fmt"
	"os/exec"
	"strings"
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
	Diff                  string
	Prompt                string
	FallbackPrompt        string
	FailureClassification FailureClassification
}

// Build prepares a review prompt from the requested diff mode.
func Build(root string, opts Options) (Task, error) {
	if err := validateOptions(opts); err != nil {
		return Task{}, err
	}

	task := Task{
		Mode:       opts.Mode,
		Ref:        strings.TrimSpace(opts.Ref),
		ExtraFocus: strings.TrimSpace(opts.ExtraFocus),
	}

	if !isGitRepository(root) {
		task.FailureClassification = FailureNotGit
		task.FallbackPrompt = buildFallbackPrompt(task.ExtraFocus)
		task.Prompt = buildFallbackPromptWithReason(task.ExtraFocus, "This directory is not a git repository.")
		return task, nil
	}

	diff, err := resolveDiff(root, opts.Mode, task.Ref)
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

func resolveDiff(root string, mode Mode, ref string) (string, error) {
	switch mode {
	case ModeDefault:
		return runGit(root, "diff", "--no-ext-diff", "--unified=3", "HEAD")
	case ModeStaged:
		return runGit(root, "diff", "--no-ext-diff", "--unified=3", "--cached")
	case ModeBranch:
		base, err := runGit(root, "merge-base", "HEAD", ref)
		if err != nil {
			return "", err
		}
		return runGit(root, "diff", "--no-ext-diff", "--unified=3", strings.TrimSpace(base), "HEAD")
	case ModeCommit:
		return runGit(root, "show", "--no-ext-diff", "--format=", "--unified=3", ref)
	default:
		return "", fmt.Errorf("unknown review mode %d", mode)
	}
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
	lines := []string{defaultReviewGoal}
	if extraFocus != "" {
		lines = append(lines, "", "Additional focus:", extraFocus)
	}
	lines = append(lines, "", "Diff:", diff)
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

var defaultReviewGoal = "Review my current change for concrete bugs, edge cases, maintainability issues, and unintended behavior changes. Focus on issues I should fix before committing."
