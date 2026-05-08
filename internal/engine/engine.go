package engine

import (
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/scanner"
	"github.com/PVRLabs/aibadger/internal/writer"
)

// Engine coordinates Badger's core scan, extraction, formatting, and
// write-planning workflow. CLI and future integration layers should depend on
// this orchestration boundary instead of wiring lower-level packages directly.
type Engine struct {
	Root      string
	Topology  *model.ProjectTopology
	formatter *protocol.Formatter
	extractor *extractor.Extractor
}

// New scans the project root and creates an engine for follow-up workflow
// steps.
func New(root string) (*Engine, error) {
	if err := CheckDisabled(root); err != nil {
		return nil, err
	}

	s := scanner.NewScanner(root)
	topology, err := s.Scan()
	if err != nil {
		return nil, err
	}

	return FromTopology(root, topology), nil
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
	e.formatter.Instructions = instr
}

// GenerateMap builds Prompt 1: Topology.
func (e *Engine) GenerateMap(goal string) string {
	return e.formatter.GenerateSchemaA(e.Topology, goal)
}

// ParseCommands parses FILE/PREFIX/NEAR extraction commands.
func (e *Engine) ParseCommands(input string) []extractor.Command {
	return e.extractor.ParseCommands(input)
}

// GenerateContext extracts requested source and builds the Schema B context.
func (e *Engine) GenerateContext(goal string, commands []extractor.Command) (string, []protocol.ExtractionMetadata, error) {
	extractions, err := e.extractor.Extract(commands)
	if err != nil {
		return "", nil, err
	}
	schema, metadata := e.formatter.GenerateSchemaB(e.Topology, extractions, goal)
	return schema, metadata, nil
}

// ParseWritePlan extracts planned file operations from an AI response without
// mutating disk. Keep planning separate from applying so callers can preview,
// audit, and require explicit consent.
func (e *Engine) ParseWritePlan(input string) []writer.FileUpdate {
	return writer.ParseAIResponse(input)
}

// ParseWritePlanDetailed extracts planned file operations, preserves non-file
// text, and surfaces protocol validation errors.
func (e *Engine) ParseWritePlanDetailed(input string) writer.ParseResult {
	return writer.ParseAIResponseDetailed(input)
}

// ApplyUpdate applies a single planned file operation relative to the project
// root.
func (e *Engine) ApplyUpdate(update writer.FileUpdate, mode writer.WhitespaceMode) error {
	return writer.WriteFile(e.Root, update, mode)
}

// ApplyWrite applies a single planned file update relative to the project root.
func (e *Engine) ApplyWrite(update writer.FileUpdate, mode writer.WhitespaceMode) error {
	return e.ApplyUpdate(update, mode)
}
