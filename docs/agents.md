# Agent Integrations

AI Badger remains primarily a human-facing tool for preparing repository
context for an external AI chat. For coding agents that already have repository
tools, Badger provides a compact topology for initial orientation:

```bash
badger api topology --root .
```

Topology can reveal likely entrypoints, packages, tests, configuration, and
documentation before an agent starts broad recursive exploration. It is a map
of the repository, not authoritative source code and not a replacement for the
agent's native tools.

## When to use Badger

Use topology when:

- the repository is unfamiliar;
- the repository contains multiple modules or packages;
- the task spans several areas;
- the relevant entrypoint is unclear;
- the user asks for architecture or implementation planning; or
- broad recursive exploration would otherwise be required.

Skip topology when:

- the user identifies the exact file or function;
- the repository is very small;
- the relevant source area is already known;
- topology has already been generated during the current task; or
- the request is a narrow follow-up to work already in progress.

Run topology near the beginning of a task, use it to choose promising source
areas, and then use native search, file-reading, editing, and testing tools.
Avoid rerunning it unless repository structure has materially changed. If
Badger is unavailable or fails, continue with native repository exploration.

The stable [`api topology`](api.md#api-topology) operation is the current
coding-agent primitive. The `prompt` and `extract` operations produce the two
prompts used by Badger's human AI-chat workflow; agents with direct repository
access do not normally need them.

For session transfer or an independent review, install Badger's two official
producers with:

```bash
badger skills install
```

This offline command writes `handoff` and `badger-review` to
`~/.agents/skills/` and preserves unrelated Skills and files. The producers
write only compact conversation/session context to `.badger-handoff`; they do
not inspect repositories, collect Git state, invoke Badger, or use a
clipboard. After a Skill reports success, run `badger continue` in a separate
terminal from the written file's directory. Badger independently collects
repository context for `review` and `handoff` modes.
