![AI Badger](assets/hero.png)
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

Or install the latest release with curl:

```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | sh
```

For source builds, release builds, version checks, and Windows install
instructions, see [docs/install.md](docs/install.md).

## Quick start

[**Watch the demo**](https://pvrlabs.xyz/aibadger/demo.html) - See how AI Badger works in a simple animated walkthrough.

1. **Goal** — Run `badger` in your project, type a goal like `Review this change for bugs`, paste a git diff, and press Enter.
2. **Map** — AIBadger scans your project and shows **Prompt 1: Topology** (project structure and files, not source code). Copy it and paste into any AI chat (Claude, ChatGPT, Gemini, etc.).
3. **Extract** — The AI asks for the files it needs. Copy its reply (e.g. `FILE:internal/scanner/scanner.go`) and paste it back into AIBadger. AIBadger prepares **Prompt 2: Code Context** with the relevant source.
4. **Analyze** — Copy Prompt 2 back to the AI chat. The AI reads the code and responds with analysis or code changes.
5. **Apply** — Paste the AI's final response into AIBadger, review the write plan, and confirm.

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

For walkthroughs and more examples, see [docs/usage.md](docs/usage.md).

## Learn more

- [Usage walkthrough and examples](docs/usage.md)
- [Protocol reference](docs/protocol.md)
- [Supported project model and limitations](docs/limitations.md)
- [Privacy and safety](docs/privacy.md)
- [Contributor guide](docs/development.md)
