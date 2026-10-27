package main

import (
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/PVRLabs/aibadger/internal/codemap"
	"github.com/PVRLabs/aibadger/internal/pathmatch"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

func runMapCommand(args []string, stdout io.Writer) error {
	return runMapCommandWithGenerator(args, stdout, codemap.GenerateWithOptions)
}

func runMapCommandWithGenerator(args []string, stdout io.Writer, generate func(string, codemap.Options) (string, error)) error {
	if len(args) == 1 && isHelpArg(args[0]) {
		_, err := io.WriteString(stdout, mapHelp)
		return err
	}
	path, options, err := parseMapOptions(args)
	if err != nil {
		return err
	}
	options.Version = badger.Version
	path, err = generate(path, options)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Generated codebase map HTML report.\nUse the following command to open:\n%s\n", mapOpenCommand(path, runtime.GOOS))
	return err
}

// parseMapOptions preserves exclusion text, order and duplicates.
func parseMapOptions(args []string) (string, codemap.Options, error) {
	target := "."
	options := codemap.Options{}
	seenTarget, literal := false, false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case !literal && arg == "--":
			literal = true
		case !literal && arg == "--no-repro-command":
			options.NoReproCommand = true
		case !literal && strings.HasPrefix(arg, "--exclude="):
			options.ExcludePatterns = append(options.ExcludePatterns, strings.TrimPrefix(arg, "--exclude="))
		case !literal && arg == "--exclude":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", options, fmt.Errorf("flag needs an argument: --exclude (use --exclude=<pattern> for patterns beginning with -)")
			}
			i++
			options.ExcludePatterns = append(options.ExcludePatterns, args[i])
		case !literal && strings.HasPrefix(arg, "-"):
			return "", options, fmt.Errorf("unknown badger map option %q; see badger map --help", arg)
		default:
			if seenTarget || arg == "" {
				return "", options, fmt.Errorf("badger map accepts at most one nonempty directory path; see badger map --help")
			}
			target, seenTarget = arg, true
		}
	}
	return target, options, pathmatch.Validate(options.ExcludePatterns)
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

const mapHelp = `Usage: badger map [path] [--exclude <pattern>]... [--no-repro-command]

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

Exclusions use case-sensitive relative paths from the mapped directory.
Slashless patterns match basenames at any depth; paths with / are root-anchored.
* matches within a component; whole-component ** matches zero or more components.
Matching a directory excludes its descendants. Use / on Windows too.
?, brackets, backslashes, absolute paths and . or .. components are rejected.
Other punctuation is literal; quote patterns to protect them from your shell.

With user exclusions, relative patterns are embedded in the HTML by default.
They can expose sensitive names, even paths absent from the map. Use
--no-repro-command to omit the indicator, command and exclusion metadata.
Existing report paths/counts remain embedded. Regeneration runs from the mapped
directory with target . and needs the same tree, version and settings.
Version is provenance, not a pinned executable. CWD controls output placement;
only the destination report is automatically omitted. Remove or explicitly
exclude stale reports for equivalent counts; nested reports may otherwise count.

Options:
  --exclude <pattern>  Add an exclusion (repeatable; also --exclude=<pattern>).
                       Use equals for patterns starting with -.
  --no-repro-command   Omit regeneration controls and exclusion metadata.
  --                   Treat remaining arguments as the directory path.
  -h, --help  Print help without scanning or writing.
`
