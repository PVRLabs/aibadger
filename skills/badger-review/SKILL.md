---
name: badger-review
description: Prepare current-session work as an AI Badger review package for an independent AI reviewer. Use when the user explicitly asks to hand off a review to another AI or chat, asks for a Badger review package, or asks to copy review context for external review. Do not use for ordinary code review, bug finding, or requests for the current agent to inspect changes itself.
---

# Badger Review

Prepare one standalone review package for the current coding session. The
calling agent understands the conversation; AI Badger understands the
repository and review Git context.

Do not invoke this skill merely because the user asks to review code, find
bugs, find serious issues, inspect changes, or assess correctness. In those
cases, the current agent should perform the review normally with its native
repository tools. Use this skill only when the requested outcome is a
portable Badger review package for another AI or reviewer.

Typical triggers include “prepare this for independent review,” “create a
Badger review package,” or “send these changes to another reviewer.” Requests
such as “review this code,” “find bugs,” or “check my changes” are ordinary
reviews and must not invoke this skill.

## Workflow

1. Resolve the repository root from the current session or environment. Use a
   known workspace/repository root when one is already available. Ask the user
   only if the root is ambiguous or cannot be determined safely. Do not inspect
   repository files or independently inspect Git state to resolve it.
2. Check that Badger is available with `badger --version`. If it is missing,
   stop and explain that AI Badger is required. Point the user to the official
   installation guide:
   https://github.com/PVRLabs/aibadger/blob/main/docs/install.md
   Never install Badger or access the network on the user's behalf.
3. Derive a concise guidance summary from the current conversation. Include
   only review-relevant information: the objective, requirements,
   constraints, design decisions, intentional tradeoffs, explicit non-goals,
   implementation approach, and tests or verification already performed.
4. Keep the summary task-relevant and never reproduce credentials, tokens,
   passwords, private keys, `.env` values, or other secret values.
5. Use a private temporary file and delete only the file created by this
   workflow when finished.
6. Run exactly one review-context request:

   ```bash
   badger api review-context \
     --root <repository> \
     --input <guidance-file> \
     --clipboard
   ```

   Do not add a second `FILE:`, `PREFIX:`, or `NEAR:` round and do not invoke
   `api review-continuation`.
7. On success, report only that the complete one-shot review package is on the
   clipboard and can be pasted into another AI reviewer. Preserve stderr for
   diagnostics and preserve the command's exit status. Do not reproduce the
   package or add an agent-authored review envelope around it.

AI Badger owns repository scanning, topology, authoritative review Git state,
supporting-file selection, untracked-file policy, privacy exclusions, limits,
prompt construction, and native clipboard delivery. This skill must not scan
or read repository file contents, reconstruct review context, implement its own
clipboard integration, enter the TUI, or contact an AI provider.

If the command fails, report its stderr and nonzero result without claiming a
review package was produced. If a deeper or multi-round review is needed,
direct the user to the existing interactive Badger Review workflow.
