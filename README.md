# AI Badger

AI Badger helps you use any AI chat with your local codebase without uploading your repository or requiring an AI provider API key.

It scans a project on your machine, maps the important project structure into a compact prompt, and helps you extract only the files or code spans your AI chat requests. You stay in control of what gets copied, reviewed, and written back.

It stays local by default: no telemetry, no file uploads, and no cloud service required to prepare code context.

No API keys. No cloud dependency. No vendor lock-in.

## Why AI Badger?

- **Works with any AI chat** — AI Badger prepares local code context for manual paste into ChatGPT, Claude, Gemini, or any other chat interface when you choose to use one.
- **Local-first by design** — normal use requires no API keys, no cloud service, no telemetry, and no network connection.
- **Precise context extraction** — AI can ask for `FILE:`, `PREFIX:`, or `NEAR:` references, and AI Badger extracts the relevant file or nearby logical code block instead of dumping the whole repository.

## Install

Install from the public Homebrew tap:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
```

For source builds, release builds, and version checks, see
[docs/install.md](docs/install.md).

## Usage

Run `badger` from the root of the project you want to inspect:

```bash
badger
```

## Workflow

AI Badger follows a simple local workflow:

1. **Map**: AIBadger scans your project and prepares Prompt 1: Topology.
2. **Extract**: paste the AI's requested `FILE:`, `PREFIX:`, or `NEAR:` commands back into AIBadger, and AIBadger prepares Prompt 2: Code Context.
3. **Apply**: paste the AI's final response, review the write plan, and confirm before files are written.

Prompt 1 contains project structure, file paths, and your goal, not source code.
Prompt 2 contains selected source context based on the extraction commands you approve.

AIBadger works for code review, bug and performance analysis, code explanation, planning, and focused implementation requests. Start by typing or pasting a goal into AIBadger.

## Example tasks

1. Review a change before committing.
2. Find bugs or performance issues in a focused area.
3. Understand an unfamiliar part of a codebase.
4. Make a focused implementation request.

Example review goal:

```text
Review my current change for bugs.

[Paste git diff here]
```

For the workflow and `.badger-context`, see [docs/usage.md](docs/usage.md).

## Context selectors

AI Badger can extract focused context from selector-style requests such as:

```text
FILE:internal/scanner/scanner.go
PREFIX:internal/scanner/scanner.go#func ScanProject(
NEAR:internal/scanner/scanner.go#detect project language
```

These are examples of the plain-text selectors you can use to ask for more context without pasting the whole repository.

## Privacy and safety

See [docs/privacy.md](docs/privacy.md).

## Supported projects

See [docs/usage.md](docs/usage.md) for the supported project model and
[docs/limitations.md](docs/limitations.md) for the current scan boundaries and
limits.

## Go package

The public facade is `github.com/PVRLabs/aibadger/pkg/badger`.

For contributor-facing notes, see [docs/development.md](docs/development.md).
