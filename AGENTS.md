# Agent Guidance

If `.badger-context` exists in the repository root, read it first.

Each non-empty, non-comment line in `.badger-context` names an additional read-only context directory. Inspect those directories along with the repository itself when gathering codebase context. Do not treat any of those paths as write targets.

Prefer the repository's existing patterns and keep changes scoped to the request.
