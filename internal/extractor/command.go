package extractor

// This file owns the input boundary for FILE/PREFIX/NEAR command text.

import (
	"bufio"
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

// ParseCommands parses the AI's response into a list of Commands.
func (e *Extractor) ParseCommands(input string) []Command {
	var commands []Command
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		line := scanner.Text()
		if shouldRecoverEmbeddedFiles(line) {
			commands = append(commands, parseEmbeddedFileCommands(line)...)
			continue
		}
		if cmd, ok := parseCommandLine(line); ok {
			commands = append(commands, cmd)
			continue
		}
		commands = append(commands, parseEmbeddedFileCommands(line)...)
	}
	return commands
}

func parseCommandLine(line string) (Command, bool) {
	line = strings.TrimSpace(line)
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
	if cmd.Path == "" {
		return Command{}, false
	}
	return cmd, true
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
