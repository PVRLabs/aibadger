package codemap

import "strings"

// Options configures exclusions and optional report provenance. Exclusions are
// relative to the supplied source directory, independently of the caller's CWD.
type Options struct {
	ExcludePatterns []string
	NoReproCommand  bool
	Version         string
}

type reproduction struct {
	Patterns []string `json:"patterns"`
	Shell    string   `json:"shell"`
	Command  string   `json:"command"`
}

func reproductionFor(options Options, goos string) *reproduction {
	if options.NoReproCommand || len(options.ExcludePatterns) == 0 {
		return nil
	}
	shell := "POSIX"
	if goos == "windows" {
		shell = "PowerShell"
	}
	var command strings.Builder
	command.WriteString("badger map .")
	for _, pattern := range options.ExcludePatterns {
		command.WriteString(" --exclude=")
		if goos == "windows" {
			command.WriteString(quotePowerShell(pattern))
		} else {
			command.WriteString("'" + strings.ReplaceAll(pattern, "'", "'\"'\"'") + "'")
		}
	}
	return &reproduction{Patterns: append([]string(nil), options.ExcludePatterns...), Shell: shell, Command: command.String()}
}

// PowerShell treats U+2018 through U+201B as single-quote delimiters too.
// Double each delimiter without normalizing it, preserving the original text.
// See PowerShell's CharExtensions.IsSingleQuote and about_Quoting_Rules.
var powerShellQuoteEscapes = strings.NewReplacer(
	"'", "''", "‘", "‘‘", "’", "’’", "‚", "‚‚", "‛", "‛‛",
)

func quotePowerShell(value string) string {
	return "'" + powerShellQuoteEscapes.Replace(value) + "'"
}
