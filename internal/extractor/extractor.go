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
	if len(failures) > 0 {
		if len(extracted) > 0 && excluded == 0 {
			return extracted, &ExtractionError{
				Failures:   append([]string(nil), failures...),
				CanProceed: true,
			}
		}
		return extracted, &ExtractionError{
			Failures: append([]string(nil), failures...),
			Excluded: append([]string(nil), excludedFailures...),
		}
	}
	return extracted, nil
}

func (e *Extractor) processCommand(cmd Command) (string, bool, error) {
	if cmd.Type == "FILE" {
		if content, ok, err := e.processExternalFileCommand(cmd.Path); ok || err != nil {
			return content, true, err
		}
	}

	resolvedPath := e.resolveCommandPath(cmd.Path)
	if resolvedPath == "" {
		return "", false, fmt.Errorf("file not found: %s", cmd.Path)
	}

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

func (e *Extractor) processExternalFileCommand(requestPath string) (string, bool, error) {
	if len(e.ExternalContext) == 0 {
		return "", false, nil
	}
	_, absolutePath, ok := externalcontext.ContainsFile(e.ProjectRoot, e.ExternalContext, requestPath)
	if !ok {
		return "", false, nil
	}
	if promptpolicy.IsSensitivePath(requestPath) {
		return "", true, errPrompt2Excluded
	}
	kind := filekind.Classify(absolutePath)
	if kind == model.FileKindBinary {
		return "", true, errPrompt2Excluded
	}
	if kind == model.FileKindAsset {
		info, err := os.Stat(absolutePath)
		if err != nil {
			return "", true, fmt.Errorf("file not found: %s", requestPath)
		}
		return summarizeBinaryFile(requestPath, int(info.Size()), kind), true, nil
	}
	data, err := os.ReadFile(absolutePath)
	if err != nil {
		return "", true, fmt.Errorf("file not found: %s", requestPath)
	}
	return string(data), true, nil
}

func isWithinProjectRoot(root, absPath string) bool {
	rootClean := filepath.Clean(root)
	absClean := filepath.Clean(absPath)
	return absClean == rootClean || strings.HasPrefix(absClean, rootClean+string(filepath.Separator))
}

func summarizeBinaryFile(path string, size int, kind string) string {
	return fmt.Sprintf("Binary file: %s (%s, kind: %s)\n", path, protocol.FormatFileSize(int64(size)), kind)
}
