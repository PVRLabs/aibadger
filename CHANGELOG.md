# Changelog

Notable user-facing changes to Badger are documented here.

## Unreleased

## [v0.4.0] - 2026-08-11

### Highlights

- Added a topology-aware review workflow for preparing focused AI code-review requests from current Git changes.
- Added stable, non-interactive `review-context` and `review-continuation` API operations for editor, script, and coding-agent integrations.
- Restored and strengthened the interactive review experience, including clearer focus guidance and safer prompt budgeting.

### Review integrations

- `review-context` supports working-tree, staged, branch, and commit review modes, optional guidance, selected changed paths, configurable payload limits, and opt-in project topology.
- `review-continuation` accepts selector-only follow-up requests and returns supplemental current context without repeating the original diff or review envelope.
- Stable review APIs keep usable output on stdout and diagnostics on stderr, avoid clipboard, browser, provider, and network access, and use repository-relative paths in normal output.

### Safety and reliability

- Hardened fenced source and diff payloads against delimiter collisions and prompt-boundary confusion.
- Improved handling of untracked, binary, sensitive, oversized, deleted, and changing files while preserving the authoritative Git diff.
- Added deterministic validation, partial-success reporting, topology failure handling, and broader interactive and API test coverage.

### Integration note

- Integrations should detect supported API operations and flags from `badger api --help` and command-specific help. The earlier development-only headless review adapter has been removed in favor of the stable review APIs.
