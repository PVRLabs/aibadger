# API Reference

Badger provides a supported, stable, non-interactive command surface for
editors, scripts, coding agents, and other local tools. The stable operations
are `topology`, `prompt`, `extract`, `review-context`, and
`review-continuation`. The official
[VS Code companion](https://github.com/PVRLabs/aibadger-vscode) uses this API
for Ask and Deep Review.

Use `badger api --help` for an overview or add `--help` (or `-h`) to a stable
operation for command-specific usage.

Every API command requires `--root <project>`, which must be an absolute or
relative path to an existing directory. Badger normalizes it to an absolute
path and uses it as the project root.

Input files (`--input`, `--goal-file`) are UTF-8, caller-managed files. Badger
reads them without modifying or retaining them. Caller-provided paths are
resolved relative to the current working directory, not the `--root`.

Usable output goes to stdout. Errors and warnings go only to stderr. The
`prompt` and `review-context` operations also accept `--clipboard`; in that
mode, Badger copies the complete output using its native clipboard support and
writes only a short confirmation to stdout. A nonzero exit status means the
operation could not produce usable output. A zero exit with content on stderr
means usable output was produced alongside diagnostics (for example, partial
extraction with some failed selectors).

The API outputs only directly usable AI-facing text. It does not produce JSON,
structured topology, or extraction metadata. All existing safety rules apply:
`.badger-disable`, sensitive/binary file protection, external-context read-only
behavior, and size limits.

## Coding-agent usage

For an unfamiliar or non-trivial repository, a coding agent can use topology
near the beginning of a task:

```bash
badger api topology --root .
```

The agent should use the result to identify likely entrypoints, packages,
tests, configuration, and documentation, then continue with its native search,
file-reading, editing, and testing tools. Topology is a compact map, not a
substitute for reading source code.

Avoid rerunning topology during the same task unless the repository structure
has materially changed. Coding agents with direct repository access do not
normally need `prompt` or `extract`; those operations support Badger's existing
human AI-chat handoff workflow.

See [Agent Integrations](agents.md) for practical decision rules and usage
guidance.

## Human handoff integration flow

A human AI-chat integration normally uses two calls:

1. Run `api prompt` with the user's goal and send its complete stdout to the
   model.
2. Save the model's selector-only response, run `api extract`, and send that
   command's complete stdout back to the same model conversation.

Use `api topology` instead when an integration needs only Badger's compact
project map and will manage its own repository access.

## Commands

### `api review-context`

Print a complete review request from current Git state. “Complete” means the
output is directly usable without assembling another review-context envelope.
Topology is omitted unless `--include-topology` is explicitly requested.

```bash
badger api review-context --root <repository> \
  [--mode <default|staged|branch|commit>] [--ref <revision>] \
  [--input <guidance-file>] [--paths-file <paths.json>] \
  [--include-topology] \
  [--max-payload-bytes <bytes>] [--max-file-bytes <bytes>] \
  [--clipboard]
```

The default mode reviews staged and unstaged tracked changes and relevant
Git-untracked paths. `staged` reviews the index, `branch` reviews `HEAD` from
the merge base with `--ref`, and `commit` reviews the commit named by `--ref`.
Only default mode accepts `--paths-file`; that file is a UTF-8 JSON array of
literal repository-relative changed paths, for example
`["internal/client.go","deleted.go"]`. Badger validates and deduplicates the
paths, accepts current deleted paths, and rejects absolute, escaping, or
unchanged paths.

`--input` optionally supplies current review guidance. Both input files are
caller-owned, read once, and limited to 1 MiB. The byte-limit options must be
positive. Omitting them uses Badger's 512 KiB complete-prompt and 64 KiB
per-supporting-file defaults. The authoritative diff is mandatory; complete
eligible changed files are included only when they fit. Tracked additions,
deletions, binary files, sensitive files, oversized files, and untracked files
follow the status policy printed in the prompt.

The selected mode controls the authoritative fenced diff. Optional complete
supporting files are read from the current checked-out working tree at
generation time, including in staged, branch, and commit modes. Supporting
content may therefore be newer than the reviewed diff and never changes which
patch is authoritative.

The operation builds the complete AI-facing review request before delivery.
Without `--clipboard`, it writes that request to stdout exactly as usual. With
`--clipboard`, it copies the request using Badger's native clipboard support
and writes only a short success message to stdout. Clipboard failure exits
nonzero and never prints the complete request as a fallback. With
`--include-topology`, Badger owns the project scan, source-tree
format, ordering, and shared payload budget; the authoritative diff remains
mandatory. Without that flag, topology is not inspected or included. It
exits nonzero without writing stdout for generation failures such as no changes,
an invalid Git root or ref, Git failure, invalid selections, or mandatory
overflow. A destination write failure also returns nonzero; as with any stream,
the destination may already contain a partial prefix when that happens. The
operation is non-interactive and read-only. It scans project topology only when
`--include-topology` is supplied. It does not read stdin, open the TUI or a
browser, contact providers, or access the network. It accesses the clipboard
only when `--clipboard` is supplied. Normal output uses repository-relative paths and does
not expose the absolute repository root.

Editor integrations can capability-check this operation with `badger api
--help` and `badger api review-context --help`; the latter advertises
`--include-topology` and `--clipboard` when supported. Without `--clipboard`,
successful stdout is the complete request to copy verbatim. With it, successful
stdout is only the clipboard confirmation. On failure, stderr contains the
`Error: ...` diagnostic and stdout is empty. The command never contacts an AI
provider.

### `api review-continuation`

Print supplemental context for an existing Deep Review conversation.

```bash
badger api review-continuation --root <repository> \
  --input <selector-file> \
  [--max-payload-bytes <bytes>] [--max-file-bytes <bytes>]
```

The input must contain only complete `FILE:`, `PREFIX:`, or `NEAR:` selectors,
one per non-empty line. A findings-only response needs no continuation call.
Badger rejects findings-only, mixed prose-and-selector, empty, and malformed
responses rather than interpreting natural-language findings.

Output contains compact continuation instructions and the requested context;
it does not repeat the initial diff, changed-file blocks, guidance, or
topology. Files are read from the current filesystem when this command runs and
may therefore be newer than the initial review context. Existing extraction
safety, deduplication, partial-success, and deterministic ordering rules apply.
Warnings go to stderr. Positive byte-limit options override the normal Prompt 2
limits; the call fails without stdout if no usable supplemental context fits.

### `api topology`

Print the project topology text.

```bash
badger api topology --root <project>
```

The topology is identical to the prompt section produced by `api prompt`, but
without the task or constraint sections. Useful for callers that need only the
project structure.

Example stdout (abbreviated):

```text
[PROJECT TOPOLOGY]
Languages: Go
Stack: Go Modules
Structure: Single Module

[SOURCE TREE]
Pkg: . [3 files] -> Top: README.md (4KB), go.mod (1KB), main.go (1KB)
Pkg: internal/client [4 files] -> Top: client.go (8KB), config.go (2KB)
...
```

The topology is AI-facing text rather than JSON. Callers can pass it directly
to a model or embed it in their own prompt.

#### Topology contract

`api topology`:

- is non-interactive and read-only;
- does not access the clipboard, open a browser, access the network, or change
  Badger settings;
- accepts an explicit repository root through `--root`;
- writes topology content to stdout and diagnostics only to stderr;
- returns a nonzero exit status when it cannot produce usable output;
- uses root-relative paths and does not expose the absolute repository root in
  normal output; and
- produces deterministic output for an unchanged repository.

The command can fail when its arguments or root are invalid, the project is
disabled with `.badger-disable`, or the repository cannot be scanned.

### `api prompt`

Print a complete Prompt 1 (Map) — topology plus task and extraction constraint.

```bash
badger api prompt --root <project> --focus <code|design> --input <goal-file> [--clipboard]
```

`--focus` selects the initial instruction set. Supported values are `code` and
`design`. `--input <goal-file>` must point to a UTF-8 file containing the goal
or question for the AI.

For example, `goal.txt` might contain:

```text
Add timeout handling to the API client.
```

```bash
badger api prompt --root ./my-project --focus code --input goal.txt > prompt-1.txt
```

The resulting `prompt-1.txt` has this structure (abbreviated):

```text
[PROJECT TOPOLOGY]
...

[SOURCE TREE]
...

[TASK]
Add timeout handling to the API client.

[CONSTRAINT]
...
```

Send the complete stdout payload to the model. Prompt 1 contains topology and
file metadata, but not source-file contents. Its constraint asks the model to
return extraction selectors for the context it needs.

Alternatively, pass `--clipboard` to copy the complete prompt using Badger's
native clipboard support. In that mode stdout contains only a short success
message. Clipboard failure exits nonzero without printing the prompt as a
fallback. Other behavior is unchanged.

### `api extract`

Print a complete Prompt 2 (Code Context) — topology, task, and extracted source
code.

```bash
badger api extract --root <project> [--focus <code|design>] --input <selector-file> --goal-file <goal-file>
```

`--input <selector-file>` is a UTF-8 file containing the AI's extraction
selectors (`FILE:`, `PREFIX:`, `NEAR:`), one per line. `--goal-file
<goal-file>` is the same original goal that was passed to `api prompt`.
`--focus` selects the final-answer instruction set and accepts `code` or
`design`. It is optional for backward compatibility; omitting it uses `code`.
Callers that use a focus for `api prompt` should pass the same focus to
`api extract`.

For example, a model response saved as `selectors.txt` might contain:

```text
FILE:internal/client/client.go
PREFIX:internal/client/client_test.go#func TestClientTimeout
NEAR:internal/client/config.go#Timeout
```

`FILE:` requests a complete file. `PREFIX:` and `NEAR:` use the text after `#`
to locate a relevant source span. See the [Protocol](protocol.md#step-2-extract)
for the selector matching rules.

Use the same goal file from `api prompt`:

```bash
badger api extract \
  --root ./my-project \
  --focus code \
  --input selectors.txt \
  --goal-file goal.txt \
  > prompt-2.txt
```

The resulting `prompt-2.txt` has this structure (abbreviated):

```text
[PROJECT TOPOLOGY]
...

[TASK]
Add timeout handling to the API client.

[OUTPUT CONSTRAINT]
...

[CONTEXT]
--- File: internal/client/client.go (Full File) ---
...
--- End File ---
--- File: internal/client/client_test.go (Extracted Span) ---
...
--- End File ---
```

Send the complete stdout payload back to the same model conversation. Prompt 2
contains only the source context selected for extraction, subject to Badger's
safety and size limits.

If some selectors fail (file not found, ambiguous path, safety exclusion), the
corresponding diagnostics go to stderr while any usable extracted content is
still written to stdout. The exit status is nonzero only when no usable content
can be produced.

## Error example

```bash
$ badger api prompt --root /nonexistent --focus code --input goal.txt
Error: validating api root: stat /nonexistent: no such file or directory
$ echo $?
1
```

All errors follow the same pattern: an `Error:` prefix on stderr and a nonzero
exit status. Help commands return zero, write help to stdout, and do not scan
the repository.
