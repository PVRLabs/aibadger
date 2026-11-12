# Settings and External Context

Badger has two small configuration mechanisms. User-level settings live in
`~/.badger/settings.json`; project-specific external context
is configured separately with `.badger-context` in the project root.

## Settings file

Badger stores optional user settings in
`~/.badger/settings.json`. Settings are not required; Badger uses its built-in
defaults when the file is missing.

## Directory exclusions

Automatic discovery uses these optional keys in the existing
`~/.badger/settings.json` file:

```json
{
  "exclude_generated_directories": true,
  "additional_excluded_directories": [
    ".my-cache",
    "custom-output"
  ]
}
```

| Setting | Default | Behavior |
|---|---|---|
| `exclude_generated_directories` | `true` | Enables the built-in generated-directory list: tool caches, temporary directories, virtual environments, build output, and test reports. Set `false` to disable that list, including its automatic build/cache exclusions. |
| `additional_excluded_directories` | `[]` | Excludes these directory names independently of the generated-directory toggle. |

Each additional name matches a complete directory component at any depth,
case-insensitively. For example, `.my-cache` excludes both `.my-cache/` and
`packages/app/.MY-CACHE/`. Names must be nonempty single directory names;
paths, `.`/`..`, glob patterns, negation, and control characters are invalid.
These are literal names, with no regex or recursive ignore-file grammar.
Invalid values produce warnings. Valid fields and valid entries continue to
apply; malformed entries do not reset unrelated settings. Badger avoids
rewriting settings containing invalid entries.

To keep custom exclusions while allowing generated directories:

```json
{
  "exclude_generated_directories": false,
  "additional_excluded_directories": ["custom-output"]
}
```

Omitted keys retain defaults. Explicit `false` and an explicitly empty array
are preserved when saving. An empty array means no additional exclusions.

These are Badger-wide discovery settings. They apply before scanning to the
interactive CLI, headless/API topology, optional review topology, Codebase Map,
external-context summaries, automatic untracked review-file discovery, and path
completion. The VS Code extension
inherits them through Badger. Existing Codebase Map `--exclude` rules still apply
independently. Mandatory privacy, safety and symlink rules, and independently
required detector exclusions remain enforced. Deliberate file requests and
extraction can still select files omitted from automatic discovery, subject to
mandatory safety rules. Basename and suffix searches in external context retain
these discovery exclusions; use a root-relative or displayed full path to request
an excluded external file explicitly.

Only the user-level file is supported here. Project-local settings and precedence
are a separate feature; no settings file in the project is loaded.

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
| `max_prompt_two_bytes` | 512 KiB | 512 KiB–1 MiB |
| `max_topology_prompt_bytes` | 512 KiB | 512 KiB–2 MiB |

- An out-of-range number is ignored individually and produces a startup
  warning.
- If the `limits` object is malformed, Badger ignores all limit overrides but
  keeps valid top-level settings.
- If the settings file itself cannot be read or parsed, Badger ignores it and
  warns. It does not rewrite the file, so you can correct it manually.

A missing settings file is normal and does not produce a warning; first-run
onboarding is still controlled by the onboarding-completion setting.

The limits settings apply only to interactive sessions, not `badger api` commands.
The directory limit currently affects Node, C#, and generic fallback scanning;
Go, Java, and Python detectors keep their existing behavior. Prompt size limits are
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
