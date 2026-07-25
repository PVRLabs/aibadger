# API Reference

Badger provides a small, stable, non-interactive command surface for editor,
script, and local-tool use. API commands write directly usable prompt text to
stdout and use stderr plus exit status for diagnostics.

Every API command requires `--root <project>`, which must be an absolute or
relative path to an existing directory. Badger normalizes it to an absolute
path and uses it as the project root.

Input files (`--input`, `--goal-file`) are UTF-8, caller-managed files. Badger
reads them without modifying or retaining them. Caller-provided paths are
resolved relative to the current working directory, not the `--root`.

Errors and warnings go to stderr. A nonzero exit status means the operation
could not produce usable output. A zero exit with content on stderr means
usable output was produced alongside diagnostics (for example, partial
extraction with some failed selectors).

The API outputs only directly usable AI-facing text. It does not produce JSON,
structured topology, or extraction metadata. All existing safety rules apply:
`.badger-disable`, sensitive/binary file protection, external-context read-only
behavior, and size limits.

## Integration flow

A model integration normally uses two calls:

1. Run `api prompt` with the user's goal and send its complete stdout to the
   model.
2. Save the model's selector-only response, run `api extract`, and send that
   command's complete stdout back to the same model conversation.

Use `api topology` instead when the integration needs only Badger's compact
project map and will manage its own prompting or extraction workflow.

## Commands

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

### `api prompt`

Print a complete Prompt 1 (Map) — topology plus task and extraction constraint.

```bash
badger api prompt --root <project> --focus <code|design> --input <goal-file>
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
exit status.
