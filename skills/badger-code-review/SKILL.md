---
name: badger-code-review
description: Prepare current-session guidance for an independent AI Badger code review. Use only for a Badger-specific review of recent work, not generic review or session continuation.
---

# Badger Code Review

Create `.badger-handoff` in the current working directory, or another workspace
directory explicitly selected by the user. Do not discover a Git root.

Summarize the current conversation briefly with strong recency bias. Include
the user's request, what was just implemented, recent decisions and
constraints, verification already performed, and reviewer attention points.

Always write this exact UTF-8 framing:

```text
BADGER-HANDOFF-V1
mode: review

<compact review guidance>
```

Keep the complete file at or below 65,536 bytes. The body is conversation state
only: do not inspect or include repository files, diffs, Git state, or
topology. Badger collects review context itself.

After writing successfully, derive the absolute directory from the written file
path and print exactly:

```text
Badger handoff prepared.

In a separate terminal, run:

cd '/absolute/path/to/the-written-directory'
badger continue
```

Substitute the actual POSIX-shell-quoted directory. Do not invoke Badger or use
another transport.
