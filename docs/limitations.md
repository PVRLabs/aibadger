# Limitations

Badger is a context bridge, not an AI provider or autonomous coding agent.

## Current Constraints

- Extraction commands are intentionally simple: `FILE:`, `PREFIX:`, and `NEAR:`.
- Non-interactive review automation uses the stable `badger api review-context` operation.
- Binary and generated files are intentionally excluded or minimized to keep prompts compact.

## Scope Notes

The supported language detectors improve ranking for common project shapes, but Badger is designed to work across arbitrary repositories.
