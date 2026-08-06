# Protocol

Badger bridges your local project and an AI chat in a three-step exchange: **Map → Extract → Apply**.

The goal editor may carry separate removable attachments, such as large pasted diffs or supporting notes. Those attachments stay outside the typed instruction surface and are assembled only when the goal is submitted.

Bare/default interactive startup selects Design focus. When an interactive
Design submission is empty, with no attachments, Badger uses this internal
exploratory task and proceeds through the normal Map → Extract flow:

```text
Explore this project with an open mind. Explain what stands out, how its main parts fit together, and surface any interesting opportunities, risks, or improvements worth investigating.
```

The exploratory task is not displayed in the editor beforehand. Code remains
explicit through `badger code` or `/code`; non-interactive API callers continue
to select `--focus <code|design>` explicitly. Typed goals and attachment-only
submissions retain their supplied content.

## Step 1: Map

**Prompt 1 (Map)** — the project topology — has this structure:

- **PROJECT TOPOLOGY** — languages, build stack, and module structure.
- **SOURCE TREE** — packages with file names and sizes, grouped by priority
  (docs, config, source code, assets).
- **EXTERNAL CONTEXT** — optional read-only context roots configured outside
  the normal project tree.
- **USER TAGGED FILES** — optional user-selected files you pin into the goal
  with `@path/to/file`; the section appears only when those references resolve.
  References inside fenced code, diffs, or attachment payloads are treated as
  literal context rather than file tags.
- **TASK** — your goal or question.
- **CONSTRAINT** — instructs the AI to reply with selectors only.

No source code is included.

Copy **Prompt 1 (Map)** and paste it into an AI chat.

## Step 2: Extract

The AI reads the topology and replies with selectors for the files it needs:

- `FILE:path` — extracts the entire file.
- `PREFIX:path#literal prefix` — finds the first line whose trimmed content starts with the prefix, then extracts a logical code block.
- `NEAR:path#literal string` — finds the first line containing the literal string, then extracts a logical code block.

Repo-local files are resolved first. If no repo-local file matches a `FILE:`
request, Badger may resolve it against configured read-only external context
using the exact displayed path, a path relative to the external root, a suffix
such as `docs/spec.md`, or a unique basename. Ambiguous external matches fail
with a candidate list instead of guessing.

Extraction attempts to include relevant comments preceding the matched line.
Badger searches for structural blocks (balanced braces, indentation, or
declarations) within a lookahead limit; if structural detection fails, it
falls back to a 10-line window (3 before, 6 after the match).

```text
FILE:internal/scanner/scanner.go
PREFIX:internal/scanner/scanner.go#func ScanProject(
NEAR:internal/scanner/scanner.go#detect project language
```

Copy the AI's reply and paste it back into Badger. Badger extracts the
relevant code and produces **Prompt 2 (Code Context)** — the extracted files or
code blocks with their full contents, alongside the project topology and task.

Prompt 2 has this structure:

- **PROJECT TOPOLOGY** — languages, build stack, module structure, and active extraction count.
- **TASK** — your goal or question.
- **OUTPUT CONSTRAINT** — instructs the AI to answer using the provided context, not selector lines.
- **CONTEXT** — extracted file blocks such as `(Full File)`, `(Extracted Span)`, or `(Binary Summary)`.

Interactive Review and Deep Review integrations use the same selectors as an
optional continuation escape hatch. Their initial review prompt asks for
immediate findings when the supplied diff and supporting context are
sufficient. Ordinary findings end the review without another Badger call. A
selector-only response can be pasted into interactive Review or passed to
`api review-continuation`; the supplemental payload contains only current
requested file context and compact review framing, so it does not resend the
initial diff. Files reflect the filesystem at continuation time rather than a
persisted review snapshot.

Interactive Review composes generated review context into the normal Prompt 1
schema, so it includes `[PROJECT TOPOLOGY]` and `[SOURCE TREE]`. The stable
`api review-context` output is intentionally standalone and topology-free; it
contains the review instructions, authoritative diff, status, guidance, and
eligible supporting context only. Integrations must not assume the API and TUI
Prompt 1 are byte-for-byte equivalent.

## Step 3: Apply

Copy **Prompt 2 (Code Context)** back to the AI chat. The AI reads the code and can write back
using:

- `--- File: <path> ---` ... content ... `--- End File ---` — creates or updates a file.
- `--- Delete File: <path> ---` — deletes a file.

```text
--- File: cmd/main.go ---
package main

func main() {}
--- End File ---
```
