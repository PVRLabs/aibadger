---
name: badger-handoff
description: Prepare the current coding session for continuation in another AI chat or coding-agent session using AI Badger. Use when the user explicitly wants to hand off, transfer, export, or continue the current work in another AI session. Do not use for ordinary summaries, status reports, or continuing work in the current session.
---

# Badger Handoff

Prepare a portable continuation package for another AI session. The calling
agent understands the conversation; AI Badger understands the repository.

Do not invoke this skill for an ordinary session summary or when the user wants
to continue working in the current agent. The intended outcome must be
continuation in another AI session.

## Workflow

1. Resolve the repository root from the current session or environment. Use a
   known workspace/repository root when one is already available. Ask the user
   only if the root is ambiguous or cannot be determined safely. Do not read
   repository files or reconstruct repository context to resolve it.
2. Check that Badger is available with `badger --version`. If it is missing,
   stop and explain that AI Badger is required. Point the user to the official
   installation guide:
   https://github.com/PVRLabs/aibadger/blob/main/docs/install.md
   Never install Badger or access the network on the user's behalf.
3. Derive a concise handoff summary from the current conversation. Include
   only relevant state: the goal, current status, decisions and rationale,
   important constraints, unresolved questions, blockers or risks, and the
   exact next step.
4. Collect only shallow Git metadata for the work in progress. Run:

   ```bash
   git -C <repository> status --porcelain=v1 --branch
   ```

   Summarize the current branch (including detached HEAD when applicable) and
   each changed path/status entry. Include this shallow Git summary in the same
   temporary handoff input passed to Badger through `--input`; do not leave it
   only in command output or agent notes. If Git is unavailable or the root is
   not a Git repository, state that the shallow Git metadata is unavailable
   and continue when the Badger command can do so.

   Do not run or inspect `git diff`, `git show`, `git log`, `git blame`, or
   commands that read diffs, file contents, commit bodies, or broader history.
5. Keep the summary task-relevant and never reproduce credentials, tokens,
   passwords, private keys, `.env` values, or other secret values.
6. Use a private temporary file and delete only the file created by this
   workflow when finished.
7. Run the existing Design-focused prompt operation exactly once:

   ```bash
   badger api prompt \
     --root <repository> \
     --focus design \
     --input <handoff-summary-file> \
     --clipboard
   ```

   On success, report only that the complete handoff package is on the
   clipboard and can be pasted into another AI session. Preserve stderr for
   diagnostics and preserve the command's exit status. Do not reproduce the
   package, add a separate repository scan, or reconstruct Badger's prompt.

The receiving AI may use Badger's existing `FILE:`, `PREFIX:`, and `NEAR:`
selector protocol to request focused context in its new session. This remains
a Design task; the session summary is simply the input to that existing API.

This skill must not scan or read repository file contents, inspect diffs or
history, implement its own clipboard integration, enter the TUI, install
Badger, or contact an AI provider. If the command fails, report its stderr and
nonzero result without claiming a handoff was produced.
