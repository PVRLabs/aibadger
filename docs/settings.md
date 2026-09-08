# Settings and External Context

Badger has two small configuration mechanisms. User-level interactive
settings live in `~/.badger/settings.json`; project-specific external context
is configured separately with `.badger-context` in the project root.

## Settings file

Badger stores optional interactive-session settings in
`~/.badger/settings.json`. Settings are not required; Badger uses its built-in
defaults when the file is missing.

## Configuring interactive limits

For unusually large repositories or source files, add a `limits` object to
your existing `~/.badger/settings.json`. Preserve any other settings already
in the file.

```json
{
  "limits": {
    "max_files_per_directory": 1000,
    "max_context_file_bytes": 131072,
    "max_topology_prompt_bytes": 1048576,
    "max_prompt_two_bytes": 524288
  }
}
```

The optional limits are increase-only. Omit a key or set it to `0` to keep
Badger's normal value. Accepted inclusive ranges are:

| Setting | Default | Accepted range |
|---|---:|---:|
| `max_files_per_directory` | 250 | 250–5,000 |
| `max_context_file_bytes` | 50 KiB | 50–512 KiB |
| `max_prompt_two_bytes` | 192 KiB | 192 KiB–1 MiB |
| `max_topology_prompt_bytes` | 512 KiB | 512 KiB–2 MiB |

- An out-of-range number is ignored individually and produces a startup
  warning.
- If the `limits` object is malformed, Badger ignores all limit overrides but
  keeps valid top-level settings.
- If the settings file itself cannot be read or parsed, Badger ignores it and
  warns. It does not rewrite the file, so you can correct it manually.

A missing settings file is normal and does not produce a warning; first-run
onboarding is still controlled by the onboarding-completion setting.

These settings apply only to interactive sessions, not `badger api` commands.
The directory limit currently affects Node and generic fallback scanning; Go,
Java, and Python detectors keep their existing behavior. Prompt size limits are
targets rather than strict output-size guarantees. Required framing, task, and
instruction text is never removed just to meet the limit. The context-file
setting limits how much extracted content is retained; it does not change
extraction I/O behavior.

## External Context

You can add read-only external directories by creating a `.badger-context`
file in the project root, one path per line. Paths are relative to the
`.badger-context` file location.

Example:

```text
../shared/docs
```

External directories are summarized separately from the main project and
cannot be used as patch targets. See [privacy.md](privacy.md) for the
read-only and safety rules around external context.
