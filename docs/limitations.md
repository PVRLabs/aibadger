# Limitations

Badger is a context bridge, not an AI provider or autonomous coding agent.

## Current Constraints

- Extraction commands are intentionally simple: `FILE:`, `PREFIX:`, and `NEAR:`.
- Non-interactive review automation uses the stable `badger api review-context` operation.
- Binary and generated files are intentionally excluded or minimized to keep prompts compact.

## Scope Notes

The supported language detectors improve ranking for common project shapes, but Badger is designed to work across arbitrary repositories.

## Project Size and Topology

Badger is designed to work best with small and medium-size projects. It can
handle projects of arbitrary size, but topology becomes increasingly sparse as
the project grows. For a large repository or monorepo, run Badger from a
relevant subfolder such as a module or service to focus the context. Use
[External Context](settings.md#external-context) to connect related
directories when the work spans multiple subprojects.

## File Size Limits

Badger bounds file and prompt context to keep requests compact. For unusually
large repositories or source files, see [Configuring interactive limits](settings.md#configuring-interactive-limits)
for supported increase-only overrides.

## Report an Issue or Suggest an Improvement

If you run into a problem with Badger or want to suggest an improvement,
[file an issue](https://github.com/PVRLabs/aibadger/issues). For an
environment-dependent problem, include the complete output of:

```bash
badger diagnose
```

The diagnostic output is designed to be shareable and reports Badger and
development-tool versions and availability without analyzing project files or
inferring project requirements.
