package main

import (
	"fmt"
	"io"
)

func apiHelpRequest(args []string) (string, bool) {
	if len(args) == 1 && isAPIHelpFlag(args[0]) {
		return "", true
	}
	if len(args) < 2 || !isStableAPIOperation(args[0]) {
		return "", false
	}
	for _, arg := range args[1:] {
		if isAPIHelpFlag(arg) {
			return args[0], true
		}
	}
	return "", false
}

func isAPIHelpFlag(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func isStableAPIOperation(operation string) bool {
	switch operation {
	case "topology", "prompt", "extract", "review-context", "review-continuation":
		return true
	default:
		return false
	}
}

func printAPIHelp(w io.Writer, operation string) {
	switch operation {
	case "topology":
		fmt.Fprint(w, `Usage:
  badger api topology --root <project>

Purpose:
  Produce a compact repository topology for tools and coding agents.

Required arguments:
  --root <project>  Existing project directory, absolute or relative.

Optional arguments:
  --help, -h        Print this help and exit.

Example:
  badger api topology --root .

Output and side effects:
  Writes topology content to stdout and diagnostics only to stderr. This
  command is non-interactive and read-only. It does not use the clipboard,
  open a browser, access the network, or change Badger settings.

Failures:
  Exits nonzero when arguments or the root are invalid, the project is
  disabled, or the repository cannot be scanned. Normal output uses
  root-relative paths and does not expose the absolute repository root.
`)
	case "prompt":
		fmt.Fprint(w, `Usage:
  badger api prompt --root <project> --focus <code|design> --input <goal-file>

Purpose:
  Produce the complete first-stage Badger prompt for the human AI-chat
  handoff workflow.

Required arguments:
  --root <project>   Existing project directory, absolute or relative.
  --focus <value>    Prompt focus: code or design.
  --input <file>     UTF-8 file containing the goal or question.

Optional arguments:
  --help, -h         Print this help and exit.

Example:
  badger api prompt --root . --focus code --input goal.txt

Output and side effects:
  Writes the complete prompt to stdout and diagnostics only to stderr. This
  command is non-interactive and read-only. It does not use the clipboard,
  open a browser, access the network, or change Badger settings.

Failures:
  Exits nonzero for invalid arguments, root or input errors, an empty goal,
  a disabled project, or a repository scan failure.
`)
	case "extract":
		fmt.Fprint(w, `Usage:
  badger api extract --root <project> [--focus <code|design>] --input <selector-file> --goal-file <goal-file>

Purpose:
  Produce the complete second-stage Badger prompt with selected repository
  context for the human AI-chat handoff workflow.

Required arguments:
  --root <project>    Existing project directory, absolute or relative.
  --input <file>      UTF-8 file containing FILE, PREFIX, or NEAR selectors.
  --goal-file <file>  UTF-8 file containing the original goal.

Optional arguments:
  --focus <value>     Final-answer focus: code or design (default: code).
  --help, -h          Print this help and exit.

Example:
  badger api extract --root . --focus code --input selectors.txt --goal-file goal.txt

Output and side effects:
  Writes the complete prompt to stdout and diagnostics only to stderr.
  Partial selector failures may produce usable stdout with stderr warnings.
  This command is non-interactive and read-only. It does not use the
  clipboard, open a browser, access the network, or change Badger settings.

Failures:
  Exits nonzero for invalid arguments, root or input errors, empty input,
  a disabled project, scan failure, or when no safe usable context exists.
`)
	case "review-context":
		fmt.Fprint(w, `Usage:
  badger api review-context --root <repository> [--mode <default|staged|branch|commit>] [--ref <revision>] [--input <guidance-file>] [--paths-file <paths.json>] [--include-topology] [--max-payload-bytes <bytes>] [--max-file-bytes <bytes>]

Purpose:
  Produce a complete standalone review request from authoritative Git state.
  The request is directly usable. Project topology and the source tree are
  omitted unless --include-topology is explicitly supplied.

Required arguments:
  --root <repository>  The Git repository root.

Optional arguments:
  --mode <value>       Review scope (default: default working tree).
  --ref <revision>     Required for branch and commit modes only.
  --input <file>       UTF-8 review guidance file (maximum 1 MiB).
  --paths-file <file>  JSON array of literal repository-relative changed paths;
                       accepted only in default mode (maximum 1 MiB).
  --include-topology    Include Badger-owned project topology and source tree
                        before the review context under the shared byte limit.
  --max-payload-bytes <bytes>  Positive complete-prompt byte limit.
  --max-file-bytes <bytes>     Positive per-file supporting-context limit.
  --help, -h           Print this help and exit.

Example:
  badger api review-context --root . --mode default --input guidance.txt

Output and side effects:
  Writes the complete AI-facing review request to stdout. Topology is included
  only when --include-topology is explicitly supplied.
  Diagnostics go only to stderr. This command is deterministic for unchanged Git state,
  non-interactive, and read-only. It does not use stdin, the clipboard, a
  browser, the TUI, providers, or the network.

Failures:
  Exits nonzero without writing stdout for generation failures such as invalid
  arguments or inputs, invalid Git roots or refs, no reviewable changes, Git
  failures, invalid selected paths, or mandatory payload overflow. A stdout
  destination failure also exits nonzero but may have accepted a partial
  prefix. Normal output uses root-relative paths.
`)
	case "review-continuation":
		fmt.Fprint(w, `Usage:
  badger api review-continuation --root <repository> --input <selector-file> [--max-payload-bytes <bytes>] [--max-file-bytes <bytes>]

Purpose:
  Produce supplemental current-file context for an existing Deep Review chat.

The input must contain only FILE, PREFIX, or NEAR selectors, one per line.
Normal findings need no continuation call; mixed findings and selectors are
rejected. Output omits the initial diff and changed-file context. Files reflect
the filesystem at continuation time and may be newer than the initial review.
The operation is non-interactive, read-only, and provider-independent.
`)
	default:
		fmt.Fprint(w, `Usage:
  badger api --help
  badger api topology --root <project>
  badger api prompt --root <project> --focus <code|design> --input <goal-file>
  badger api extract --root <project> [--focus <code|design>] --input <selector-file> --goal-file <goal-file>
  badger api review-context --root <repository> [--mode <default|staged|branch|commit>]
  badger api review-continuation --root <repository> --input <selector-file>

Purpose:
  Run Badger's stable, non-interactive text API for local integrations.

Stable operations:
  topology  Produce a compact repository map.
  prompt    Produce the complete first-stage human handoff prompt.
  extract   Produce the complete second-stage prompt with selected context.
  review-context  Produce an initial Deep Review prompt from Git changes.
  review-continuation  Produce supplemental context for an existing review.

Arguments:
  Every operation requires --root <project>. Run an operation with --help or
  -h for its required and optional arguments.

Output and side effects:
  Usable content is written to stdout. Diagnostics are written only to
  stderr. API commands are non-interactive and read-only; they do not use the
  clipboard, open a browser, access the network, or change Badger settings.

Failures:
  Invalid arguments, invalid or disabled roots, unreadable inputs, scan
  failures, and operations that cannot produce usable output exit nonzero.

Examples:
  badger api topology --root .
  badger api prompt --root . --focus code --input goal.txt
  badger api review-context --root .
`)
	}
}
