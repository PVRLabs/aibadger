# Official Agent Skills

AI Badger includes two small, portable workflow integrations for AI coding
agents:

- `badger-handoff` continues work in another AI session with a concise summary
  of the current conversation and focused repository context.
- `badger-review` prepares a quick, one-shot independent review of the current
  work using the session's intent and Badger's review context.

These are workflow integrations, not a replacement for an AI agent's native
repository tools. The calling agent must understand the current conversation
and summarize the requirements, constraints, decisions, and relevant status
correctly. The summary is selective session guidance, not a complete
conversation transcript, so include anything that must survive into the next
session or review. Behavior may evolve with real-world use; no strong
compatibility or security guarantee is made across every agent environment.

## Responsibility and privacy

The calling agent owns conversation understanding and summarization. AI Badger
owns repository context, including repository scanning, review Git context,
privacy exclusions, and prompt construction through its existing APIs. The
skills contain little repository-analysis logic.

Summaries should include only relevant information and should avoid repeating
credentials, tokens, passwords, private keys, `.env` values, or other obvious
secret values. This conversation minimization is best effort: the skill layer
is not a complete data-loss-prevention or secret-scanning system. Badger's
normal repository privacy and exclusion behavior applies to the repository
context it generates.

Handoff may include shallow Git metadata so work can be resumed, such as the
current branch and changed path names with their status codes. It does not
include diffs, file contents, commit contents, or broad history.

## How the workflows behave

Review is intentionally a quick, one-shot independent review. It uses the
stable `badger api review-context` operation and returns the complete review
package. For another focused repository-context round, use interactive Badger
Review/TUI instead of expecting the skill to orchestrate additional `FILE`,
`PREFIX`, or `NEAR` requests.

Handoff reuses the stable `badger api prompt --focus design` operation. The
result gives the receiving model repository topology and Badger's existing
`FILE`, `PREFIX`, and `NEAR` selection protocol for requesting focused context.
Handoff is a Design task whose input summarizes an existing session; there is
no separate handoff API or command.

## Installation

First install AI Badger and make the `badger` command available. An
independently installed skill requires that command and does not install it or
access the network automatically. See [the installation guide](../docs/install.md).

From a Badger binary, install or update both bundled official skills offline:

```bash
badger skills install
```

The command installs the definitions bundled with that binary at
`~/.agents/skills/badger-review/SKILL.md` and
`~/.agents/skills/badger-handoff/SKILL.md`. It preserves unrelated files and
other skills under `~/.agents/skills`.

The canonical definitions are also available directly in this repository at
[`skills/badger-review/SKILL.md`](badger-review/SKILL.md) and
[`skills/badger-handoff/SKILL.md`](badger-handoff/SKILL.md). This layout is a
portable source layout; support for every coding agent is not implied, and
this project does not claim to be a universal agent-skill installer.

The skills require an AI agent capable of following the instructions and
providing a caller-owned session summary. They do not silently install Badger,
publish to a catalog, or promise vendor-specific installation behavior.
