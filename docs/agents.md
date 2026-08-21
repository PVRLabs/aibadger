# Agent Integrations

AI Badger provides two official Agent Skills for common cross-session
workflows. `badger-handoff` continues work in another AI session, while
`badger-review` prepares a quick, one-shot independent review. Install both
offline with `badger skills install`; see the [official skills guide](../skills/README.md)
for behavior, limitations, privacy boundaries, and installation details.

For coding agents that already have repository tools, Badger also provides a
compact topology for initial orientation:

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
topology-based coding-agent primitive. The official handoff and review skills
are separate workflow integrations built on Badger's existing prompt and
review-context APIs. They use those APIs' native `--clipboard` option so the
calling agent reports only a short confirmation; paste the complete clipboard
package into the receiving AI session. See [Official Agent
Skills](../skills/README.md) for installation, behavior, and limitations.
