# Protocol

AIBadger bridges your local project and an AI chat in a three-step exchange.

## Step 1: Map

**Prompt 1: Topology** has this structure:

- **PROJECT TOPOLOGY** — languages, build stack, and module structure.
- **SOURCE TREE** — packages with file names and sizes, grouped by priority
  (docs, config, source code, assets).
- **USER TAGGED FILES** — optional user-selected files you pin into the goal
  with `@path/to/file`; the section appears only when those references resolve.
- **TASK** — your goal or question.
- **CONSTRAINT** — instructs the AI to reply with selectors only.

No source code is included.

Copy Prompt 1 and paste it into an AI chat.

## Step 2: Extract

The AI reads the topology and replies with selectors for the files it needs:

- `FILE:path` — extracts the entire file.
- `PREFIX:path#symbol` — extracts a declaration starting with that prefix.
- `NEAR:path#keyword` — extracts the code block around the first matching line.

```text
FILE:internal/scanner/scanner.go
PREFIX:internal/scanner/scanner.go#func ScanProject(
NEAR:internal/scanner/scanner.go#detect project language
```

Copy the AI's reply and paste it back into AIBadger. AIBadger extracts the
relevant code and produces **Prompt 2: Code Context** — the extracted files or
code blocks with their full contents, alongside the project topology and task.

## Step 3: Respond

Copy Prompt 2 back to the AI chat. The AI reads the code and can write back
using:

- `--- File: <path> ---` ... content ... `--- End File ---` — creates or updates a file.
- `--- Delete File: <path> ---` — deletes a file.

```text
--- File: cmd/main.go ---
package main

func main() {}
--- End File ---
```
