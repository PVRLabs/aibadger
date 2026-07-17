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
// reference against the project root or external context.
type ResolvedPath struct {
	Path       string
	AbsPath    string
	IsDir      bool
	Source     SourceKind
	SourceRoot string // Absolute path of the root that resolved this reference
}

// SourceKind describes whether a tagged file reference resolved project-locally
// or to an external context root.
type SourceKind int

const (
	SourceLocal SourceKind = iota
	SourceExternal
)

// ExternalRoot represents a configured read-only context directory.
type ExternalRoot struct {
	Path      string                             // Display path (e.g. from .badger-context)
	AbsPath   string                             // Absolute path on disk
	IsOmitted func(relPath, absPath string) bool // Optional validator
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
// normalized repo-relative file path. It falls back to externalRoots if no
// local match exists.
func Resolve(projectRoot, relPath string, externalRoots []ExternalRoot) (ResolvedPath, error) {
	resolved, localErr := resolveExistingPath(projectRoot, relPath)
	if localErr == nil {
		if resolved.IsDir {
			return ResolvedPath{}, fmt.Errorf("tagged file path is a directory: %s", relPath)
		}
		resolved.Source = SourceLocal
		return resolved, nil
	}

	// Fallback to external context if local resolution failed because the path
	// didn't exist or it escaped the project root.
	isNotFound := strings.Contains(localErr.Error(), "does not exist")
	isEscape := strings.Contains(localErr.Error(), "escapes project root")
	if !isNotFound && !isEscape {
		return ResolvedPath{}, localErr
	}

	var matches []ResolvedPath
	seenAbs := make(map[string]bool)

	addMatch := func(m ResolvedPath, root ExternalRoot, rootRelPath string) {
		if seenAbs[m.AbsPath] || m.IsDir {
			return
		}
		if root.IsOmitted != nil && root.IsOmitted(rootRelPath, m.AbsPath) {
			return
		}
		m.Path = relPath // Preserve original reference path for display and fallback resolution
		m.Source = SourceExternal
		m.SourceRoot = root.AbsPath
		matches = append(matches, m)
		seenAbs[m.AbsPath] = true
	}

	for _, root := range externalRoots {
		// Strategy 1: Exact match or relative to external root (handles @spec.md)
		if m, err := resolveExistingPath(root.AbsPath, relPath); err == nil {
			addMatch(m, root, relPath)
		}

		// Strategy 2: relPath starts with root display path (handles @../badger-sidecar/docs/spec.md)
		displayPrefix := root.Path + "/"
		if strings.HasPrefix(relPath, displayPrefix) {
			remainder := relPath[len(displayPrefix):]
			if m, err := resolveExistingPath(root.AbsPath, remainder); err == nil {
				addMatch(m, root, remainder)
			}
		}

		// Strategy 3: relPath starts with stripped display path (handles @badger-sidecar/docs/spec.md)
		stripped := strings.TrimLeft(root.Path, "./")
		if stripped != "" && strings.HasPrefix(relPath, stripped+"/") {
			remainder := relPath[len(stripped)+1:]
			if m, err := resolveExistingPath(root.AbsPath, remainder); err == nil {
				addMatch(m, root, remainder)
			}
		}

		// Strategy 4: Match the configured root's final path component
		// (handles @docs/spec.md for ../badger-sidecar/docs).
		if alias := externalRootTagPrefix(root); alias != "" && strings.HasPrefix(relPath, alias+"/") {
			remainder := relPath[len(alias)+1:]
			if m, err := resolveExistingPath(root.AbsPath, remainder); err == nil {
				addMatch(m, root, remainder)
			}
		}
	}

	// Strategy 5: Loose resolution from project root (handles @../badger-sidecar/docs/spec.md safely)
	if isEscape {
		absRoot, _ := filepath.Abs(projectRoot)
		fullPath := filepath.Clean(filepath.Join(absRoot, filepath.FromSlash(relPath)))
		if resolved, err := filepath.EvalSymlinks(fullPath); err == nil {
			fullPath = resolved
		}

		for _, root := range externalRoots {
			rel, err := filepath.Rel(root.AbsPath, fullPath)
			if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
					addMatch(ResolvedPath{
						Path:    relPath,
						AbsPath: fullPath,
						IsDir:   false,
					}, root, filepath.ToSlash(rel))
				}
			}
		}
	}

	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		return ResolvedPath{}, fmt.Errorf("ambiguous tagged file reference %q: matches multiple external context roots", relPath)
	}

	return ResolvedPath{}, localErr
}

// Complete returns shallow suggestions for a tagged-file prefix. It includes
// external context suggestions when no local match exists or as lower-ranked
// alternatives.
func Complete(projectRoot, prefix string, externalRoots []ExternalRoot, limit int, skip SkipFunc) ([]Suggestion, error) {
	if limit <= 0 {
		limit = defaultCompletionLimit
	}

	if err := validateCompletionPrefix(prefix); err != nil {
		return nil, err
	}

	if hasExplicitCompletionDirectory(prefix) {
		return completeExplicitDirectoryPrefix(projectRoot, prefix, externalRoots, limit, skip)
	}

	candidates, err := collectShallowCompletionCandidates(projectRoot, skip)
	if err != nil {
		return nil, err
	}

	scored := make([]scoredSuggestion, 0, len(candidates))
	seenPaths := make(map[string]bool)
	localPaths := make(map[string]bool)
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
		seenPaths[candidate.Path] = true
		localPaths[candidate.Path] = true
	}

	for _, root := range externalRoots {
		extCandidates, err := collectShallowCompletionCandidates(root.AbsPath, func(relPath string, isDir bool) bool {
			if skip != nil && skip(relPath, isDir) {
				return true
			}
			if root.IsOmitted != nil && root.IsOmitted(relPath, filepath.Join(root.AbsPath, relPath)) {
				return true
			}
			return false
		})
		if err != nil {
			continue
		}
		for _, candidate := range extCandidates {
			if localPaths[candidate.Path] {
				continue
			}
			candidate = externalCompletionCandidate(root, candidate)
			if seenPaths[candidate.Path] {
				continue
			}
			rank, ok := scoreCompletionCandidate(prefix, candidate)
			if !ok {
				continue
			}
			// External context matches are ranked slightly lower than local
			// matches with the same score.
			scored = append(scored, scoredSuggestion{
				suggestion: Suggestion{
					Path:  candidate.Path,
					IsDir: candidate.IsDir,
				},
				rank:       rank,
				isExternal: true,
			})
			seenPaths[candidate.Path] = true
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].rank != scored[j].rank {
			return scored[i].rank < scored[j].rank
		}
		if scored[i].isExternal != scored[j].isExternal {
			return !scored[i].isExternal
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
	isExternal bool
}

func hasExplicitCompletionDirectory(prefix string) bool {
	return strings.Contains(strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/")), "/")
}

func completeExplicitDirectoryPrefix(projectRoot, prefix string, externalRoots []ExternalRoot, limit int, skip SkipFunc) ([]Suggestion, error) {
	local, err := collectExplicitDirectorySuggestions(projectRoot, prefix, skip)
	localErr := err

	scored := make([]scoredSuggestion, 0, len(local))
	seenPaths := make(map[string]bool)
	if localErr == nil {
		for _, suggestion := range local {
			scored = append(scored, scoredSuggestion{suggestion: suggestion})
			seenPaths[suggestion.Path] = true
		}
	}

	for _, root := range externalRoots {
		for _, rootPrefix := range externalCompletionSearchPrefixes(root, prefix) {
			extSuggestions, err := collectExplicitDirectorySuggestions(root.AbsPath, rootPrefix, func(relPath string, isDir bool) bool {
				if skip != nil && skip(relPath, isDir) {
					return true
				}
				if root.IsOmitted != nil && root.IsOmitted(relPath, filepath.Join(root.AbsPath, filepath.FromSlash(relPath))) {
					return true
				}
				return false
			})
			if err != nil {
				continue
			}
			for _, suggestion := range extSuggestions {
				suggestion.Path = externalCompletionPath(root, suggestion.Path, suggestion.IsDir)
				if seenPaths[suggestion.Path] {
					continue
				}
				scored = append(scored, scoredSuggestion{
					suggestion: suggestion,
					isExternal: true,
				})
				seenPaths[suggestion.Path] = true
			}
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].isExternal != scored[j].isExternal {
			return !scored[i].isExternal
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
	if len(suggestions) == 0 && localErr != nil {
		return nil, localErr
	}
	return suggestions, nil
}

func collectExplicitDirectorySuggestions(projectRoot, prefix string, skip SkipFunc) ([]Suggestion, error) {
	normalized := strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/"))
	dirPrefix, namePrefix := splitExplicitCompletionPrefix(normalized)

	resolvedDir, err := resolveCompletionDirectory(projectRoot, dirPrefix)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolvedDir.AbsPath)
	if err != nil {
		return nil, err
	}

	var suggestions []Suggestion
	for _, entry := range entries {
		name := entry.Name()
		if namePrefix != "" && !hasPrefixFold(name, namePrefix) {
			continue
		}
		relPath := joinTaggedPath(resolvedDir.Path, name)
		isDir := entry.IsDir()
		if skip != nil && skip(relPath, isDir) {
			continue
		}
		suggestions = append(suggestions, Suggestion{
			Path:  dirSuffix(relPath, isDir),
			IsDir: isDir,
		})
	}
	return suggestions, nil
}

func externalRootTagPrefix(root ExternalRoot) string {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(root.Path)))
	base := filepath.Base(clean)
	if base == "" || base == "." || base == ".." || base == string(filepath.Separator) {
		return ""
	}
	return filepath.ToSlash(base)
}

func externalCompletionCandidate(root ExternalRoot, candidate shallowCompletionCandidate) shallowCompletionCandidate {
	if externalRootTagPrefix(root) == "" {
		return candidate
	}
	candidate.Path = externalCompletionPath(root, candidate.Path, candidate.IsDir)
	candidate.Depth++
	return candidate
}

func externalCompletionPath(root ExternalRoot, relPath string, isDir bool) string {
	alias := externalRootTagPrefix(root)
	if alias == "" {
		return relPath
	}
	path := joinTaggedPath(alias, strings.TrimSuffix(relPath, "/"))
	return dirSuffix(path, isDir)
}

func externalCompletionSearchPrefixes(root ExternalRoot, prefix string) []string {
	prefixes := []string{prefix}
	alias := externalRootTagPrefix(root)
	normalized := strings.TrimSpace(strings.ReplaceAll(prefix, "\\", "/"))
	if alias == "" || len(normalized) < len(alias) || !strings.EqualFold(normalized[:len(alias)], alias) {
		return prefixes
	}
	if len(normalized) == len(alias) {
		return append(prefixes, "")
	}
	if normalized[len(alias)] != '/' {
		return prefixes
	}
	return append(prefixes, normalized[len(alias)+1:])
}

func splitExplicitCompletionPrefix(prefix string) (string, string) {
	trimmed := strings.TrimSuffix(prefix, "/")
	if strings.HasSuffix(prefix, "/") {
		return trimmed, ""
	}
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 {
		return "", trimmed
	}
	return trimmed[:idx], trimmed[idx+1:]
}

func resolveCompletionDirectory(projectRoot, relPath string) (ResolvedPath, error) {
	clean := strings.TrimSpace(relPath)
	if clean == "" {
		return resolveExistingDirectory(projectRoot, "")
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return ResolvedPath{}, fmt.Errorf("failed to resolve project root: %w", err)
	}

	currentAbs := absRoot
	var actualSegments []string
	for _, segment := range strings.Split(strings.ReplaceAll(clean, "\\", "/"), "/") {
		if segment == "" || segment == "." {
			continue
		}
		entries, err := os.ReadDir(currentAbs)
		if err != nil {
			return ResolvedPath{}, err
		}
		var matched os.DirEntry
		for _, entry := range entries {
			if strings.EqualFold(entry.Name(), segment) {
				matched = entry
				break
			}
		}
		if matched == nil {
			return ResolvedPath{}, fmt.Errorf("tagged file completion directory does not exist: %s", relPath)
		}
		if !matched.IsDir() {
			return ResolvedPath{}, fmt.Errorf("tagged file completion prefix is not a directory: %s", relPath)
		}
		currentAbs = filepath.Join(currentAbs, matched.Name())
		actualSegments = append(actualSegments, matched.Name())
	}

	resolved, err := resolveExistingDirectory(projectRoot, strings.Join(actualSegments, "/"))
	if err != nil {
		return ResolvedPath{}, err
	}
	return resolved, nil
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
			continue
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

func hasPrefixFold(s, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

func scoreCompletionCandidate(prefix string, candidate shallowCompletionCandidate) (int, bool) {
	query := strings.TrimSpace(prefix)
	if query == "" {
		return completionEmptyRank(candidate), true
	}

	if !candidate.IsDir {
		if candidate.Depth == 0 && hasPrefixFold(candidate.base(), query) {
			return 0, true
		}
		if candidate.Depth == 1 && hasPrefixFold(candidate.base(), query) {
			return 1, true
		}
		if hasPrefixFold(candidate.Path, query) {
			return 2, true
		}
		if hasSegmentPrefixMatch(candidate.Path, query) {
			return 3, true
		}
		return 0, false
	}

	if candidate.Depth == 0 && hasPrefixFold(candidate.base(), query) {
		return 4, true
	}
	if candidate.Depth == 1 && hasPrefixFold(candidate.base(), query) {
		return 5, true
	}
	if hasPrefixFold(candidate.Path, query) {
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
	if hasPrefixFold(segment, query) {
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
		if hasPrefixFold(part, query) {
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
