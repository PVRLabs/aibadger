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

// Options configures review task generation.
type Options struct {
	Mode       Mode
	Ref        string
	ExtraFocus string
	// SelectedPaths narrows default working-tree review to literal,
	// repository-relative changed paths. Other modes reject selected paths.
	SelectedPaths []string
	// ExcludePatterns filters repository-relative changes in every mode.
	ExcludePatterns []string
	// MaxPayloadBytes and MaxFileBytes override the initial review-context
	// defaults when positive. Negative values are invalid.
	MaxPayloadBytes int
	MaxFileBytes    int
	// IncludeTopology asks the initial payload builder to prepend Badger-owned
	// project topology and source-tree context under the same payload budget.
	IncludeTopology bool
	// MaxFilesPerDirectory applies the normal scanner bound when topology is
	// requested. Zero retains the scanner's unlimited low-level default.
	MaxFilesPerDirectory int
	// MaxPromptBytes bounds the complete interactive Schema A prompt. It is
	// ignored by non-interactive payload generation.
	MaxPromptBytes int
}

func validateOptions(opts Options) error {
	if err := ValidateExcludePatterns(opts.ExcludePatterns); err != nil {
		return err
	}
	if opts.MaxPayloadBytes < 0 || opts.MaxFileBytes < 0 {
		return errors.New("review context byte limits cannot be negative")
	}
	if len(opts.SelectedPaths) > 0 && opts.Mode != ModeDefault {
		return fmt.Errorf("review mode %s does not accept selected paths", opts.Mode)
	}
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

const maxUntrackedReviewFiles = 25

type untrackedDiscoverer func(string) ([]string, int, error)

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

var defaultReviewGoal = "Review the following change for concrete bugs, edge cases, regressions, maintainability problems, and unintended behavior changes. Report concise findings, or clearly state that no issues were found. Include a brief, directional recommendation for addressing each finding when useful. Do not provide detailed patches or implementation code unless explicitly requested."
