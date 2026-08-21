![AI Badger](assets/hero.png)

# AI Badger — Local AI Coding Context Tool

**Local-first codebase context extraction for any AI chat (Claude, ChatGPT, Grok, DeepSeek, etc.).**

Get **precise, token-efficient context** on demand without uploading your entire repo or wasting tokens on irrelevant files.

[![GitHub stars](https://img.shields.io/github/stars/PVRLabs/aibadger.svg)](https://github.com/PVRLabs/aibadger/stargazers)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.26+-00ADD8.svg)](go.mod)
[![Homebrew](https://img.shields.io/badge/Homebrew-available-brightgreen)](https://github.com/PVRLabs/homebrew-tap)

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

Perfect for **Claude token saving**, local LLM workflows, code reviews, design sessions, and debugging.

## Why AI Badger?

- **Universal compatibility** — Works with any AI chat interface or local model
- **Local-first codebase context tool** — Complete privacy, no uploads
- **Token efficient** — Stop burning agent tokens on reviews, explanations, or brainstorming
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

Also available with an official [VS Code companion](https://marketplace.visualstudio.com/items?itemName=pvrlabs.ai-badger).

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
- `badger continue` — consume a handoff prepared by another AI coding session

To continue work from another agent, run `badger continue` from the same
directory where that agent wrote `.badger-handoff`. The file is an ephemeral,
conversation-only handoff; do not commit it or include diffs, Git status, or
repository files in its body. See [docs/usage.md](docs/usage.md) for the three
supported modes and the exact format.

For editor integrations and scripts, the stable `badger api review-context`
operation generates a standalone, topology-free review request from one Git
repository; the optional `review-continuation` operation supplies
selector-requested context.

Full usage: [docs/usage.md](docs/usage.md)

## Learn More

- [Usage Examples & Walkthrough](docs/usage.md)
- [Browser Handoff Guide](docs/handoff.md)
- [API Reference](docs/api.md) — Non-interactive commands for editors and scripts
- [Agent Integrations](docs/agents.md) — Compact repository orientation for coding agents
- [Articles](docs/articles/)
- [Protocol Reference](docs/protocol.md)
- [Limitations & Supported Projects](docs/limitations.md)
- [Privacy & Safety](docs/privacy.md)
- [Contributing](docs/development.md)

---

**Star if this local AI coding context tool solves a real pain for you ⭐**

Built in San Diego by [PVR Labs](https://pvrlabs.xyz). 🌊

[Website](https://pvrlabs.xyz/aibadger) • [X @kupolov](https://x.com/kupolov)
