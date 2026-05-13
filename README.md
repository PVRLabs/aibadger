# AIBadger

AIBadger is a local context bridge for bringing codebase context to an AI chat. It scans a project on your machine, builds compact prompts, and helps apply AI-written file updates only after you review them.

AIBadger does not call AI providers, upload files, run telemetry, or require a network connection for normal use. You choose what to copy into your AI chat.

## Install

The current public release is `v0.1.1`.

Install from the public Homebrew tap:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
```

The tap is published at `https://github.com/PVRLabs/homebrew-aibadger` and
pulls release tarballs from GitHub Releases.

If you prefer to build from source:

```bash
go build -o badger ./cmd/badger
```

Release-mode build:

```bash
go build -tags aibadger_release -ldflags="-s -w" -o badger ./cmd/badger
```

Check the version:

```bash
./badger --version
```

Expected output:

```text
badger v0.1.1
```

For release instructions, see [docs/releasing.md](docs/releasing.md).

## Usage

Run AIBadger from the root of the project you want to inspect:

```bash
badger
```

The interactive workflow is:

1. Map: AIBadger scans your project and prepares a topology prompt.
2. Extract: You paste the AI's requested file commands back into AIBadger, and AIBadger prepares selected source context.
3. Apply: You paste the AI's final response, review the write plan, and confirm before files are written.

Prompt 1 contains project structure and file paths, not source code. Prompt 2 contains selected source context based on the extraction commands you approve.

### External Context

You can add read-only external directories by creating a `.badger-context` file in the project root, one path per line.

## Privacy And Safety

- All scanning and extraction runs locally.
- No telemetry is collected.
- No cloud sync is used.
- No source code is copied until you approve the handoff.
- No file writes happen until you review the preview and confirm.
- Secret and credential locations such as `.env` and `.git` are excluded from scanning.

## Supported Projects

AIBadger includes first-class scanning for:

- Go
- Java
- JavaScript
- TypeScript
- Python

It also includes generic project scanning for common source and configuration files when a first-class detector does not apply.

## Known Limits

- AIBadger is a context bridge, not an AI provider or autonomous coding agent.
- It does not solve tasks by itself; your AI chat remains the reasoning layer.
- Extraction commands are intentionally simple: `FILE:`, `PREFIX:`, and `NEAR:`.
- Release builds hide development-only headless automation flags.
- Binary and generated files are intentionally excluded or minimized to keep prompts compact.

## Go Package

The public facade is:

```go
github.com/PVRLabs/aibadger/pkg/badger
```

Most implementation packages remain under `internal/` so the CLI and facade can evolve without exposing scanner, extractor, protocol, writer, or TUI internals as public API.
