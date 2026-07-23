package badger

// This file owns the non-interactive API runner used by external tools and
// certification. It deliberately does not enter the headless session: callers
// provide all input up front and own both output streams.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

// APIOptions describes one non-interactive API invocation. InputPath is read
// exactly once and is never modified. Topology, prompt, and extract are the
// stable text-first operations; the other current operations support
// certification.
type APIOptions struct {
	Operation    string
	InputPath    string
	GoalFilePath string
	Focus        protocol.Focus
	Stdout       io.Writer
	Stderr       io.Writer
}

// RunAPI executes a non-interactive API operation. It never reads stdin,
// changes settings, asks for confirmation, or accesses the clipboard.
func RunAPI(cfg Config, opts APIOptions) error {
	if err := validateAPIOperation(opts.Operation, opts.InputPath, opts.GoalFilePath, opts.Focus); err != nil {
		return err
	}

	root, err := normalizeAPIRoot(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = root
	cfg = cfg.withDefaults()

	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	input, err := readAPIInput(opts.InputPath)
	if err != nil {
		return err
	}
	goal, err := readAPIFile(opts.GoalFilePath, "api goal file")
	if err != nil {
		return err
	}
	if opts.Operation == "prompt" && strings.TrimSpace(input) == "" {
		return fmt.Errorf("api prompt input file is empty")
	}
	if opts.Operation == "extract" {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("api extract input file is empty")
		}
		if strings.TrimSpace(goal) == "" {
			return fmt.Errorf("api extract goal file is empty")
		}
	}
	if err := engine.CheckDisabled(cfg.Root); err != nil {
		if errors.Is(err, engine.ErrProjectDisabled) {
			fmt.Fprintln(stderr, "project explicitly disabled via .badger-disable")
		}
		return err
	}

	scanOutput := scanOutputSilent
	if opts.Operation == "scan" {
		fmt.Fprintf(stdout, "%s — Headless\n", Name)
		fmt.Fprintln(stdout, "----------------------")
		scanOutput = scanOutputStable
	}
	eng, err := scanProject(stdout, cfg.Root, scanOutput, cfg.MaxFilesPerDirectory)
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}
	if opts.Operation == "scan" {
		return nil
	}

	workflow.ConfigureEngine(eng, headlessEngineOptions(cfg, HeadlessOptions{Focus: opts.Focus}))
	session := workflow.NewSession(eng, writer.WhitespaceMode(cfg.WhitespaceMode))
	switch opts.Operation {
	case "topology":
		fmt.Fprint(stdout, eng.GenerateTopology())
	case "prompt":
		schemaA, warnings := session.GenerateMapDetailed(input)
		printTaggedFileWarnings(stderr, warnings)
		fmt.Fprint(stdout, schemaA)
	case "extract":
		parsed := session.ParseExtractionInputDetailed(input)
		if parsed.Empty {
			printExtractionWarnings(stderr, 0, parsed.Failures, nil)
			return fmt.Errorf("no valid extraction selectors")
		}
		schemaB, metadata, extractedCount, failedCommands, safetyExclusions, err := session.GenerateContextDetailed(goal, parsed.Commands)
		if err != nil {
			return err
		}
		failedCommands = append(parsed.Failures, failedCommands...)
		printExtractionWarnings(stderr, extractedCount, failedCommands, safetyExclusions)
		printExtractionMetadata(stderr, metadata)
		fmt.Fprint(stdout, schemaB)
	case "goal":
		fmt.Fprintf(stdout, "Dev goal: %s\n", input)
	case "extraction":
		printHeadlessExtractionPlan(stdout, input, session.ParseExtractionInput(input).Commands)
	case "write-plan":
		printHeadlessResponsePlan(stdout, input, session.ParseWritePlan(input))
	}
	return nil
}

func validateAPIOperation(operation, inputPath, goalFilePath string, focus protocol.Focus) error {
	switch operation {
	case "scan", "topology":
		if inputPath != "" {
			return fmt.Errorf("api %s does not accept --input", operation)
		}
		if focus != "" {
			return fmt.Errorf("api %s does not accept --focus", operation)
		}
		if goalFilePath != "" {
			return fmt.Errorf("api %s does not accept --goal-file", operation)
		}
	case "prompt":
		if inputPath == "" {
			return fmt.Errorf("api prompt requires --input <file>")
		}
		if focus != protocol.FocusCode && focus != protocol.FocusDesign {
			return fmt.Errorf("api prompt requires --focus <code|design>")
		}
		if goalFilePath != "" {
			return fmt.Errorf("api prompt does not accept --goal-file")
		}
	case "extract":
		if inputPath == "" {
			return fmt.Errorf("api extract requires --input <file>")
		}
		if goalFilePath == "" {
			return fmt.Errorf("api extract requires --goal-file <file>")
		}
		if focus != "" {
			return fmt.Errorf("api extract does not accept --focus")
		}
	case "goal", "extraction", "write-plan":
		if inputPath == "" {
			return fmt.Errorf("api %s requires --input <file>", operation)
		}
		if focus != "" {
			return fmt.Errorf("api %s does not accept --focus", operation)
		}
		if goalFilePath != "" {
			return fmt.Errorf("api %s does not accept --goal-file", operation)
		}
	default:
		return fmt.Errorf("unknown api operation: %s", operation)
	}
	return nil
}

func normalizeAPIRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("api operation requires --root <project>")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("normalizing api root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("validating api root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validating api root: not a directory: %s", root)
	}
	return absRoot, nil
}

func readAPIInput(path string) (string, error) {
	return readAPIFile(path, "api input file")
}

func readAPIFile(path, label string) (string, error) {
	if path == "" {
		return "", nil
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	if !utf8.Valid(input) {
		return "", fmt.Errorf("reading %s: invalid UTF-8: %s", label, path)
	}
	return string(input), nil
}
