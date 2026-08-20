---
name: badger-review
description: Prepare a quick, independent review of current-session work using the session's intent and AI Badger's authoritative Git and repository context. Use when a coding session needs one standalone review request; use the deeper interactive review workflow for additional context rounds.
---

# Badger Review

Prepare one standalone review package for the current coding session. The
calling agent understands the conversation; AI Badger understands the
repository and review Git context.

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
     --input <guidance-file>
   ```

   Do not add a second `FILE:`, `PREFIX:`, or `NEAR:` round and do not invoke
   `api review-continuation`.
7. Return the command's complete stdout as the one-shot portable review
   package. Preserve stderr for diagnostics and preserve the command's exit
   status. Do not add an agent-authored review envelope around the output.

AI Badger owns repository scanning, topology, authoritative review Git state,
supporting-file selection, untracked-file policy, privacy exclusions, limits,
and prompt construction. This skill must not scan or read repository file
contents, reconstruct review context, use the clipboard, enter the TUI, or
contact an AI provider.

If the command fails, report its stderr and nonzero result without claiming a
review package was produced. If a deeper or multi-round review is needed,
direct the user to the existing interactive Badger Review workflow.
