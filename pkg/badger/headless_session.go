package badger

// This file owns the interactive headless BYOL loop that connects goal entry,
// prompt generation, extraction, context delivery, and final response handling.

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/PVRLabs/aibadger/internal/workflow"
)

// runSession drives the interactive headless BYOL loop from goal entry through
// final response handling.
func runSession(w io.Writer, reader *bufio.Reader, session *workflow.Session, cfg Config, opts HeadlessOptions) {
	for {
		goal := readGoal(w, reader, cfg, opts)
		if goal == cfg.ExitCommand {
			fmt.Fprintln(w, "Goodbye! 🦡")
			return
		}
		if goal == "" && opts.Step == "" {
			continue
		}
		if opts.Step == "goal" {
			fmt.Fprintf(w, "Dev goal: %s\n", goal)
			return
		}
		if runGoalFlow(w, reader, session, goal, cfg, opts) {
			return
		}
	}
}

func readGoal(w io.Writer, reader *bufio.Reader, cfg Config, opts HeadlessOptions) string {
	if opts.Goal != "" {
		return opts.Goal
	}
	if opts.Step == "goal" {
		return readInputFile(w, opts.InputPath)
	}

	fmt.Fprintf(w, "\nStep 1: What is your coding goal? (type %s to quit)\n", cfg.ExitCommand)
	fmt.Fprint(w, "> ")
	goal, _ := reader.ReadString('\n')
	return strings.TrimSpace(goal)
}

func runGoalFlow(w io.Writer, reader *bufio.Reader, session *workflow.Session, goal string, cfg Config, opts HeadlessOptions) bool {
	schemaA := session.GenerateMap(goal)
	if handleProjectMap(w, reader, schemaA, opts) {
		return true
	}

	commands, effectiveGoal, stop := handleExtractionCommands(w, goal, session, opts)
	if stop {
		return true
	}
	if len(commands) == 0 {
		fmt.Fprintln(w, "No extraction commands provided. Skipping to next task.")
		return false
	}

	schemaB, metadata, err := session.GenerateContext(effectiveGoal, commands)
	if err != nil {
		fmt.Fprintf(w, "Extraction error: %v\n", err)
		return true
	}

	printExtractionMetadata(w, metadata)

	if handleContextCopy(w, reader, schemaB, opts) {
		return true
	}

	return handleFinalResponse(w, reader, session, opts)
}
