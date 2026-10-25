package extractor

// This file owns the input boundary for FILE/PREFIX/NEAR command text.

import (
	"bufio"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Command represents a single extraction command.
type Command struct {
	Type    string // FILE, PREFIX, NEAR
	Path    string
	Pattern string
}

// CommandParseResult preserves valid literal selectors while reporting
// malformed non-empty input lines separately.
type CommandParseResult struct {
	Commands []Command
	Failures []string
}

// ParseCommands parses the AI's response into a list of Commands.
func (e *Extractor) ParseCommands(input string) []Command {
	return e.parseCommands(input, false).Commands
}

// ParseCommandsDetailed parses selectors and reports malformed input lines
// without discarding selectors that remain usable.
func (e *Extractor) ParseCommandsDetailed(input string) CommandParseResult {
	return e.parseCommands(input, true)
}

// ParseStrictCommandsDetailed accepts only one complete selector per non-empty
// line. Review continuation uses this boundary so ordinary findings are never
// mistaken for extraction requests merely because they mention FILE: in prose.
func (e *Extractor) ParseStrictCommandsDetailed(input string) CommandParseResult {
	input = unwrapSelectorFence(input)
	var result CommandParseResult
	scanner := bufio.NewScanner(strings.NewReader(input))
	// The caller already owns input as a string, so allow one token to span the
	// complete response. Otherwise Scanner's 64 KiB default can stop after an
	// earlier valid selector and silently hide oversized mixed prose.
	scanner.Buffer(make([]byte, 64*1024), len(input)+1)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		cmd, ok := parseCommandLine(line)
		if !ok {
			result.Failures = append(result.Failures, fmt.Sprintf("line %d: not a complete FILE, PREFIX, or NEAR selector", lineNumber))
			continue
		}
		if (cmd.Type == "PREFIX" || cmd.Type == "NEAR") && cmd.Pattern == "" {
			result.Failures = append(result.Failures, fmt.Sprintf("line %d: %s requires path#pattern", lineNumber, cmd.Type))
			continue
		}
		result.Commands = append(result.Commands, e.unwrapFilePath(cmd))
	}
	if err := scanner.Err(); err != nil {
		result.Failures = append(result.Failures, fmt.Sprintf("reading selector response: %v", err))
	}
	return result
}

func (e *Extractor) parseCommands(input string, reportMalformed bool) CommandParseResult {
	input = unwrapSelectorFence(input)
	var result CommandParseResult
	scanner := bufio.NewScanner(strings.NewReader(input))
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := unwrapSelectorLine(scanner.Text())
		if strings.TrimSpace(line) == "" {
			continue
		}
		if shouldRecoverEmbeddedFiles(line) {
			result.Commands = append(result.Commands, parseEmbeddedFileCommands(line)...)
			continue
		}
		if cmd, ok := parseCommandLine(line); ok {
			if reportMalformed && (cmd.Type == "PREFIX" || cmd.Type == "NEAR") && cmd.Pattern == "" {
				result.Failures = append(result.Failures, fmt.Sprintf("line %d: %s requires path#pattern", lineNumber, cmd.Type))
				continue
			}
			result.Commands = append(result.Commands, e.unwrapFilePath(cmd))
			continue
		}
		embedded := parseEmbeddedFileCommands(line)
		if len(embedded) > 0 {
			result.Commands = append(result.Commands, embedded...)
			continue
		}
		if reportMalformed {
			result.Failures = append(result.Failures, fmt.Sprintf("line %d: invalid or unsupported selector: %s", lineNumber, strings.TrimSpace(line)))
		}
	}
	return result
}

func parseCommandLine(line string) (Command, bool) {
	line = unwrapSelectorLine(line)
	if line == "" {
		return Command{}, false
	}

	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 2 {
		return Command{}, false
	}

	cmdType := strings.ToUpper(parts[0])
	if !isSupportedCommandType(cmdType) {
		return Command{}, false
	}

	pathAndPattern := strings.SplitN(parts[1], "#", 2)
	cmd := Command{
		Type: cmdType,
		Path: strings.TrimSpace(pathAndPattern[0]),
	}
	if len(pathAndPattern) > 1 {
		cmd.Pattern = strings.TrimSpace(pathAndPattern[1])
	}
	if cmd.Type == "FILE" {
		cmd.Path = normalizeFileSelectorPath(cmd.Path)
	}
	if cmd.Path == "" {
		return Command{}, false
	}
	return cmd, true
}

// Strip presentation prefixes only; prose following a prefix is still rejected
// by the selector parser. Patterns are never unescaped or rewritten.
func unwrapSelectorLine(line string) string {
	line = strings.TrimSpace(line)
	for {
		previous := line
		if strings.HasPrefix(line, "> ") {
			line = strings.TrimSpace(line[2:])
		}
		if len(line) > 2 && strings.ContainsAny(line[:1], "-*+") && (line[1] == ' ' || line[1] == '\t') {
			line = strings.TrimSpace(line[2:])
		}
		i := 0
		for i < len(line) && line[i] >= '0' && line[i] <= '9' {
			i++
		}
		if i > 0 && i+1 < len(line) && (line[i] == '.' || line[i] == ')') && (line[i+1] == ' ' || line[i+1] == '\t') {
			line = strings.TrimSpace(line[i+2:])
		}
		if line == previous {
			break
		}
	}
	return unwrapBackticks(line)
}

func unwrapBackticks(text string) string {
	n := len(text) - len(strings.TrimLeft(text, "`"))
	if n > 0 && len(text) > 2*n && strings.HasSuffix(text, strings.Repeat("`", n)) {
		inner := text[n : len(text)-n]
		if !strings.HasPrefix(inner, "`") && !strings.HasSuffix(inner, "`") {
			return strings.TrimSpace(inner)
		}
	}
	return text
}

// Only a complete selector-only fenced response is treated as presentation.
// Keep line numbers intact so malformed responses retain useful diagnostics.
func unwrapSelectorFence(input string) string {
	lines := strings.Split(input, "\n")
	first, last := 0, len(lines)-1
	for first <= last && strings.TrimSpace(lines[first]) == "" {
		first++
	}
	for last > first && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if first >= last {
		return input
	}
	opening := strings.TrimSpace(lines[first])
	n := len(opening) - len(strings.TrimLeft(opening, "`"))
	if n < 3 || strings.TrimSpace(lines[last]) != strings.Repeat("`", n) || strings.Contains(opening[n:], "`") {
		return input
	}
	count := 0
	for _, line := range lines[first+1 : last] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		cmd, ok := parseCommandLine(line)
		if !ok || (cmd.Type != "FILE" && cmd.Pattern == "") {
			return input
		}
		count++
	}
	if count == 0 {
		return input
	}
	lines[first], lines[last] = "", ""
	return strings.Join(lines, "\n")
}

// Preserve literal backtick filenames. Only use the unwrapped path when it
// resolves to an existing repository file and the literal path does not.
func (e *Extractor) unwrapFilePath(cmd Command) Command {
	if cmd.Type == "FILE" {
		path := unwrapBackticks(cmd.Path)
		if path != cmd.Path && e.resolveCommandPath(cmd.Path) == "" && e.resolveCommandPath(path) != "" {
			cmd.Path = path
		}
	}
	return cmd
}

// AI responses sometimes use a trailing backslash as a Markdown hard line break.
// Remove only one such suffix; internal path separators remain literal.
func normalizeFileSelectorPath(path string) string {
	return strings.TrimSpace(strings.TrimSuffix(path, `\`))
}

func isSupportedCommandType(cmdType string) bool {
	return cmdType == "FILE" || cmdType == "PREFIX" || cmdType == "NEAR"
}

func shouldRecoverEmbeddedFiles(line string) bool {
	trimmed := strings.TrimSpace(line)
	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) < 2 {
		return false
	}

	cmdType := strings.ToUpper(parts[0])
	if cmdType == "PREFIX" || cmdType == "NEAR" {
		return false
	}
	return len(fileTokenIndexes(line)) > 1
}

func parseEmbeddedFileCommands(line string) []Command {
	indexes := fileTokenIndexes(line)
	commands := make([]Command, 0, len(indexes))
	for i, idx := range indexes {
		start := idx + len("FILE:")
		end := len(line)
		if i+1 < len(indexes) {
			end = indexes[i+1]
		}

		path := strings.TrimSpace(line[start:end])
		path = strings.TrimRight(path, " \t\r\n.,;:)]}")
		path = normalizeFileSelectorPath(path)
		if path == "" {
			continue
		}
		commands = append(commands, Command{
			Type: "FILE",
			Path: path,
		})
	}
	return commands
}

func fileTokenIndexes(line string) []int {
	upper := strings.ToUpper(line)
	var indexes []int
	for searchFrom := 0; searchFrom < len(upper); {
		idx := strings.Index(upper[searchFrom:], "FILE:")
		if idx < 0 {
			break
		}
		idx += searchFrom
		if hasFileTokenBoundary(line, idx) {
			indexes = append(indexes, idx)
		}
		searchFrom = idx + len("FILE:")
	}
	return indexes
}

func hasFileTokenBoundary(line string, idx int) bool {
	if idx == 0 {
		return true
	}
	prev, _ := utf8.DecodeLastRuneInString(line[:idx])
	return !(unicode.IsLetter(prev) || unicode.IsDigit(prev) || prev == '_')
}
