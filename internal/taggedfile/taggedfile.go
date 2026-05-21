package taggedfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const defaultCompletionLimit = 8

// Reference describes a tagged-file token found in goal text.
type Reference struct {
	Raw          string
	Path         string
	Quoted       bool
	Start        int
	End          int
	ContentStart int
	ContentEnd   int
}

// ResolvedPath is the normalized path returned after validating a tagged file
// reference against the project root.
type ResolvedPath struct {
	Path    string
	AbsPath string
	IsDir   bool
}

// Suggestion is a shallow tagged-file completion candidate.
type Suggestion struct {
	Path  string
	IsDir bool
}

// SkipFunc lets callers filter completion candidates without baking policy into
// the helper itself.
type SkipFunc func(relPath string, isDir bool) bool

var defaultCompletionIgnoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"target":       true,
	"build":        true,
}

// DefaultCompletionSkip filters known noisy directories from shallow
// completion suggestions.
func DefaultCompletionSkip(relPath string, isDir bool) bool {
	if !isDir {
		return false
	}
	return defaultCompletionIgnoredDirs[filepath.Base(strings.TrimSuffix(relPath, "/"))]
}

// Parse extracts tagged-file references from arbitrary text.
func Parse(input string) ([]Reference, []error) {
	var refs []Reference
	var errs []error

	for i := 0; i < len(input); {
		if input[i] != '@' {
			_, width := utf8.DecodeRuneInString(input[i:])
			if width <= 0 {
				break
			}
			i += width
			continue
		}

		ref, ok, err := scanReference(input, i, false)
		if err != nil {
			errs = append(errs, err)
			i++
			continue
		}
		if ok {
			refs = append(refs, ref)
			i = ref.End
			continue
		}
		i++
	}

	return refs, errs
}

// ActiveTokenAt finds the tagged-file token that is active at cursor.
func ActiveTokenAt(input string, cursor int) (Reference, bool) {
	if cursor < 0 || cursor > len(input) {
		return Reference{}, false
	}

	limit := cursor
	if limit > len(input) {
		limit = len(input)
	}

	for i := limit - 1; i >= 0; i-- {
		if input[i] != '@' {
			continue
		}
		if !hasTaggedBoundary(input, i) {
			continue
		}
		ref, ok, _ := scanReference(input, i, true)
		if !ok {
			continue
		}
		if cursor < ref.Start || cursor > ref.End {
			continue
		}
		return ref, true
	}

	return Reference{}, false
}

// Resolve validates a tagged-file reference against projectRoot and returns a
// normalized repo-relative file path.
func Resolve(projectRoot, relPath string) (ResolvedPath, error) {
	resolved, err := resolveExistingPath(projectRoot, relPath)
	if err != nil {
		return ResolvedPath{}, err
	}
	if resolved.IsDir {
		return ResolvedPath{}, fmt.Errorf("tagged file path is a directory: %s", relPath)
	}
	return resolved, nil
}

// Complete returns shallow suggestions for a tagged-file prefix.
func Complete(projectRoot, prefix string, limit int, skip SkipFunc) ([]Suggestion, error) {
	if limit <= 0 {
		limit = defaultCompletionLimit
	}

	if err := validateCompletionPrefix(prefix); err != nil {
		return nil, err
	}

	candidates, err := collectShallowCompletionCandidates(projectRoot, skip)
	if err != nil {
		return nil, err
	}

	scored := make([]scoredSuggestion, 0, len(candidates))
	for _, candidate := range candidates {
		rank, ok := scoreCompletionCandidate(prefix, candidate)
		if !ok {
			continue
		}
		scored = append(scored, scoredSuggestion{
			suggestion: Suggestion{
				Path:  candidate.Path,
				IsDir: candidate.IsDir,
			},
			rank: rank,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].rank != scored[j].rank {
			return scored[i].rank < scored[j].rank
		}
		if scored[i].suggestion.IsDir != scored[j].suggestion.IsDir {
			return !scored[i].suggestion.IsDir && scored[j].suggestion.IsDir
		}
		return scored[i].suggestion.Path < scored[j].suggestion.Path
	})

	if len(scored) > limit {
		scored = scored[:limit]
	}
	suggestions := make([]Suggestion, 0, len(scored))
	for _, item := range scored {
		suggestions = append(suggestions, item.suggestion)
	}
	return suggestions, nil
}

type shallowCompletionCandidate struct {
	Path  string
	IsDir bool
	Depth int
}

type scoredSuggestion struct {
	suggestion Suggestion
	rank       int
}

func collectShallowCompletionCandidates(projectRoot string, skip SkipFunc) ([]shallowCompletionCandidate, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve project root: %w", err)
	}

	entries, err := os.ReadDir(absRoot)
	if err != nil {
		return nil, err
	}

	var candidates []shallowCompletionCandidate
	for _, entry := range entries {
		name := entry.Name()
		isDir := entry.IsDir()
		relPath := name
		if skip != nil && skip(relPath, isDir) {
			continue
		}
		candidates = append(candidates, shallowCompletionCandidate{
			Path:  dirSuffix(relPath, isDir),
			IsDir: isDir,
			Depth: 0,
		})
		if !isDir {
			continue
		}

		childEntries, err := os.ReadDir(filepath.Join(absRoot, name))
		if err != nil {
			return nil, err
		}
		for _, child := range childEntries {
			childPath := joinTaggedPath(name, child.Name())
			childIsDir := child.IsDir()
			if skip != nil && skip(childPath, childIsDir) {
				continue
			}
			candidates = append(candidates, shallowCompletionCandidate{
				Path:  dirSuffix(childPath, childIsDir),
				IsDir: childIsDir,
				Depth: 1,
			})
		}
	}

	return candidates, nil
}

func scoreCompletionCandidate(prefix string, candidate shallowCompletionCandidate) (int, bool) {
	query := strings.TrimSpace(prefix)
	if query == "" {
		return completionEmptyRank(candidate), true
	}

	if !candidate.IsDir {
		if candidate.Depth == 0 && strings.HasPrefix(candidate.base(), query) {
			return 0, true
		}
		if candidate.Depth == 1 && strings.HasPrefix(candidate.base(), query) {
			return 1, true
		}
		if strings.HasPrefix(candidate.Path, query) {
			return 2, true
		}
		if hasSegmentPrefixMatch(candidate.Path, query) {
			return 3, true
		}
		return 0, false
	}

	if candidate.Depth == 0 && strings.HasPrefix(candidate.base(), query) {
		return 4, true
	}
	if candidate.Depth == 1 && strings.HasPrefix(candidate.base(), query) {
		return 5, true
	}
	if strings.HasPrefix(candidate.Path, query) {
		return 6, true
	}
	if hasSegmentPrefixMatch(candidate.Path, query) {
		return 7, true
	}
	return 0, false
}

func completionEmptyRank(candidate shallowCompletionCandidate) int {
	if candidate.IsDir {
		return 4 + candidate.Depth
	}
	return candidate.Depth
}

func (c shallowCompletionCandidate) base() string {
	path := strings.TrimSuffix(c.Path, "/")
	return filepath.Base(filepath.FromSlash(path))
}

func hasSegmentPrefixMatch(path, query string) bool {
	trimmed := strings.TrimSuffix(path, "/")
	for _, segment := range strings.Split(trimmed, "/") {
		if segmentHasPrefix(segment, query) {
			return true
		}
	}
	return false
}

func segmentHasPrefix(segment, query string) bool {
	if segment == "" || query == "" {
		return false
	}
	if strings.HasPrefix(segment, query) {
		return true
	}
	for _, part := range strings.FieldsFunc(segment, func(r rune) bool {
		switch r {
		case '.', '-', '_', '+':
			return true
		default:
			return false
		}
	}) {
		if strings.HasPrefix(part, query) {
			return true
		}
	}
	return false
}

func validateCompletionPrefix(prefix string) error {
	trimmed := strings.TrimSpace(prefix)
	if trimmed == "" {
		return nil
	}
	if filepath.IsAbs(trimmed) {
		return fmt.Errorf("tagged file completion prefix must be repo-relative: %s", prefix)
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return fmt.Errorf("tagged file completion prefix escapes project root: %s", prefix)
		}
	}
	return nil
}

func scanReference(input string, at int, allowIncomplete bool) (Reference, bool, error) {
	if at < 0 || at >= len(input) || input[at] != '@' {
		return Reference{}, false, nil
	}
	if !hasTaggedBoundary(input, at) {
		return Reference{}, false, nil
	}

	ref := Reference{
		Start:        at,
		ContentStart: at + 1,
		ContentEnd:   at + 1,
	}
	if ref.ContentStart >= len(input) {
		ref.Raw = "@"
		ref.End = at + 1
		return ref, allowIncomplete, nil
	}

	if input[ref.ContentStart] == '"' {
		ref.Quoted = true
		ref.ContentStart++
		path, end, ok, err := scanQuotedPath(input, ref.ContentStart, allowIncomplete)
		if err != nil {
			return Reference{}, false, err
		}
		if !ok {
			return Reference{}, false, nil
		}
		ref.Path = path
		if end > 0 && input[end-1] == '"' {
			ref.ContentEnd = end - 1
		} else {
			ref.ContentEnd = end
		}
		ref.End = end
		ref.Raw = input[at:end]
		return ref, true, nil
	}

	path, end := scanUnquotedPath(input, ref.ContentStart)
	if path == "" {
		return Reference{}, false, nil
	}
	ref.Path = path
	ref.ContentEnd = end
	ref.End = end
	ref.Raw = input[at:end]
	return ref, true, nil
}

func scanQuotedPath(input string, start int, allowIncomplete bool) (string, int, bool, error) {
	var b strings.Builder
	for i := start; i < len(input); {
		r, width := utf8.DecodeRuneInString(input[i:])
		if r == utf8.RuneError && width == 1 {
			b.WriteByte(input[i])
			i++
			continue
		}
		if r == '\\' {
			nextIndex := i + width
			if nextIndex >= len(input) {
				if allowIncomplete {
					return b.String(), len(input), true, nil
				}
				return "", 0, false, fmt.Errorf("malformed tagged file reference: missing closing quote")
			}
			nextRune, nextWidth := utf8.DecodeRuneInString(input[nextIndex:])
			if nextRune == '"' || nextRune == '\\' {
				b.WriteRune(nextRune)
				i = nextIndex + nextWidth
				continue
			}
			b.WriteRune(r)
			i += width
			continue
		}
		if r == '"' {
			end := i + width
			return b.String(), end, true, nil
		}
		b.WriteRune(r)
		i += width
	}

	if allowIncomplete {
		return b.String(), len(input), true, nil
	}
	return "", 0, false, fmt.Errorf("malformed tagged file reference: missing closing quote")
}

func scanUnquotedPath(input string, start int) (string, int) {
	var b strings.Builder
	i := start
	for i < len(input) {
		r, width := utf8.DecodeRuneInString(input[i:])
		if !isTaggedPathRune(r) {
			break
		}
		b.WriteRune(r)
		i += width
	}

	path := strings.TrimRight(b.String(), ".,;:)]}")
	if path == "" {
		return "", start
	}
	end := start + len(path)
	return path, end
}

func isTaggedPathRune(r rune) bool {
	switch {
	case unicode.IsLetter(r), unicode.IsDigit(r):
		return true
	case r == '/', r == '.', r == '-', r == '_', r == '~', r == '+':
		return true
	default:
		return false
	}
}

func hasTaggedBoundary(input string, at int) bool {
	if at == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(input[:at])
	return !(unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_')
}

func resolveExistingPath(projectRoot, relPath string) (ResolvedPath, error) {
	clean := strings.TrimSpace(relPath)
	if clean == "" {
		return ResolvedPath{}, fmt.Errorf("tagged file path is empty")
	}
	if filepath.IsAbs(clean) {
		return ResolvedPath{}, fmt.Errorf("tagged file path must be repo-relative: %s", relPath)
	}

	clean = filepath.Clean(filepath.FromSlash(clean))
	if clean == "." || clean == "" {
		return ResolvedPath{}, fmt.Errorf("tagged file path is empty")
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ResolvedPath{}, fmt.Errorf("tagged file path escapes project root: %s", relPath)
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("failed to resolve project root: %w", err)
	}
	realRoot := absRoot
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		realRoot = resolvedRoot
	}

	fullPath := filepath.Clean(filepath.Join(absRoot, clean))
	info, err := os.Lstat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedPath{}, fmt.Errorf("tagged file path does not exist: %s", relPath)
		}
		return ResolvedPath{}, fmt.Errorf("failed to inspect tagged file path %s: %w", relPath, err)
	}

	resolvedPath := fullPath
	if resolved, err := filepath.EvalSymlinks(fullPath); err == nil {
		resolvedPath = resolved
	}
	rel, err := filepath.Rel(realRoot, resolvedPath)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("failed to validate tagged file path %s: %w", relPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ResolvedPath{}, fmt.Errorf("tagged file path escapes project root: %s", relPath)
	}

	return ResolvedPath{
		Path:    filepath.ToSlash(clean),
		AbsPath: fullPath,
		IsDir:   info.IsDir(),
	}, nil
}

func resolveExistingDirectory(projectRoot, relPath string) (ResolvedPath, error) {
	clean := strings.TrimSpace(relPath)
	if clean == "" {
		absRoot, err := filepath.Abs(projectRoot)
		if err != nil {
			return ResolvedPath{}, fmt.Errorf("failed to resolve project root: %w", err)
		}
		return ResolvedPath{Path: "", AbsPath: absRoot, IsDir: true}, nil
	}

	resolved, err := resolveExistingPath(projectRoot, relPath)
	if err != nil {
		return ResolvedPath{}, err
	}
	if !resolved.IsDir {
		return ResolvedPath{}, fmt.Errorf("tagged file completion prefix is not a directory: %s", relPath)
	}
	return resolved, nil
}

func joinTaggedPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return filepath.ToSlash(filepath.Join(filepath.FromSlash(parent), name))
}

func dirSuffix(path string, isDir bool) string {
	if isDir {
		return path + "/"
	}
	return path
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
