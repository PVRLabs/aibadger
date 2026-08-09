package badger

import (
	"fmt"
	"io"
	"strings"

	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
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

func printAPIResponsePlan(w io.Writer, response string, result writer.ParseResult) {
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

func printAPIExtractionPlan(w io.Writer, input string, commands []extractor.Command) {
	fmt.Fprintln(w, "[EXTRACTION PLAN]")
	fmt.Fprintf(w, "commands=%d\n", len(commands))
	for _, command := range commands {
		fmt.Fprintf(w, "%s path=%s\n", command.Type, extractionCommandReference(command))
	}
	fmt.Fprintf(w, "plaintext_input_bytes=%d\n", len(strings.TrimSpace(input)))
}

func extractionCommandReference(command extractor.Command) string {
	if command.Pattern == "" {
		return command.Path
	}
	return command.Path + "#" + command.Pattern
}
