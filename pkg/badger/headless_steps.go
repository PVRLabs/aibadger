package badger

// This file owns one-shot development headless steps while preserving the
// stable command output contract used by automation.

import (
	"fmt"
	"io"

	"github.com/PVRLabs/aibadger/internal/workflow"
)

// runHeadlessStep routes one-shot development headless steps while preserving
// the stable command output contract used by automation.
func runHeadlessStep(w io.Writer, session *workflow.Session, opts HeadlessOptions) {
	switch {
	case opts.Step == "goal":
		fmt.Fprintf(w, "Dev goal: %s\n", readInput(w, opts))
	case isTopologyStep(opts.Step):
		fmt.Fprintln(w, session.GenerateMap(readInput(w, opts)))
	case opts.Step == "extraction":
		handleExtractionCommands(w, opts.Goal, session, opts)
	case isCodeContextStep(opts.Step):
		_, schemaB, metadata, err := session.GenerateContextFromInput(opts.Goal, readInput(w, opts))
		if err != nil {
			fmt.Fprintf(w, "Extraction error: %v\n", err)
			return
		}
		printExtractionMetadata(w, metadata)
		fmt.Fprintln(w, schemaB)
	case isWritePlanStep(opts.Step):
		response := readInput(w, opts)
		printHeadlessResponsePlan(w, response, session.ParseWritePlan(response))
	default:
		fmt.Fprintf(w, "Unknown headless step: %s\n", opts.Step)
	}
}

func isTopologyStep(step string) bool {
	return step == "topology" || step == "map" || step == "prompt1"
}

func isCodeContextStep(step string) bool {
	return step == "context" || step == "prompt2"
}

func isWritePlanStep(step string) bool {
	return step == "write-plan" || step == "write"
}
