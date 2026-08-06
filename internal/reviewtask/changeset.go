package reviewtask

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// ChangeKind identifies the Git-level kind of one reviewed path.
type ChangeKind string

const (
	ChangeModified ChangeKind = "modified"
	ChangeAdded    ChangeKind = "added"
	ChangeDeleted  ChangeKind = "deleted"
	ChangeRenamed  ChangeKind = "renamed"
)

// Change is one structured, repository-relative review change. Patch is the
// complete Git-generated patch for this entry; PreviousPath is set for renames.
type Change struct {
	Path         string
	PreviousPath string
	Kind         ChangeKind
	Binary       bool
	Patch        string
}

// ChangeSet describes the authoritative diff scope without rendering a prompt
// or reading optional complete-file content.
type ChangeSet struct {
	Mode             Mode
	Ref              string
	Changes          []Change
	UntrackedPaths   []string
	UntrackedOmitted int
}

// BuildChangeSet inspects one Git repository and returns a deterministic,
// structured representation of the requested review diff.
func BuildChangeSet(root string, opts Options) (ChangeSet, error) {
	return buildChangeSet(root, opts, buildChangePatch, discoverUntrackedFiles)
}

type changePatchBuilder func(string, []string, changeMetadata) (string, bool, error)

func buildChangeSet(root string, opts Options, buildPatch changePatchBuilder, discoverUntracked untrackedDiscoverer) (ChangeSet, error) {
	if err := validateOptions(opts); err != nil {
		return ChangeSet{}, err
	}
	repoRoot, err := validateRepositoryRoot(root)
	if err != nil {
		return ChangeSet{}, err
	}
	selected, err := normalizeSelectedPaths(repoRoot, opts.SelectedPaths)
	if err != nil {
		return ChangeSet{}, err
	}

	baseArgs, err := changeDiffArgs(repoRoot, opts.Mode, strings.TrimSpace(opts.Ref))
	if err != nil {
		return ChangeSet{}, err
	}
	metadata, err := readTrackedMetadata(repoRoot, baseArgs)
	if err != nil {
		return ChangeSet{}, err
	}
	var untrackedPaths []string
	var untrackedOmitted int
	if opts.Mode == ModeDefault && len(selected) > 0 {
		untracked, err := readUntrackedMetadata(repoRoot)
		if err != nil {
			return ChangeSet{}, err
		}
		metadata = append(metadata, untracked...)
	} else if opts.Mode == ModeDefault {
		untrackedPaths, untrackedOmitted, err = discoverUntracked(repoRoot)
		if err != nil {
			return ChangeSet{}, err
		}
	}

	return assembleChangeSet(repoRoot, opts, baseArgs, metadata, untrackedPaths, untrackedOmitted, buildPatch)
}

// assembleChangeSet converts already-discovered structured Git metadata into
// the public ChangeSet. Keeping this boundary separate lets higher-level tests
// exercise selection, ordering, classification, and patch assembly without
// starting Git processes; BuildChangeSet remains the real-Git integration
// entry point.
func assembleChangeSet(repoRoot string, opts Options, baseArgs []string, metadata []changeMetadata, untrackedPaths []string, untrackedOmitted int, buildPatch changePatchBuilder) (ChangeSet, error) {
	if err := validateOptions(opts); err != nil {
		return ChangeSet{}, err
	}
	selected, err := normalizeSelectedPaths(repoRoot, opts.SelectedPaths)
	if err != nil {
		return ChangeSet{}, err
	}

	byPath := make(map[string]changeMetadata, len(metadata))
	for _, item := range metadata {
		byPath[item.path] = item
	}
	if len(selected) > 0 {
		filtered := make([]changeMetadata, 0, len(selected))
		for _, path := range selected {
			item, ok := byPath[path]
			if !ok {
				return ChangeSet{}, fmt.Errorf("selected path %q is not a current change", path)
			}
			if item.untracked {
				untrackedPaths = append(untrackedPaths, path)
				continue
			}
			filtered = append(filtered, item)
		}
		metadata = filtered
	}

	sort.Slice(metadata, func(i, j int) bool {
		if metadata[i].path != metadata[j].path {
			return metadata[i].path < metadata[j].path
		}
		return metadata[i].previousPath < metadata[j].previousPath
	})
	changes := make([]Change, 0, len(metadata))
	for _, item := range metadata {
		patch, binary, err := buildPatch(repoRoot, baseArgs, item)
		if err != nil {
			return ChangeSet{}, err
		}
		changes = append(changes, Change{Path: item.path, PreviousPath: item.previousPath, Kind: item.kind, Binary: binary, Patch: strings.TrimRight(patch, "\n")})
	}
	return ChangeSet{
		Mode:             opts.Mode,
		Ref:              strings.TrimSpace(opts.Ref),
		Changes:          changes,
		UntrackedPaths:   untrackedPaths,
		UntrackedOmitted: untrackedOmitted,
	}, nil
}

type changeMetadata struct {
	path         string
	previousPath string
	kind         ChangeKind
	untracked    bool
	binary       bool
}

func validateRepositoryRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	top, err := runGit(resolved, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New("root is not a Git repository")
	}
	topResolved, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil || topResolved != resolved {
		return "", errors.New("root must be the Git repository root")
	}
	return resolved, nil
}

func normalizeSelectedPaths(root string, paths []string) ([]string, error) {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, input := range paths {
		if input == "" || filepath.IsAbs(input) {
			return nil, fmt.Errorf("invalid selected path %q", input)
		}
		path := filepath.ToSlash(filepath.Clean(filepath.FromSlash(input)))
		if path == "." || path == ".." || strings.HasPrefix(path, "../") {
			return nil, fmt.Errorf("invalid selected path %q", input)
		}
		abs := filepath.Join(root, filepath.FromSlash(path))
		rel, err := filepath.Rel(root, abs)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("selected path %q escapes the repository", input)
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func changeDiffArgs(root string, mode Mode, ref string) ([]string, error) {
	switch mode {
	case ModeDefault:
		base, err := defaultDiffBase(root)
		return []string{"diff", "--no-ext-diff", "--binary", "-M", base}, err
	case ModeStaged:
		return []string{"diff", "--no-ext-diff", "--binary", "-M", "--cached"}, nil
	case ModeBranch:
		base, err := runGit(root, "merge-base", "HEAD", ref)
		return []string{"diff", "--no-ext-diff", "--binary", "-M", strings.TrimSpace(base), "HEAD"}, err
	case ModeCommit:
		return []string{"show", "--no-ext-diff", "--format=", "--binary", "-M", ref}, nil
	default:
		return nil, fmt.Errorf("unknown review mode %d", mode)
	}
}

func readTrackedMetadata(root string, baseArgs []string) ([]changeMetadata, error) {
	args := append(append([]string{}, baseArgs...), "--name-status", "-z")
	out, err := runGit(root, args...)
	if err != nil {
		return nil, err
	}
	binaryPaths, err := readTrackedBinaryPaths(root, baseArgs)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(out, "\x00")
	var result []changeMetadata
	for i := 0; i < len(fields) && fields[i] != ""; {
		status := fields[i]
		i++
		if i >= len(fields) {
			return nil, errors.New("Git returned incomplete change metadata")
		}
		kind := ChangeModified
		item := changeMetadata{}
		switch status[0] {
		case 'A':
			kind = ChangeAdded
		case 'D':
			kind = ChangeDeleted
		case 'R':
			kind = ChangeRenamed
			item.previousPath = normalizeReviewPath(fields[i])
			i++
			if i >= len(fields) {
				return nil, errors.New("Git returned incomplete rename metadata")
			}
		default:
			kind = ChangeModified
		}
		item.kind = kind
		item.path = normalizeReviewPath(fields[i])
		item.binary = binaryPaths[item.path]
		i++
		result = append(result, item)
	}
	return result, nil
}

func readTrackedBinaryPaths(root string, baseArgs []string) (map[string]bool, error) {
	args := append([]string{}, baseArgs...)
	for i, arg := range args {
		if arg == "--binary" {
			args[i] = "--numstat"
			break
		}
	}
	args = append(args, "-z")
	out, err := runGit(root, args...)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields) && fields[i] != ""; i++ {
		record := fields[i]
		firstTab := strings.IndexByte(record, '\t')
		secondRelative := -1
		if firstTab >= 0 {
			secondRelative = strings.IndexByte(record[firstTab+1:], '\t')
		}
		if firstTab < 0 || secondRelative < 0 {
			return nil, errors.New("Git returned incomplete numstat metadata")
		}
		secondTab := firstTab + 1 + secondRelative
		binary := record[:firstTab] == "-" && record[firstTab+1:secondTab] == "-"
		path := record[secondTab+1:]
		if path == "" {
			// With -z, rename records put old and new paths in the following
			// two NUL-delimited fields.
			if i+2 >= len(fields) {
				return nil, errors.New("Git returned incomplete rename numstat metadata")
			}
			i += 2
			path = fields[i]
		}
		result[normalizeReviewPath(path)] = binary
	}
	return result, nil
}

func readUntrackedMetadata(root string) ([]changeMetadata, error) {
	out, err := runGit(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	var result []changeMetadata
	for _, path := range strings.Split(out, "\x00") {
		if path != "" {
			result = append(result, changeMetadata{path: normalizeReviewPath(path), untracked: true})
		}
	}
	return result, nil
}

func buildChangePatch(root string, baseArgs []string, item changeMetadata) (string, bool, error) {
	return buildChangePatchWithRunner(root, baseArgs, item, runGitAllowDiffExit)
}

type diffGitRunner func(string, ...string) (string, error)

func buildChangePatchWithRunner(root string, baseArgs []string, item changeMetadata, runDiff diffGitRunner) (string, bool, error) {
	if item.untracked {
		return "", false, errors.New("untracked paths do not have authoritative Git patches")
	}
	args := append([]string{}, baseArgs...)
	args = append(args, "--", item.path)
	if item.previousPath != "" {
		args = append(args, item.previousPath)
	}
	patch, err := runDiff(root, args...)
	if err != nil {
		return "", false, err
	}
	return patch, item.binary, nil
}

func runGitAllowDiffExit(root string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w", args[0], err)
	}
	return string(out), nil
}
