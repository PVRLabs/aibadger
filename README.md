# AI Badger

AI Badger is a local context bridge for using any AI chat with your codebase.

It scans a project on your machine, maps the important project structure into a compact prompt, and helps you extract only the files or code spans the AI needs. Paste a diff, include project topology/context, and use any AI chat as a lightweight code reviewer before committing.

It stays local by default: no telemetry, no file uploads, and no cloud service required to prepare code context.

No API keys. No cloud dependency. No vendor lock-in.

## Why Badger?

- **Works with any AI chat** — Badger prepares local code context for manual paste into ChatGPT, Claude, Gemini, or any other chat interface when you choose to use one.
- **Local-first by design** — normal use requires no API keys, no cloud service, no telemetry, and no network connection.
- **Precise context extraction** — AI can ask for `FILE:`, `PREFIX:`, or `NEAR:` references, and Badger extracts the relevant file or nearby logical code block instead of dumping the whole repository.

## Install

Install from the public Homebrew tap:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
```

For source builds, release builds, and version checks, see
[docs/install.md](docs/install.md).

## Usage

Run Badger from the root of the project you want to inspect:

```bash
badger
```

## Workflow

1. Run `badger` from your project root.
2. Copy the generated project context into your AI chat.
3. Ask the AI to review a diff, explain code, or request more context.
4. Use Badger's plain-text selectors to extract focused files or spans.
5. Review any AI-written updates before applying them.

Example review prompt:

```text
Review this diff for concrete bugs, edge cases, maintainability issues, and unintended behavior changes.

Focus on issues I should fix before committing.

[Paste Badger project context here]

[Paste git diff here]
```

For the workflow and `.badger-context`, see [docs/usage.md](docs/usage.md).

## Privacy and safety

See [docs/privacy.md](docs/privacy.md).

## Supported projects

See [docs/usage.md](docs/usage.md) for the supported project model and
[docs/limitations.md](docs/limitations.md) for the current scan boundaries.

## Known limits

See [docs/limitations.md](docs/limitations.md).

## Go package

The public facade is `github.com/PVRLabs/aibadger/pkg/badger`.

For contributor-facing notes, see [docs/development.md](docs/development.md).
