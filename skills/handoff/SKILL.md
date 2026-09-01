---
name: handoff
description: Transfer the current coding, debugging, planning, or architecture session to AI Badger. Use for broad session continuation; use badger-review for an independent review.
---

# Badger Handoff

Create `.badger-handoff` in the current working directory, or another workspace
directory explicitly selected by the user. Do not discover a Git root.

Summarize the session concisely and with high information density. Preserve
enough context to continue without repeating investigation or misunderstanding
a decision. Give later decisions and corrections precedence.

Remove conversational noise, repetition, obsolete details, and information
Badger can recover from the repository. Preserve the current goal, material
progress, decisions and rationale, constraints and non-goals, completed work,
verification results, significant failed approaches, blockers, and immediate
next steps. Include exact details only when they are necessary for continuity.

Aim for no more than 2,000 output tokens under normal circumstances. Use less
for simple sessions. Exceed this target only when further compression would
discard continuation-critical information.

Use `mode: handoff` when repository work is part of the session. Use
`mode: design` only for reasoning where current worktree state is irrelevant.
Write this exact UTF-8 framing:

```text
BADGER-HANDOFF-V1
mode: handoff

<compact session summary>
```

Keep the complete file at or below 65,536 bytes. The body is conversation state
only: do not inspect or include repository files, diffs, Git state, or
topology. Badger collects repository context itself.

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
