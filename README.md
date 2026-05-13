# AIBadger

AIBadger is a local context bridge for bringing codebase context to an AI chat. It scans a project on your machine, builds compact prompts, and helps apply AI-written file updates only after you review them.

AIBadger does not call AI providers, upload files, run telemetry, or require a network connection for normal use. You choose what to copy into your AI chat.

## Install

The first public release is planned as `v0.1.0`. Until packaged binaries are published, build from source:

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
badger v0.1.0
```

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

To include read-only directories from outside your project (e.g., a shared spec or library repo), create a `.badger-context` file in the project root listing one path per line:

```
# .badger-context
../badger-sidecar/docs
```

Blank lines and `#` comments are ignored. All paths must exist and be directories. External context appears under `[EXTERNAL CONTEXT]` in the topology prompt and is never a valid apply target. Other AI coding tools, including agents like Codex, should look for `.badger-context` and treat each listed path as an additional read-only context directory to inspect.

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
