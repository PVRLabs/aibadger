package main

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/PVRLabs/aibadger/internal/codemap"
)

func runMapCommand(args []string, stdout io.Writer) error {
	return runMapCommandWithGenerator(args, stdout, codemap.Generate)
}

func runMapCommandWithGenerator(args []string, stdout io.Writer, generate func(string) (string, error)) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := io.WriteString(stdout, mapHelp)
		return err
	}
	if len(args) > 1 || (len(args) == 1 && args[0] == "") {
		return fmt.Errorf("badger map accepts at most one directory path; usage: badger map [path] (see badger map --help)")
	}
	path := "."
	if len(args) == 1 {
		path = args[0]
	}
	if strings.HasPrefix(path, "-") {
		return fmt.Errorf("unknown badger map option %q; use ./ for a directory name beginning with '-'; see badger map --help", path)
	}
	path, err := generate(path)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Generated codebase map HTML report.\nUse the following command to open:\n%s\n", mapOpenCommand(path, runtime.GOOS))
	return err
}

// The Windows command is explicitly for PowerShell; POSIX commands quote all
// path characters literally, including apostrophes and shell substitutions.
func mapOpenCommand(path, goos string) string {
	if goos == "windows" {
		return "PowerShell:\nInvoke-Item -LiteralPath '" + strings.ReplaceAll(path, "'", "''") + "'"
	}
	quoted := "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'"
	if goos == "darwin" {
		return "open -- " + quoted
	}
	return "xdg-open " + quoted
}

const mapHelp = `Usage: badger map [path]

Generate a local, offline Codebase Map from one directory (no Git required).
Defaults to the current directory when path is omitted.
Write badger-codemap-<repo_name>.html in the caller's current directory and
print a success message and an OS-specific command to open the report.
Run the printed command to open the file directly from disk.
Unusual basenames are normalized for portable filenames. Successful generation
replaces an existing regular report; failures preserve it. No confirmation prompt.

Area represents raw nonblank physical source lines, including comments, not
complexity or recommended AI context size. Recognized source includes Markdown;
dependency/build/cache trees are skipped and symlinks are not followed.
The report embeds relative paths and aggregate metadata, not source contents.
No provider, upload, server, browser launch, or frontend runtime is required.

Hard limits: depth 50 below root, 20,000 directories including root,
100,000 recognized files, 50 MiB per source file, and 20 MiB complete HTML.
Invalid, unreadable, empty-source, or over-limit inputs fail without partial output.

Options:
  -h, --help  Print help without scanning or writing.
`
