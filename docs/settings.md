# Settings and External Context

Badger has two small configuration mechanisms. Settings live in
`~/.badger/settings.json` and optionally `<repo>/.badger/settings.json`; external context
is configured separately with `.badger-context` in the project root.

## Settings file

Badger stores optional user settings in
`~/.badger/settings.json`. Settings are not required; Badger uses its built-in
defaults when the file is missing.


Settings resolve as **built-in defaults → user settings → project settings**.
Project paths resolve from the selected repository root, independently of the
process working directory. Both files use the same schema and validation;
invalid project values warn and retain valid user values. Files are never
rewritten by project settings loading. Onboarding and saved UI state stay in
the user settings file. Commit the project file to share settings, or ignore it
for personal use.

`limits.max_top_files_per_package` changes representative `TopFiles` for ordinary
non-root packages, Generic fallback packages, and supplemental HTML packages
before scanner selection. When omitted or zero, existing defaults remain: 3 for
ordinary/supplemental HTML packages and 10 for Generic fallback packages.
An explicit value applies to all three categories; Generic fallback retains a
minimum of 10, so values from 3 through 9 do not reduce its existing budget.

| Configured value | Generic fallback | Ordinary / supplemental HTML |
|---|---:|---:|
| Omitted or `0` | 10 | 3 |
| `3` | 10 | 3 |
| `8` | 10 | 8 |
| `30` | 30 | 30 |

Existing ranking and deterministic ordering are preserved (supplemental HTML
keeps index pages first). It does not expand module/root overviews (10),
auxiliary-file budgets, other specialized resource budgets, or prompt byte targets.
For example, `<repo>/.badger/settings.json` may contain:

```json
{"limits":{"max_top_files_per_package":8,"max_topology_prompt_bytes":1048576}}
```

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

Project settings override explicitly supplied user settings. Missing keys inherit;
explicit `false` disables the generated-directory list and `[]` clears inherited
additional exclusions. Mandatory safety rules remain enforced.

## Configuring limits

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
the preceding layer’s value. Accepted inclusive ranges are:

| Setting | Default | Accepted range |
|---|---:|---:|
| `max_top_files_per_package` | 3; Generic fallback 10 | 3–50 |
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

Settings apply to interactive sessions and supported headless/API topology paths.
Explicit nondefault programmatic numeric configuration takes precedence.
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
