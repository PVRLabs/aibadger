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

## Commands

### `api topology`

Print the project topology text.

```bash
badger api topology --root <project>
```

The topology is identical to the prompt section produced by `api prompt`, but
without the task or constraint sections. Useful for callers that need only the
project structure.

### `api prompt`

Print a complete Prompt 1 (Map) — topology plus task and extraction constraint.

```bash
badger api prompt --root <project> --focus <code|design> --input <goal-file>
```

`--focus` selects the initial instruction set. Supported values are `code` and
`design`. `--input <goal-file>` must point to a UTF-8 file containing the goal
or question for the AI.

### `api extract`

Print a complete Prompt 2 (Code Context) — topology, task, and extracted source
code.

```bash
badger api extract --root <project> --input <selector-file> --goal-file <goal-file>
```

`--input <selector-file>` is a UTF-8 file containing the AI's extraction
selectors (`FILE:`, `PREFIX:`, `NEAR:`), one per line. `--goal-file
<goal-file>` is the same original goal that was passed to `api prompt`.

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
