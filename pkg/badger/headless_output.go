package badger

// This file owns stable headless output formatting for topology, extraction,
// context copy, and write-plan summaries.

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

func printExtractionMetadata(w io.Writer, metadata []protocol.ExtractionMetadata) {
	for _, meta := range metadata {
		if meta.Dropped {
			fmt.Fprintf(w, "  [!] %s (DROPPED - EXCEEDS TOTAL LIMIT)\n", meta.Path)
		} else if meta.Truncated {
			fmt.Fprintf(w, "  [!] %s (TRUNCATED)\n", meta.Path)
		}
	}
}

func printExtractionWarnings(w io.Writer, extractedCount int, failedCommands, safetyExclusions []string) {
	if len(failedCommands) == 0 && len(safetyExclusions) == 0 {
		return
	}
	fileLabel := "file"
	if extractedCount != 1 {
		fileLabel = "files"
	}
	fmt.Fprintf(w, "\n[!] Extracted %d %s with warnings.\n", extractedCount, fileLabel)
	if len(failedCommands) > 0 {
		fmt.Fprintln(w, "Failed requests:")
		for _, failure := range failedCommands {
			fmt.Fprintf(w, "  - %s\n", failure)
		}
	}
	if len(safetyExclusions) > 0 {
		fmt.Fprintln(w, "Excluded by Prompt 2 safety rules:")
		for _, exclusion := range safetyExclusions {
			fmt.Fprintf(w, "  - %s\n", exclusion)
		}
	}
}

func printTaggedFileWarnings(w io.Writer, warnings []string) {
	if len(warnings) == 0 {
		return
	}
	fmt.Fprintln(w, "\n[!] Tagged file warnings:")
	for _, warning := range warnings {
		fmt.Fprintf(w, "  - %s\n", warning)
	}
}

func handleProjectMap(w io.Writer, reader *bufio.Reader, schemaA string, opts HeadlessOptions) bool {
	if isTopologyStep(opts.Step) {
		fmt.Fprintln(w, schemaA)
		return true
	}

	fmt.Fprintln(w, "\n--- PROMPT 1: TOPOLOGY ---")
	fmt.Fprintln(w, schemaA)
	fmt.Fprintln(w, "---------------------------")
	fmt.Fprintf(w, "\nReady to copy %s to clipboard.\n", workflow.TopologyPromptKind)
	fmt.Fprint(w, "Copy? (y/N): ")

	if opts.InputPath != "" {
		return false
	}

	if err := copyIfConfirmed(w, reader, schemaA, workflow.TopologyPromptKind+" copied!"); err != nil {
		fmt.Fprintf(w, "Clipboard error: %v.\nFor instructions on installing a clipboard tool visit %s.\n%s is printed above.\n", err, clipboard.DocsURL, workflow.TopologyPromptKind)
	}
	return false
}

func handleExtractionCommands(w io.Writer, goal string, session *workflow.Session, opts HeadlessOptions) ([]extractor.Command, string, bool) {
	if opts.InputPath == "" {
		fmt.Fprintln(w, "\nNext steps:")
		fmt.Fprintf(w, "1. Paste %s into your AI chat.\n", workflow.TopologyPromptKind)
		fmt.Fprintln(w, "2. Copy the AI's extraction commands (FILE/PREFIX/NEAR).")
		fmt.Fprintln(w, "3. Paste them below and type 'DONE' on a new line to finish.")
		fmt.Fprint(w, "\n> ")
	}
	pasteInput := readInput(w, opts)

	result := session.ParseExtractionInput(pasteInput)
	fmt.Fprintf(w, "\n[✓] Parsed %d commands.\n", result.Count)

	if opts.Step == "extraction" {
		for _, c := range result.Commands {
			fmt.Fprintf(w, "  cmd: %s %s\n", c.Type, c.Path)
		}
		return nil, goal, true
	}

	return result.Commands, goal, false
}

func handleContextCopy(w io.Writer, reader *bufio.Reader, schemaB string, opts HeadlessOptions) bool {
	if isCodeContextStep(opts.Step) {
		fmt.Fprintln(w, schemaB)
		return true
	}

	fmt.Fprintf(w, "\nReady to copy %s to clipboard.\n", workflow.CodeContextPromptKind)
	fmt.Fprint(w, "Copy? (y/N): ")
	if confirm(reader) {
		if err := clipboard.Copy(schemaB); err != nil {
			fmt.Fprintf(w, "Clipboard error: %v.\nFor instructions on installing a clipboard tool visit %s.\nPrinting %s instead:\n\n%s\n", err, clipboard.DocsURL, workflow.CodeContextPromptKind, schemaB)
		} else {
			fmt.Fprintf(w, "[✓] %s copied!\n", workflow.CodeContextPromptKind)
		}
	}
	return false
}

func printHeadlessResponsePlan(w io.Writer, response string, result writer.ParseResult) {
	if len(result.Updates) == 0 {
		fmt.Fprintln(w, "\n[TEXT RESPONSE]")
		fmt.Fprintln(w, "updates=0")
		fmt.Fprintf(w, "plaintext_response_bytes=%d\n", len(strings.TrimSpace(response)))
		for _, err := range result.Errors {
			fmt.Fprintf(w, "parse_error=%v\n", err)
		}
		return
	}

	fmt.Fprintln(w, "\n[WRITE PLAN]")
	fmt.Fprintf(w, "updates=%d\n", len(result.Updates))
	fmt.Fprintf(w, "plaintext_response_bytes=%d\n", len(result.Text))
	for _, up := range result.Updates {
		fmt.Fprintf(w, "%s path=%s bytes=%d\n", strings.ToUpper(string(up.Kind)), up.Path, len(up.Content))
	}
	for _, err := range result.Errors {
		fmt.Fprintf(w, "parse_error=%v\n", err)
	}
}
