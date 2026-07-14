# Releasing AI Badger

This document describes the public OSS release process for AI Badger.

Release tags use exact versions, such as `vX.Y.Z`. The `main` branch should
carry the next development version, such as `vX.Y.Z-dev`, so source builds are
clearly distinguishable from published release binaries.

## What Gets Released

Current release artifacts are built for:

- macOS `amd64`
- macOS `arm64`
- Linux `amd64`
- Linux `arm64`
- Windows `amd64`

Each artifact is published as a `.tar.gz` (macOS/Linux) or `.zip` (Windows)
containing the `badger` binary (or `badger.exe` on Windows) and a matching
`.sha256` file.

## Before Releasing

1. Replace the development version constant in `internal/version/version.go`
   with the exact release version.
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

Set the release version once:

```bash
RELEASE_VERSION=vX.Y.Z
```

1. Commit the version bump and any release notes changes.
2. Create a Git tag that matches the version:

```bash
git tag "${RELEASE_VERSION}"
```

3. Push the tag to GitHub:

```bash
git push origin "${RELEASE_VERSION}"
```

4. Publish the GitHub Release for that tag.
5. Confirm the release workflow builds and uploads the release archives.
6. After the release is public, bump `internal/version/version.go` on `main` to
   the next development version (e.g. `v0.2.8-dev` after releasing `v0.2.7`),
   commit, and push.
7. Update the Homebrew tap formula at `Formula/badger.rb` in the
   [`homebrew-tap`](https://github.com/PVRLabs/homebrew-tap) repo with
   the new version and release archive checksums, then push.

The release workflow lives in `.github/workflows/release.yml` and is triggered by
tag pushes and published releases.

## Public Availability

The public Homebrew tap lives at `https://github.com/PVRLabs/homebrew-tap`.
After a release is published, update the tap's `Formula/badger.rb` with the new
version and release checksums, then verify the public install path:

```bash
brew install pvrlabs/tap/badger
badger --version
```

Verify the curl installer against the new release:

```bash
tmp_dir="$(mktemp -d)"
curl -fsSL https://raw.githubusercontent.com/PVRLabs/aibadger/main/install.sh | BADGER_VERSION="${RELEASE_VERSION}" BADGER_INSTALL_DIR="${tmp_dir}" sh
"${tmp_dir}/badger" --version
```

## Verification Checklist

- The GitHub Release page exists for the new tag.
- All expected `.tar.gz`/`.zip` and `.sha256` assets are attached.
- Downloading an asset yields the expected binary archive.
- `./badger --version` reports the release version.
- Source builds from `main` after the release report the next `-dev` version.
- The shared public Homebrew tap installs `badger` from GitHub Releases.
- The curl installer downloads, verifies, and runs the release binary.

## Manual Fallback

If the release workflow is unavailable, build the archives locally with the same
release flags and upload them to the GitHub Release manually. Use the workflow
as the source of truth for the artifact names and supported platforms.
