# Releasing AI Badger

This document describes the public OSS release process for AI Badger.

Release tags use exact versions, such as `vX.Y.Z`. Tag the release commit, not
the later development bump. After the release is public, `main` should carry
the next development version, such as `vX.Y.Z-dev`, so source builds are
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
`.sha256` file. Checksum files are portable: they name the archive itself, not
a `dist/` path.

Do not commit ignored local binaries (`/badger`, `/badger-release`, `*.test`,
`/bin/badger`).

## Before Releasing

Set the release version once and reuse it:

```bash
RELEASE_VERSION=vX.Y.Z
```

1. Write user-facing notes under `## Unreleased` in `CHANGELOG.md` if that
   section is still empty.
2. Replace the development version constant in `internal/version/version.go`
   with the exact release version (`vX.Y.Z`, not `-dev`).
3. Move the `Unreleased` notes to a dated heading that matches the tag, and
   leave an empty `## Unreleased` section for the next cycle:

```markdown
## Unreleased

## [vX.Y.Z] - YYYY-MM-DD
```

   The heading form `## [vX.Y.Z]` or `## [vX.Y.Z] - date` is required. The
   release workflow reads that changelog section for GitHub Release notes and
   appends the comparison link. README does not currently pin a Badger version.
4. Run the test suite:

```bash
gofmt -w internal/version/version.go
go vet ./...
go-lite test ./...
```

   Use `go-lite --full test ./...` when complete live test output is needed.
5. Build a release-mode binary locally if you want a smoke test. It must print
   the exact release version, with no `-dev` suffix:

```bash
go build -tags aibadger_release -ldflags="-s -w" -o badger ./cmd/badger
./badger --version
```

## Release Steps

1. Commit the version bump and changelog on `main`.
2. Push that commit, then tag that exact commit and push the tag:

```bash
git push origin main
git tag "${RELEASE_VERSION}"
git push origin "${RELEASE_VERSION}"
```

   Pushing the `v*` tag is what starts `.github/workflows/release.yml`. That
   workflow builds the archives and **creates** the GitHub Release, including
   notes from `CHANGELOG.md`. Do not run `gh release create` or publish a
   release by hand first; that races the workflow and can produce an empty
   release.

3. Wait until **this tag's** workflow succeeds and the GitHub Release has all
   assets. Select the run by tag name (`headBranch` is the `v*` tag), not by
   "latest release.yml run":

```bash
release_sha="$(git rev-parse "${RELEASE_VERSION}^{commit}")"
run_id=""
for _ in 1 2 3 4 5 6 7 8 9 10; do
  run_id="$(gh run list \
    --workflow=release.yml \
    --branch "${RELEASE_VERSION}" \
    --commit "${release_sha}" \
    --limit 1 \
    --json databaseId \
    --jq '.[0].databaseId')"
  [ -n "${run_id}" ] && break
  sleep 2
done
[ -n "${run_id}" ] || { echo "no release.yml run for ${RELEASE_VERSION}" >&2; exit 1; }
gh run watch "${run_id}" --exit-status
gh release view "${RELEASE_VERSION}"
```

   Confirm five archives and five `.sha256` files are attached, and that each
   checksum file names the archive (for example
   `badger_X.Y.Z_linux_amd64.tar.gz`), not `dist/...`.
4. After the release is public, bump `internal/version/version.go` on `main` to
   the next development version (for example `v0.4.2-dev` after releasing
   `v0.4.1`), commit, and push. Do not move the release tag to this commit.
5. Update the published Homebrew formula in
   [`homebrew-tap`](https://github.com/PVRLabs/homebrew-tap)
   (`Formula/badger.rb` **in that repository**), then push. Copy the hash field
   from the GitHub Release `.sha256` files. The tap ships macOS and Linux
   only; Windows is GitHub Releases and the PowerShell installer.

   This repository has no Homebrew formula. If `repo-map get homebrew-tap` is
   registered, use that checkout; otherwise clone
   `https://github.com/PVRLabs/homebrew-tap`.

The release workflow is triggered only by pushing tags that match `v*`. It is
not triggered by publishing a GitHub Release.

## Public Availability

The public Homebrew tap lives at `https://github.com/PVRLabs/homebrew-tap`.
After the tap formula is updated, verify:

```bash
brew update
brew install pvrlabs/tap/badger
# or: brew upgrade pvrlabs/tap/badger
badger --version
```

Verify the curl installer against the new release in an isolated `HOME` so it
cannot rewrite your real `~/.bashrc` / `~/.zshrc` or replace
`~/.local/bin/badger`. `BADGER_INSTALL_DIR` alone is not enough: the installer
may still symlink into `~/.local/bin` and append a PATH block to your shell
rc.

```bash
work="$(mktemp -d)"
HOME="${work}" curl -fsSL "https://raw.githubusercontent.com/PVRLabs/aibadger/${RELEASE_VERSION}/install.sh" \
  | HOME="${work}" BADGER_VERSION="${RELEASE_VERSION}" BADGER_INSTALL_DIR="${work}/bin" sh
"${work}/bin/badger" --version
rm -rf "${work}"
```

That should print `badger vX.Y.Z`.

## Verification Checklist

- The GitHub Release page exists for the new tag and was created by the
  release workflow, not a manual `gh release create`.
- All expected `.tar.gz`/`.zip` and `.sha256` assets are attached.
- Downloading an asset yields the expected binary archive.
- Checksum files are portable (archive basename, not a `dist/` prefix).
- A release-mode `./badger --version` reports the exact release version.
- Source builds from `main` after the release report the next `-dev` version.
- The release tag still points at the exact-version commit, not the `-dev`
  bump.
- The shared public Homebrew tap installs `badger` from GitHub Releases.
- The curl installer, run with an isolated `HOME`, downloads, verifies, and
  runs the release binary.

## Manual Fallback

If the release workflow is unavailable, build the archives locally with the
same release flags and upload them to the GitHub Release manually. Use the
workflow as the source of truth for artifact names, supported platforms, and
portable `.sha256` files:

```bash
(cd dist && sha256sum "${archive_name}" > "${archive_name}.sha256")
```
