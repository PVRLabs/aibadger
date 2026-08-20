# Changelog

Notable user-facing changes to Badger are documented here.

## Unreleased

## [v0.4.1] - 2026-08-19

### Highlights

- Deep Review and `review-context` in default working-tree mode now include bounded complete contents of eligible untracked files, instead of listing those paths only.
- Interactive review warns before copying when the prepared context includes sensitive paths.

### Safety and reliability

- Omit sensitive untracked paths and their contents from review payloads.
- Publish portable `.sha256` files for release archives so checksum verification does not depend on a `dist/` path prefix.

### Docs

- Linked the official VS Code companion from the README and install, API, and usage guides.

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
