# Privacy And Safety

AIBadger is local-first.

## Guarantees

- All scanning and extraction runs locally.
- No telemetry is collected.
- No cloud sync is used.
- No source code is copied until you approve the handoff.
- No file writes happen until you review the preview and confirm.

## Exclusions

Secret and credential locations such as `.env` and `.git` are excluded from scanning.

## Consent Model

AIBadger only copies content and applies file changes after explicit user confirmation.

## External Context

Read-only external directories can be listed in `.badger-context`.
They are summarized separately from the main project and cannot be used as patch targets.
