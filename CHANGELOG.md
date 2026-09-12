# Changelog

Notable user-facing changes to Badger are documented here.

## Unreleased

## [v0.5.5] - 2026-09-11

### Documentation

- Expanded manual handoff guidance for corporate and restricted development
  environments where repository access must remain local and context sharing
  with an approved browser AI is explicit.
- Upgraded the project toolchain to Go 1.27.1; source builds now require that
  version or newer.
- Documented the macOS 13 minimum for release binaries in the installation
  guide.

## [v0.5.4] - 2026-09-09

### Review experience

- Added `Ctrl+R` in the interactive Review editor to refresh generated review
  context from the current Git state while preserving edited instructions,
  user-added attachments, review mode/ref, and default-mode selected paths.
  Failed refreshes leave the existing review context unchanged.

### Interactive settings

- Added optional `~/.badger/settings.json` limits for larger interactive
  repositories and source files: directory scan count, per-file context,
  Prompt 1 topology, and Prompt 2 size targets.
- Invalid values produce visible non-blocking startup warnings. Malformed
  settings are not automatically overwritten during the same session, and
  settings writes preserve existing known fields.

## [v0.5.3] - 2026-09-03

### Prompt delivery

- Added **Save to Downloads** for both normal and large Prompt 1 and Prompt 2
  delivery screens. It safely replaces a stable `badger-prompt.txt` and shows
  the path for direct upload while clipboard remains the Enter/default choice
  and clipboard failures continue to use the separate temp-file fallback.
- Clarified prompt-delivery choices with conventional `(Y/n)` wording and a
  subordinate `d` shortcut for Downloads, while retaining clipboard as the
  default handoff.
- This is especially useful for Gemini users who encounter its approximately
  30 KB pasted-content ceiling: Gemini Apps accepts supported file uploads up
  to 100 MB, so the Downloads artifact provides a direct upload path for
  prompts that are too large to paste. See [Google's Gemini Apps file upload
  guidance](https://support.google.com/gemini/answer/14903178).

### Review experience

- Refined review prompts and the default review task to focus on concrete
  bugs, edge cases, regressions, maintainability problems, and unintended
  behavior changes. Reviews now request concise findings or an explicit
  no-issues result, with brief directional recommendations where useful,
  instead of detailed patches unless requested.
- Improved handoff summaries to preserve continuation-critical context while
  removing conversational noise and repository-recoverable details.

### Offline behavior

- Removed automatic GitHub API requests and browser launching from the
  terminal UI. GitHub prompts are now static, offline-safe calls to action,
  including a star reminder after review and design responses.

## [v0.5.2] - 2026-08-27

### Fixes

- Bound binary review context to Git's compact binary summary by avoiding
  binary deltas and textconv output, preventing large binary changes from
  exceeding the review payload limit.

## [v0.5.1] - 2026-08-26

### Review integrations

- Added a basename-derived `[REPOSITORY: <label>]` marker to generated
  standalone, interactive, and supplemental repository review-context
  payloads, including optional attachments prepared for `mode: handoff`. The
  marker is bounded, included in payload byte accounting, and does not change
  handoff text, continuation selector/context semantics, or clean/non-Git
  fallback behavior.

## [v0.5.0] - 2026-08-21

### Highlights

- Added the official `handoff` and `badger-review` Agent Skills for continuing
  active sessions and requesting independent reviews with Badger.

### Installation

- Added offline Skill installation through `badger skills install` and public
  installation through the skills.sh ecosystem.

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
