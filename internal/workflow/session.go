package workflow

// This file owns shared workflow operations used by both TUI and headless
// modes while leaving interaction and output behavior in those callers.

import (
	"fmt"
	"strings"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/writer"
)

type EngineOptions struct {
	MaxContextFileBytes  int
	MaxTotalContextBytes int
	MaxPackages          int
	Focus                protocol.Focus
	SchemaAConstraint    string
	SchemaBConstraint    string
}

func ConfigureEngine(eng *engine.Engine, opts EngineOptions) {
	eng.SetMaxContextFileBytes(opts.MaxContextFileBytes)
	eng.SetMaxTotalContextBytes(opts.MaxTotalContextBytes)
	if opts.MaxPackages > 0 {
		eng.SetMaxPackages(opts.MaxPackages)
	}
	eng.SetFocus(opts.Focus)
	if opts.SchemaAConstraint == "" && opts.SchemaBConstraint == "" {
		return
	}

	instr := protocol.DefaultInstructions
	if opts.SchemaAConstraint != "" {
		instr.SchemaAConstraint = opts.SchemaAConstraint
	}
	if opts.SchemaBConstraint != "" {
		instr.SchemaBConstraint = opts.SchemaBConstraint
	}
	eng.SetPromptInstructions(instr)
}

type Session struct {
	Engine         *engine.Engine
	WhitespaceMode writer.WhitespaceMode
}

type WriteError struct {
	Path string
	Err  error
}

type ExtractionCommandResult struct {
	Commands []extractor.Command
	Count    int
	Empty    bool
}

type FinalResponseResult struct {
	Parse      writer.ParseResult
	HasErrors  bool
	HasUpdates bool
	HasNotes   bool
	TextBytes  int
}

func (e WriteError) Error() string {
	return fmt.Sprintf("%s: %v", e.Path, e.Err)
}

func (e WriteError) Unwrap() error {
	return e.Err
}

func NewSession(eng *engine.Engine, mode writer.WhitespaceMode) *Session {
	return &Session{Engine: eng, WhitespaceMode: mode}
}

func (s *Session) GenerateMap(goal string) string {
	return s.Engine.GenerateMap(goal)
}

func (s *Session) GenerateMapDetailed(goal string) (string, []string) {
	return s.Engine.GenerateMapDetailed(goal)
}

func (s *Session) ParseExtractionCommands(input string) []extractor.Command {
	return s.Engine.ParseCommands(input)
}

func (s *Session) ParseExtractionInput(input string) ExtractionCommandResult {
	commands := s.ParseExtractionCommands(input)
	return ExtractionCommandResult{
		Commands: commands,
		Count:    len(commands),
		Empty:    len(commands) == 0,
	}
}

func (s *Session) GenerateContext(goal string, commands []extractor.Command) (string, []protocol.ExtractionMetadata, error) {
	return s.Engine.GenerateContext(goal, commands)
}

func (s *Session) GenerateContextDetailed(goal string, commands []extractor.Command) (string, []protocol.ExtractionMetadata, int, []string, []string, error) {
	return s.Engine.GenerateContextDetailed(goal, commands)
}

func (s *Session) GenerateContextFromInput(goal, input string) ([]extractor.Command, string, []protocol.ExtractionMetadata, error) {
	result := s.ParseExtractionInput(input)
	schema, metadata, err := s.GenerateContext(goal, result.Commands)
	return result.Commands, schema, metadata, err
}

func (s *Session) GenerateContextDetailedFromInput(goal, input string) ([]extractor.Command, string, []protocol.ExtractionMetadata, int, []string, []string, error) {
	result := s.ParseExtractionInput(input)
	schema, metadata, extractedCount, failedCommands, safetyExclusions, err := s.GenerateContextDetailed(goal, result.Commands)
	return result.Commands, schema, metadata, extractedCount, failedCommands, safetyExclusions, err
}

func (s *Session) ParseWritePlan(input string) writer.ParseResult {
	return s.Engine.ParseWritePlanDetailed(input)
}

func (s *Session) ParseFinalResponse(input string) FinalResponseResult {
	result := s.ParseWritePlan(input)
	text := strings.TrimSpace(result.Text)
	return FinalResponseResult{
		Parse:      result,
		HasErrors:  len(result.Errors) > 0,
		HasUpdates: len(result.Updates) > 0,
		HasNotes:   text != "",
		TextBytes:  len(text),
	}
}

func (s *Session) ApplyWrite(update writer.FileUpdate) error {
	return s.Engine.ApplyUpdate(update, s.WhitespaceMode)
}

func (s *Session) ApplyWrites(updates []writer.FileUpdate) ([]writer.FileUpdate, []error) {
	applied := make([]writer.FileUpdate, 0, len(updates))
	var errs []error
	for _, update := range updates {
		if err := s.ApplyWrite(update); err != nil {
			errs = append(errs, WriteError{Path: update.Path, Err: err})
			continue
		}
		applied = append(applied, update)
	}
	return applied, errs
}
