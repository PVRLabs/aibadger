package extractor

// This file orchestrates command execution and Prompt 2 safety filtering.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

var (
	errPrompt2Excluded = errors.New("excluded from Prompt 2")

	// ErrNoSafePrompt2Files is returned when every requested file is filtered
	// out by Prompt 2 safety rules.
	ErrNoSafePrompt2Files = errors.New("no safe files available for Prompt 2 after excluding binary and sensitive files")
)

// ExtractionError reports a mix of usable extraction results and failures.
// When CanProceed is true, the caller may continue with the successful
// extractions and surface the failures as a warning.
type ExtractionError struct {
	Failures   []string
	Excluded   []string
	CanProceed bool
}

func (e *ExtractionError) Error() string {
	var parts []string
	if len(e.Failures) > 0 {
		parts = append(parts, fmt.Sprintf("extraction failed for %d command(s): %s", len(e.Failures), strings.Join(e.Failures, "; ")))
	}
	if len(e.Excluded) > 0 {
		parts = append(parts, fmt.Sprintf("%d command(s) excluded from Prompt 2 safety rules: %s", len(e.Excluded), strings.Join(e.Excluded, "; ")))
	}
	return strings.Join(parts, "; ")
}

// Extractor handles code extraction from the filesystem.
type Extractor struct {
	ProjectRoot     string
	Topology        *model.ProjectTopology
	ExternalContext []model.ExternalContext
}

type externalCommandResult struct {
	content  string
	fullFile bool
	matched  bool
}

// NewExtractor creates a new Extractor instance.
func NewExtractor(root string, t *model.ProjectTopology) *Extractor {
	var external []model.ExternalContext
	if t != nil {
		external = t.ExternalContext
	}
	return &Extractor{ProjectRoot: root, Topology: t, ExternalContext: external}
}

// Extract performs the extraction for all commands in parallel.
func (e *Extractor) Extract(commands []Command) ([]protocol.ExtractionResult, error) {
	var wg sync.WaitGroup
	results := make([]protocol.ExtractionResult, len(commands))
	errs := make([]error, len(commands))
	fileRequested := make(map[string]bool, len(commands))
	for _, cmd := range commands {
		if cmd.Type == "FILE" {
			fileRequested[cmd.Path] = true
		}
	}

	for i, cmd := range commands {
		wg.Add(1)
		go func(idx int, c Command) {
			defer wg.Done()

			content, fullFile, err := e.processCommand(c)
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = protocol.ExtractionResult{
				Path:     c.Path,
				Content:  content,
				FullFile: fullFile,
			}
		}(i, cmd)
	}

	wg.Wait()

	extracted := make([]protocol.ExtractionResult, 0, len(commands))
	emittedFile := make(map[string]bool, len(commands))
	failures := make([]string, 0)
	excludedFailures := make([]string, 0)
	excluded := 0
	for i, result := range results {
		if fileRequested[commands[i].Path] {
			if commands[i].Type != "FILE" {
				continue
			}
			if emittedFile[commands[i].Path] {
				continue
			}
			emittedFile[commands[i].Path] = true
		}
		if errs[i] != nil {
			if errors.Is(errs[i], errPrompt2Excluded) {
				excluded++
				excludedFailures = append(excludedFailures, fmt.Sprintf("%s: excluded from Prompt 2", commands[i].Path))
				continue
			}
			failures = append(failures, fmt.Sprintf("%s: %v", commands[i].Path, errs[i]))
			continue
		}
		extracted = append(extracted, result)
	}
	if len(extracted) == 0 && excluded > 0 && len(failures) == 0 {
		return nil, ErrNoSafePrompt2Files
	}
	if len(extracted) > 0 && (len(failures) > 0 || excluded > 0) {
		return extracted, &ExtractionError{
			Failures:   append([]string(nil), failures...),
			Excluded:   append([]string(nil), excludedFailures...),
			CanProceed: true,
		}
	}
	if len(failures) > 0 {
		return extracted, &ExtractionError{
			Failures: append([]string(nil), failures...),
			Excluded: append([]string(nil), excludedFailures...),
		}
	}
	return extracted, nil
}

func (e *Extractor) processCommand(cmd Command) (string, bool, error) {
	resolvedPath := e.resolveCommandPath(cmd.Path)
	if resolvedPath != "" {
		return e.processLocalCommand(cmd, resolvedPath)
	}

	externalResult, err := e.processExternalCommand(cmd)
	if err != nil {
		return "", false, err
	}
	if externalResult.matched {
		return externalResult.content, externalResult.fullFile, nil
	}

	return "", false, fmt.Errorf("file not found: %s", cmd.Path)
}

func (e *Extractor) processLocalCommand(cmd Command, resolvedPath string) (string, bool, error) {
	if promptpolicy.IsSensitivePath(resolvedPath) {
		return "", false, errPrompt2Excluded
	}

	absolutePath := filepath.Join(e.ProjectRoot, resolvedPath)
	if !isWithinProjectRoot(e.ProjectRoot, absolutePath) {
		return "", false, fmt.Errorf("file not found: %s", cmd.Path)
	}
	kind := filekind.Classify(absolutePath)
	if kind == model.FileKindBinary {
		return "", false, errPrompt2Excluded
	}
	if kind == model.FileKindAsset {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return "", false, fmt.Errorf("file not found: %s", cmd.Path)
		}
		return summarizeBinaryFile(resolvedPath, int(info.Size()), kind), false, nil
	}

	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return "", false, fmt.Errorf("file not found: %s", cmd.Path)
	}

	content := string(data)
	if cmd.Type == "FILE" {
		return content, true, nil
	}

	extracted, fullFile, err := e.extractBlock(content, cmd.Type, cmd.Pattern)
	if err != nil {
		return "", false, err
	}
	return extracted, fullFile, nil
}

func (e *Extractor) processExternalCommand(cmd Command) (externalCommandResult, error) {
	requestPath := cmd.Path
	if len(e.ExternalContext) == 0 {
		return externalCommandResult{}, nil
	}
	resolution := externalcontext.ResolveFile(e.ProjectRoot, e.ExternalContext, requestPath)
	matches := filterExternalMatchesForCommand(cmd, resolution.Matches)
	if len(matches) > 1 {
		return externalCommandResult{matched: true}, ambiguousExternalFileError(requestPath, matches)
	}
	if len(matches) == 0 {
		return externalCommandResult{}, nil
	}
	match := matches[0]
	result := externalCommandResult{matched: true}
	if promptpolicy.IsSensitivePath(requestPath) || promptpolicy.IsSensitivePath(match.RelPath) {
		return result, errPrompt2Excluded
	}
	absolutePath := match.AbsPath
	kind := filekind.Classify(absolutePath)
	if kind == model.FileKindBinary {
		return result, errPrompt2Excluded
	}
	if kind == model.FileKindAsset {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return result, fmt.Errorf("file not found: %s", requestPath)
		}
		result.content = summarizeBinaryFile(requestPath, int(info.Size()), kind)
		result.fullFile = cmd.Type == "FILE"
		return result, nil
	}
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return result, fmt.Errorf("file not found: %s", requestPath)
	}
	content := string(data)
	if cmd.Type == "FILE" {
		result.content = content
		result.fullFile = true
		return result, nil
	}

	extracted, fullFile, err := e.extractBlock(content, cmd.Type, cmd.Pattern)
	if err != nil {
		return result, err
	}
	result.content = extracted
	result.fullFile = fullFile
	return result, nil
}

func filterExternalMatchesForCommand(cmd Command, matches []externalcontext.FileMatch) []externalcontext.FileMatch {
	if cmd.Type == "FILE" {
		return matches
	}
	requestPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(cmd.Path))))
	filtered := make([]externalcontext.FileMatch, 0, len(matches))
	for _, match := range matches {
		if requestPath == match.RelPath || requestPath == match.DisplayPath || requestPath == filepath.ToSlash(match.AbsPath) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func ambiguousExternalFileError(requestPath string, matches []externalcontext.FileMatch) error {
	var sb strings.Builder
	sb.WriteString("Ambiguous file reference: ")
	sb.WriteString(requestPath)
	sb.WriteString("\n\nMatches:")
	for _, match := range matches {
		sb.WriteString("\n- ")
		sb.WriteString(match.DisplayPath)
	}
	sb.WriteString("\n\nUse a more specific path to disambiguate.")
	return errors.New(sb.String())
}

func isWithinProjectRoot(root, absPath string) bool {
	rootClean := filepath.Clean(root)
	absClean := filepath.Clean(absPath)
	return absClean == rootClean || strings.HasPrefix(absClean, rootClean+string(filepath.Separator))
}

func summarizeBinaryFile(path string, size int, kind string) string {
	return fmt.Sprintf("Binary file: %s (%s, kind: %s)\n", path, protocol.FormatFileSize(int64(size)), kind)
}
