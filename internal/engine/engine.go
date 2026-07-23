package engine

import (
	"errors"
	"fmt"
	"strings"

	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/scanner"
	"github.com/PVRLabs/aibadger/internal/taggedfile"
	"github.com/PVRLabs/aibadger/internal/writer"
)

// Engine coordinates Badger's core scan, extraction, formatting, and
// write-planning workflow. CLI and future integration layers should depend on
// this orchestration boundary instead of wiring lower-level packages directly.
type Engine struct {
	Root                 string
	Topology             *model.ProjectTopology
	maxFilesPerDirectory int
	formatter            *protocol.Formatter
	extractor            *extractor.Extractor
}

// New scans the project root and creates an engine for follow-up workflow
// steps. If maxFilesPerDir is 0, scanning is unlimited.
func New(root string, maxFilesPerDir int) (*Engine, error) {
	if err := CheckDisabled(root); err != nil {
		return nil, err
	}

	s := scanner.NewScanner(root)
	s.MaxFilesPerDirectory = maxFilesPerDir
	topology, err := s.Scan()
	if err != nil {
		return nil, err
	}

	return &Engine{
		Root:                 root,
		Topology:             topology,
		maxFilesPerDirectory: maxFilesPerDir,
		formatter:            protocol.NewFormatter(),
		extractor:            extractor.NewExtractor(root, topology),
	}, nil
}

// FromTopology creates an engine around an existing topology. This is useful
// when the caller already owns scan timing or summary output.
func FromTopology(root string, topology *model.ProjectTopology) *Engine {
	return &Engine{
		Root:      root,
		Topology:  topology,
		formatter: protocol.NewFormatter(),
		extractor: extractor.NewExtractor(root, topology),
	}
}

// SetMaxPackages configures Schema A truncation. A zero value means no limit.
func (e *Engine) SetMaxPackages(maxPackages int) {
	e.formatter.MaxPackages = maxPackages
}

// SetMaxContextFileBytes configures per-file truncation in Schema B.
func (e *Engine) SetMaxContextFileBytes(limit int) {
	e.formatter.MaxContextFileBytes = limit
}

// SetMaxTotalContextBytes configures total payload truncation in Schema B.
func (e *Engine) SetMaxTotalContextBytes(limit int) {
	e.formatter.MaxTotalContextBytes = limit
}

// SetPromptInstructions configures the LLM constraints used by the formatter.
func (e *Engine) SetPromptInstructions(instr protocol.PromptInstructions) {
	e.formatter.SetPromptInstructions(instr)
}

// SetFocus configures the prompt framing preset used by the formatter.
func (e *Engine) SetFocus(focus protocol.Focus) {
	e.formatter.SetFocus(focus)
}

// GenerateMap builds Prompt 1: Topology.
func (e *Engine) GenerateMap(goal string) string {
	schema, _ := e.GenerateMapDetailed(goal)
	return schema
}

// GenerateTopology builds the formatted topology portion of Prompt 1.
func (e *Engine) GenerateTopology() string {
	if e == nil || e.formatter == nil {
		return ""
	}
	return e.formatter.GenerateTopology(e.Topology)
}

// GenerateMapDetailed builds Prompt 1: Topology and returns any non-blocking
// tagged-file warnings collected from the submitted goal.
func (e *Engine) GenerateMapDetailed(goal string) (string, []string) {
	if e == nil || e.formatter == nil {
		return "", nil
	}
	taggedFiles, warnings := e.resolveTaggedFiles(goal)
	return e.formatter.GenerateSchemaAWithTaggedFiles(e.Topology, goal, taggedFiles), warnings
}

// ParseCommands parses FILE/PREFIX/NEAR extraction commands.
func (e *Engine) ParseCommands(input string) []extractor.Command {
	return e.extractor.ParseCommands(input)
}

// ParseCommandsDetailed parses selectors and preserves malformed-line
// diagnostics for non-interactive callers.
func (e *Engine) ParseCommandsDetailed(input string) extractor.CommandParseResult {
	return e.extractor.ParseCommandsDetailed(input)
}

// GenerateContext extracts requested source and builds the Schema B context.
func (e *Engine) GenerateContext(goal string, commands []extractor.Command) (string, []protocol.ExtractionMetadata, error) {
	extractions, err := e.extractor.Extract(commands)
	if err != nil {
		var extractionErr *extractor.ExtractionError
		if errors.As(err, &extractionErr) && extractionErr.CanProceed && len(extractions) > 0 {
			schema, metadata := e.formatter.GenerateSchemaB(e.Topology, extractions, goal)
			return schema, metadata, nil
		}
		return "", nil, err
	}
	schema, metadata := e.formatter.GenerateSchemaB(e.Topology, extractions, goal)
	return schema, metadata, nil
}

// GenerateContextDetailed extracts requested source and returns partial
// failures and safety exclusions separately so callers can warn and continue
// with the usable context.
func (e *Engine) GenerateContextDetailed(goal string, commands []extractor.Command) (string, []protocol.ExtractionMetadata, int, []string, []string, error) {
	extractions, err := e.extractor.Extract(commands)
	if err != nil {
		var extractionErr *extractor.ExtractionError
		if errors.As(err, &extractionErr) && extractionErr.CanProceed && len(extractions) > 0 {
			schema, metadata := e.formatter.GenerateSchemaB(e.Topology, extractions, goal)
			return schema, metadata, len(extractions), append([]string(nil), extractionErr.Failures...), append([]string(nil), extractionErr.Excluded...), nil
		}
		return "", nil, 0, nil, nil, err
	}
	schema, metadata := e.formatter.GenerateSchemaB(e.Topology, extractions, goal)
	return schema, metadata, len(extractions), nil, nil, nil
}

// ParseWritePlan extracts planned file operations from an AI response without
// mutating disk. Keep planning separate from applying so callers can preview,
// audit, and require explicit consent.
func (e *Engine) ParseWritePlan(input string) []writer.FileUpdate {
	return e.ParseWritePlanDetailed(input).Updates
}

// ParseWritePlanDetailed extracts planned file operations, preserves non-file
// text, and surfaces protocol validation errors.
func (e *Engine) ParseWritePlanDetailed(input string) writer.ParseResult {
	result := writer.ParseAIResponseDetailed(input)
	for _, path := range e.externalWriteTargets(input) {
		result.Errors = append(result.Errors, externalContextWriteError(path))
	}
	return e.rejectExternalContextUpdates(result)
}

// ApplyUpdate applies a single planned file operation relative to the project
// root.
func (e *Engine) ApplyUpdate(update writer.FileUpdate, mode writer.WhitespaceMode) error {
	if e.isExternalContextPath(update.Path) {
		return externalContextWriteError(update.Path)
	}
	return writer.WriteFile(e.Root, update, mode)
}

// ApplyWrite applies a single planned file update relative to the project root.
func (e *Engine) ApplyWrite(update writer.FileUpdate, mode writer.WhitespaceMode) error {
	return e.ApplyUpdate(update, mode)
}

func (e *Engine) resolveTaggedFiles(goal string) ([]protocol.TaggedFile, []string) {
	if e == nil {
		return nil, nil
	}

	refs, parseErrs := taggedfile.Parse(goal)
	warnings := make([]string, 0, len(parseErrs))
	for _, err := range parseErrs {
		warnings = append(warnings, err.Error())
	}

	externalRoots := e.ExternalRoots()
	paths := make([]protocol.TaggedFile, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		resolved, err := taggedfile.Resolve(e.Root, ref.Path, externalRoots)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		if _, ok := seen[resolved.AbsPath]; ok {
			continue
		}
		seen[resolved.AbsPath] = struct{}{}
		paths = append(paths, protocol.TaggedFile{
			Path:    resolved.Path,
			IsLocal: resolved.Source == taggedfile.SourceLocal,
		})
	}

	return paths, warnings
}

// ExternalRoots returns configured external context roots for tagged-file
// resolution.
func (e *Engine) ExternalRoots() []taggedfile.ExternalRoot {
	if e == nil || e.Topology == nil {
		return nil
	}
	roots := make([]taggedfile.ExternalRoot, 0, len(e.Topology.ExternalContext))
	for _, ctx := range e.Topology.ExternalContext {
		ctx := ctx // capture for closure
		roots = append(roots, taggedfile.ExternalRoot{
			Path:    ctx.Path,
			AbsPath: ctx.AbsPath,
			IsOmitted: func(relPath, absPath string) bool {
				return externalcontext.IsOmittedPath(ctx.AbsPath, absPath, relPath)
			},
		})
	}
	return roots
}

func (e *Engine) rejectExternalContextUpdates(result writer.ParseResult) writer.ParseResult {
	if e == nil || e.Topology == nil || len(e.Topology.ExternalContext) == 0 {
		return result
	}
	kept := result.Updates[:0]
	for _, update := range result.Updates {
		if e.isExternalContextPath(update.Path) {
			result.Errors = append(result.Errors, externalContextWriteError(update.Path))
			continue
		}
		kept = append(kept, update)
	}
	result.Updates = kept
	return result
}

func (e *Engine) isExternalContextPath(path string) bool {
	return e != nil && e.Topology != nil && externalcontext.ContainsPath(e.Root, e.Topology.ExternalContext, path)
}

func (e *Engine) externalWriteTargets(input string) []string {
	if e == nil || e.Topology == nil || len(e.Topology.ExternalContext) == 0 {
		return nil
	}
	var paths []string
	for _, rawLine := range strings.Split(input, "\n") {
		line := strings.TrimSpace(rawLine)
		path, ok := parseWriteTargetLine(line)
		if !ok {
			continue
		}
		if e.isExternalContextPath(path) {
			paths = append(paths, path)
		}
	}
	return paths
}

func parseWriteTargetLine(line string) (string, bool) {
	for _, prefix := range []string{"--- File: ", "--- Delete File: "} {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, " ---") {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), " ---"), true
		}
	}
	return "", false
}

func externalContextWriteError(path string) error {
	return fmt.Errorf("Cannot apply patch outside project root: %s\nExternal context paths are read-only.", path)
}
