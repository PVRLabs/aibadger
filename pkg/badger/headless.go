package badger

// This file owns the top-level headless runner: option defaults, disabled
// checks, scan orchestration, and routing into stable development steps.

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

type HeadlessOptions struct {
	Step      string
	InputPath string
	Focus     protocol.Focus
	// Goal is optional. If empty, RunHeadless uses cfg.Startup.Goal. Empty
	// preserves the historical headless output used by certification fixtures.
	Goal             string
	TruncateTopology bool
	Stdin            io.Reader
	Stdout           io.Writer
}

type scanOutputMode int

const (
	scanOutputVerbose scanOutputMode = iota
	scanOutputStable
	scanOutputSilent
)

// RunHeadless exposes the deterministic development automation path used by
// blackbox validation. It preserves the command-line output contract.
func RunHeadless(cfg Config, opts HeadlessOptions) error {
	cfg = cfg.withDefaults()
	opts.Focus = protocol.NormalizeFocus(cfg.Focus)
	stdin := opts.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}
	opts.Stdin = stdin
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	if opts.Goal == "" {
		opts.Goal = cfg.Startup.Goal
	}

	if err := engine.CheckDisabledAndExit(cfg.Root, stdout); err != nil {
		return nil
	}

	showHeadlessWrapper := opts.Step == "" || opts.Step == "scan"
	if showHeadlessWrapper {
		fmt.Fprintf(stdout, "%s — Headless\n", Name)
		fmt.Fprintln(stdout, "----------------------")
	}

	scanOutput := scanOutputStable
	if !showHeadlessWrapper {
		scanOutput = scanOutputSilent
	}
	eng, err := scanProject(stdout, cfg.Root, scanOutput, cfg.MaxFilesPerDirectory)
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}
	workflow.ConfigureEngine(eng, headlessEngineOptions(cfg, opts))
	session := workflow.NewSession(eng, writer.WhitespaceMode(cfg.WhitespaceMode))
	if opts.Step == "scan" {
		return nil
	}
	if opts.Step != "" {
		runHeadlessStep(stdout, session, opts)
		return nil
	}

	reader := bufio.NewReader(stdin)
	runSession(stdout, reader, session, cfg, opts)
	return nil
}

func headlessEngineOptions(cfg Config, opts HeadlessOptions) workflow.EngineOptions {
	engOpts := workflow.EngineOptions{
		MaxContextFileBytes:  cfg.MaxContextFileBytes,
		MaxTotalContextBytes: cfg.MaxTotalContextBytes,
		SchemaAConstraint:    cfg.SchemaAConstraint,
		SchemaBConstraint:    cfg.SchemaBConstraint,
		Focus:                opts.Focus,
	}
	if opts.TruncateTopology {
		engOpts.MaxPackages = cfg.TruncatedMaxPackages
	}
	return engOpts
}

func scanProject(w io.Writer, root string, output scanOutputMode, maxFilesPerDir int) (*engine.Engine, error) {
	if output != scanOutputSilent {
		fmt.Fprint(w, "Scanning project... ")
	}
	eng, err := engine.New(root, maxFilesPerDir)
	if err != nil {
		return nil, err
	}
	if output == scanOutputSilent {
		return eng, nil
	}
	topology := eng.Topology
	if output == scanOutputStable {
		fmt.Fprint(w, "Done\n\n")
	} else {
		fmt.Fprintf(w, "Done in %v\n\n", topology.ScanTime)
	}
	fmt.Fprintf(w, "Project: %s", topology.Name)
	if len(topology.Languages) > 0 {
		fmt.Fprintf(w, " (%s)", strings.Join(topology.Languages, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Found %d modules\n\n", len(topology.Modules))
	return eng, nil
}
