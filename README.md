![AI Badger](assets/hero.png)
# AI Badger

Badger is a lightweight local bridge between your codebase and any AI chat (Claude, ChatGPT, DeepSeek, Grok, etc.).

## How it works

**1. Map**  
Enter your goal. Badger builds a prompt.  
↳ You copy it → paste into your AI chat

**2. Extract**  
AI replies asking for specific files.  
↳ You copy that → paste back into Badger

**3. Apply**  
Badger fetches those files, builds a second prompt.  
↳ You copy it → paste into AI → review before writing

✓ Fully local — nothing leaves your machine until you copy it  
✓ You control every paste and every write

[▶ Watch the interactive demo](https://pvrlabs.xyz/aibadger/demo.html)

![AI Badger demo](assets/demo.gif)

_Map → Extract → Apply: prepare focused code context for any AI chat._

## Why AI Badger?

- **Works with any AI chat** — Prepares clean, local code context for manual paste into any chat interface.
- **Local-first by design** — No API keys, no cloud service, no telemetry, and no network required.
- **Precise context extraction** — AI requests only the files or spans it needs using `FILE:`, `PREFIX:`, or `NEAR:` selectors.

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

For source builds, release builds, version checks, and Windows install instructions, see [docs/install.md](docs/install.md).

## Quick Start

1. Run `badger` in your project root.
2. Type your goal (or paste a git diff).
3. Copy **Prompt 1 (Map)** and paste into your AI chat.
4. When the AI asks for files, copy its response and paste back into Badger.
5. Copy **Prompt 2 (Code Context)** back to the AI.
6. Paste the AI’s final response into Badger to review and apply changes.

### Specialized Modes

- `badger review` — Starts directly in review mode with the current git diff pre-loaded.
- `badger design` — Starts in design mode, focused on architecture, tradeoffs, and planning.

### Badge Scoreboard

- `badger badge` — Launches the TUI with `/badge` preloaded for this repo.
- It asks for confirmation before making a single GitHub API call.
- Press `[S]` to open the repo in your browser and star it.
- If the repo has 100 listed stargazers, Badger switches to "A GAZILLION BADGERS" mode.

For more commands, flags, and advanced usage, see [docs/usage.md](docs/usage.md).

### Example

```bash
badger
```

Then type your goal:

```text
Understand how authentication works. @internal/auth.go @docs/auth-design.md
```

For walkthroughs and more examples, see [docs/usage.md](docs/usage.md).

## Learn more

- [Usage walkthrough and examples](docs/usage.md)
- [Protocol reference](docs/protocol.md)
- [Supported project model and limitations](docs/limitations.md)
- [Privacy and safety](docs/privacy.md)
- [Contributor guide](docs/development.md)
