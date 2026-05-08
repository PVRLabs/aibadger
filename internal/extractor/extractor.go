package extractor

// This file orchestrates command execution and Prompt 2 safety filtering.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// Extractor handles code extraction from the filesystem.
type Extractor struct {
	ProjectRoot string
	Topology    *model.ProjectTopology
}

// NewExtractor creates a new Extractor instance.
func NewExtractor(root string, t *model.ProjectTopology) *Extractor {
	return &Extractor{ProjectRoot: root, Topology: t}
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
		return extracted, fmt.Errorf("extraction failed for %d command(s): %s", len(failures), strings.Join(failures, "; "))
	}
	return extracted, nil
}

func (e *Extractor) processCommand(cmd Command) (string, bool, error) {
	resolvedPath := e.resolveCommandPath(cmd.Path)
	if resolvedPath == "" {
		return "", false, fmt.Errorf("file not found: %s", cmd.Path)
	}

	if promptpolicy.IsSensitivePath(resolvedPath) {
		return "", false, errPrompt2Excluded
	}

	absolutePath := filepath.Join(e.ProjectRoot, resolvedPath)
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

func summarizeBinaryFile(path string, size int, kind string) string {
	return fmt.Sprintf("Binary file: %s (%s, kind: %s)\n", path, protocol.FormatFileSize(int64(size)), kind)
}
