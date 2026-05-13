# AIBadger

AIBadger is a local context bridge for bringing codebase context to an AI chat. It scans a project on your machine, builds compact prompts, and helps apply AI-written file updates only after you review them.

It stays local by default: no telemetry, no file uploads, and no network connection required for normal use.

## Install

Install from the public Homebrew tap:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
```

For source builds, release builds, and version checks, see
[docs/install.md](docs/install.md).

## Usage

Run AIBadger from the root of the project you want to inspect:

```bash
badger
```

For the workflow and `.badger-context`, see [docs/usage.md](docs/usage.md).

## Privacy And Safety

See [docs/privacy.md](docs/privacy.md).

## Supported Projects

See [docs/usage.md](docs/usage.md) for the supported project model and
[docs/limitations.md](docs/limitations.md) for the current scan boundaries.

## Known Limits

See [docs/limitations.md](docs/limitations.md).

## Go Package

The public facade is `github.com/PVRLabs/aibadger/pkg/badger`.

For contributor-facing notes, see [docs/development.md](docs/development.md).
