# Releasing AIBadger

This document describes the current public OSS release process for AIBadger.

The current published release is `v0.1.1`. Future releases should follow the same
flow unless the release workflow changes.

## What Gets Released

Current release artifacts are built for:

- macOS `amd64`
- macOS `arm64`
- Linux `amd64`
- Linux `arm64`

Each artifact is published as a `.tar.gz` containing the `badger` binary and a
matching `.sha256` file.

## Before Releasing

1. Update the version constant in `internal/version/version.go`.
2. Update user-facing version references in the README if needed.
3. Run the test suite:

```bash
go test ./...
```

4. Build a release-mode binary locally if you want a quick smoke test:

```bash
go build -tags aibadger_release -ldflags="-s -w" -o badger ./cmd/badger
./badger --version
```

## Release Steps

1. Commit the version bump and any release notes changes.
2. Create a Git tag that matches the version, for example:

```bash
git tag v0.1.1
```

3. Push the tag to GitHub:

```bash
git push origin v0.1.1
```

4. Publish the GitHub Release for that tag.
5. Confirm the release workflow builds and uploads the release archives.

The release workflow lives in `.github/workflows/release.yml` and is triggered by
tag pushes and published releases.

## Public Availability

The public Homebrew tap lives at `https://github.com/PVRLabs/homebrew-aibadger`.
After a release is published, update the tap's `Formula/badger.rb` with the new
version and release checksums, then verify the public install path:

```bash
brew tap pvrlabs/aibadger
brew install pvrlabs/aibadger/badger
badger --version
```

## Verification Checklist

- The GitHub Release page exists for the new tag.
- All expected `.tar.gz` and `.sha256` assets are attached.
- Downloading an asset yields the expected binary archive.
- `./badger --version` reports the release version.
- The public Homebrew tap installs `badger` from GitHub Releases.

## Manual Fallback

If the release workflow is unavailable, build the archives locally with the same
release flags and upload them to the GitHub Release manually. Use the workflow
as the source of truth for the artifact names and supported platforms.
