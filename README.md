![AI Badger](assets/hero.png)

# AI Badger — Local AI Coding Context Tool

**Local-first codebase context extraction for any AI chat (Claude, ChatGPT, Grok, DeepSeek, etc.).**

Get **precise, token-efficient context** on demand without uploading your entire repo or wasting tokens on irrelevant files.

[![GitHub stars](https://img.shields.io/github/stars/PVRLabs/aibadger.svg)](https://github.com/PVRLabs/aibadger/stargazers)
[![Release](https://img.shields.io/github/v/release/PVRLabs/aibadger)](https://github.com/PVRLabs/aibadger/releases/latest)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Homebrew](https://img.shields.io/badge/Homebrew-available-brightgreen)](https://github.com/PVRLabs/homebrew-tap)
[![skills.sh](https://skills.sh/b/PVRLabs/aibadger)](https://skills.sh/PVRLabs/aibadger)

**No cloud • No API keys • No telemetry • Fully local**

[▶ Try Interactive Demo](https://pvrlabs.xyz/aibadger/demo.html) • [Install](#install)

[![AI Badger Interactive Demo](assets/demo.gif)](https://pvrlabs.xyz/aibadger/demo.html)

**Map → Extract → Apply:** Smart local context bridge that prepares focused codebase snippets for any LLM chat.

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

## Why AI Badger?

- **Universal compatibility** — Works with any AI chat interface or local model
- **Local-first codebase context tool** — Complete privacy, no uploads
- **Token & cost efficient** — Send only relevant context instead of repeatedly feeding the whole repository to cloud models
- **Precise & lightweight** — Built in Go, fast, minimal overhead
- **Specialized modes** — `review` and `design` for common workflows

## Install

### Homebrew (Recommended)
```bash
brew install pvrlabs/tap/badger
```

### Quick Curl Install
```bash
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | sh
```

See [docs/install.md](docs/install.md) for Windows, source builds, and more.

Also available as an official [VS Code companion](https://marketplace.visualstudio.com/items?itemName=pvrlabs.ai-badger) ([GitHub](https://github.com/PVRLabs/aibadger-vscode)).

## Agent Skills

Badger includes the `handoff` and `badger-review` skills for continuing an AI
coding session or requesting an independent review. See the [Agent Skills
guide](skills/README.md) for installation, usage, and details.

## Quick Start

1. Run `badger` in your project root. Interactive sessions start in Design focus.
2. Type your goal (or leave the editor empty and press Enter to explore the project).
3. Copy **Prompt 1** → paste into your AI chat.
4. When the AI asks for files, copy its response → paste back into Badger.
5. Copy **Prompt 2** → paste back to the AI.
6. Paste the AI’s response into Badger → review and apply changes.

### Specialized Modes
- `badger code` — explicitly start in Code focus
- `badger review` — Git changes and bounded supporting context for immediate findings
- `badger design` — explicitly start in Design focus with an empty editor

Full usage: [docs/usage.md](docs/usage.md)

## Learn More

- [Usage Examples & Walkthrough](docs/usage.md)
- [Browser Handoff Guide](docs/handoff.md)
- [Configuration: User limits and external context](docs/settings.md)
- [API Reference](docs/api.md) — Non-interactive commands for editors and scripts
- [Agent Integrations](docs/agents.md) — Compact repository orientation for coding agents
- [Limitations & Supported Projects](docs/limitations.md)
- [Privacy & Safety](docs/privacy.md)
- [Contributing](docs/development.md)

---

**If AI Badger makes your coding workflow simpler, consider starring the repo ⭐**
