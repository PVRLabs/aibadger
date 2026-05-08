package badger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/PVRLabs/aibadger/internal/clipboard"
)

// This file contains the headless input and confirmation helpers shared by
// interactive sessions and one-shot development steps.

// confirm asks the user for a y/n confirmation.
func confirm(r *bufio.Reader) bool {
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// readFrom reads multi-line input from the provided reader until a "DONE" sentinel is found.
func readFrom(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		text := scanner.Text()
		if strings.TrimSpace(text) == "DONE" {
			break
		}
		lines = append(lines, text)
	}
	return strings.Join(lines, "\n")
}

// readInputFile reads the entire content of a file, used primarily for dev-mode testing.
func readInputFile(w io.Writer, path string) string {
	if path == "" {
		return ""
	}
	content, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(w, "Error reading input file: %v\n", err)
		return ""
	}
	return string(content)
}

func readInput(w io.Writer, opts HeadlessOptions) string {
	if opts.InputPath != "" {
		return readInputFile(w, opts.InputPath)
	}
	return readFrom(opts.Stdin)
}

func copyIfConfirmed(w io.Writer, reader *bufio.Reader, text, successMsg string) error {
	if !confirm(reader) {
		return nil
	}
	if err := clipboard.Copy(text); err != nil {
		return err
	}
	fmt.Fprintf(w, "[✓] %s\n", successMsg)
	return nil
}
