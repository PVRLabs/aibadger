package extractor

// This file owns the input boundary for FILE/PREFIX/NEAR command text.

import (
	"bufio"
	"strings"
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
		if cmd, ok := parseCommandLine(scanner.Text()); ok {
			commands = append(commands, cmd)
		}
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
