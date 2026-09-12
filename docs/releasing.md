# Releasing AI Badger

This document describes the public OSS release process for AI Badger.

Release tags use exact versions, such as `vX.Y.Z`. Tag the release commit, not
the later development bump. After the release is public, `main` should carry
the next development version, such as `vX.Y.Z-dev`, so source builds are
clearly distinguishable from published release binaries.

## Process Design

Keep this document as the high-level orchestrator for the complete release. It
should preserve the operator-visible sequence, decision points, verification,
and recovery guidance without becoming a second automation system.

Move self-contained, deterministic groups of release work into repository-local
GitHub Actions workflows when doing so makes the process shorter and more
reliable. Keep human judgment and coordination here, and avoid adding a central
workflow, cross-repository credentials, or other orchestration machinery solely
to eliminate a few clear steps from this runbook.

## What Gets Released

Current release artifacts are built for:

- macOS 13 Ventura or newer, `amd64`
- macOS 13 Ventura or newer, `arm64`
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
2. Push that commit and confirm the ordinary `main` CI run for the release
   commit succeeds:

```bash
git push origin main
release_sha="$(git rev-parse HEAD)"
ci_run_id=""
for _ in {1..30}; do
  ci_run_id="$(gh run list \
    --workflow=ci.yml \
    --branch main \
    --commit "${release_sha}" \
    --event push \
    --limit 1 \
    --json databaseId \
    --jq '.[0].databaseId')"
  [ -n "${ci_run_id}" ] && break
  sleep 2
done
[ -n "${ci_run_id}" ] || { echo "no ci.yml run for ${release_sha}" >&2; exit 1; }
gh run watch "${ci_run_id}" --exit-status
```

3. Record the previous release-workflow run, dispatch the workflow from `main`,
   then wait for the newly created run and the GitHub Release:

```bash
previous_run_id="$(gh run list \
  --workflow=release.yml \
  --branch main \
  --event workflow_dispatch \
  --limit 1 \
  --json databaseId \
  --jq '.[0].databaseId')"
gh workflow run release.yml \
  --repo PVRLabs/aibadger \
  --ref main \
  -f version="${RELEASE_VERSION}"

run_id=""
for _ in {1..30}; do
  candidate_run_id="$(gh run list \
    --workflow=release.yml \
    --branch main \
    --event workflow_dispatch \
    --limit 1 \
    --json databaseId \
    --jq '.[0].databaseId')"
  if [ -n "${candidate_run_id}" ] && [ "${candidate_run_id}" != "${previous_run_id}" ]; then
    run_id="${candidate_run_id}"
    break
  fi
  sleep 2
done
[ -n "${run_id}" ] || { echo "no release.yml run for ${RELEASE_VERSION}" >&2; exit 1; }
gh run watch "${run_id}" --exit-status
gh release view "${RELEASE_VERSION}"
```

   The workflow validates the prepared commit, creates the exact `v*` tag,
   builds the archives, and **creates** the GitHub Release, including notes
   from `CHANGELOG.md`. Do not run `git tag`, `gh release create`, or publish a
   release by hand first.

   Confirm five archives and five `.sha256` files are attached, and that each
   checksum file names the archive (for example
   `badger_X.Y.Z_linux_amd64.tar.gz`), not `dist/...`.
4. After the release is public, bump `internal/version/version.go` on `main` to
   the next development version (for example `v0.4.2-dev` after releasing
   `v0.4.1`), commit it, then push and confirm ordinary `main` CI. Do not move
   the release tag to this commit:

```bash
git push origin main
dev_sha="$(git rev-parse HEAD)"
dev_ci_run_id=""
for _ in {1..30}; do
  dev_ci_run_id="$(gh run list \
    --workflow=ci.yml \
    --branch main \
    --commit "${dev_sha}" \
    --event push \
    --limit 1 \
    --json databaseId \
    --jq '.[0].databaseId')"
  [ -n "${dev_ci_run_id}" ] && break
  sleep 2
done
[ -n "${dev_ci_run_id}" ] || { echo "no ci.yml run for ${dev_sha}" >&2; exit 1; }
gh run watch "${dev_ci_run_id}" --exit-status
```

5. Dispatch the Homebrew tap workflow for Badger after the GitHub Release is
   public:

```bash
previous_tap_run_id="$(gh run list \
  --repo PVRLabs/homebrew-tap \
  --workflow=update-formula.yml \
  --branch main \
  --event workflow_dispatch \
  --limit 1 \
  --json databaseId \
  --jq '.[0].databaseId')"
gh workflow run update-formula.yml \
  --repo PVRLabs/homebrew-tap \
  --ref main \
  -f formula=badger \
  -f version="${RELEASE_VERSION}"

tap_run_id=""
for _ in {1..30}; do
  candidate_tap_run_id="$(gh run list \
    --repo PVRLabs/homebrew-tap \
    --workflow=update-formula.yml \
    --branch main \
    --event workflow_dispatch \
    --limit 1 \
    --json databaseId \
    --jq '.[0].databaseId')"
  if [ -n "${candidate_tap_run_id}" ] && [ "${candidate_tap_run_id}" != "${previous_tap_run_id}" ]; then
    tap_run_id="${candidate_tap_run_id}"
    break
  fi
  sleep 2
done
[ -n "${tap_run_id}" ] || { echo "no update-formula.yml run found" >&2; exit 1; }
gh run watch --repo PVRLabs/homebrew-tap "${tap_run_id}" --exit-status
```

   The tap workflow downloads and validates the four macOS/Linux checksums,
   updates `Formula/badger.rb`, and commits directly to the tap. It also
   supports `formula=statlite` for Statlite releases. The tap ships macOS and
   Linux only; Windows is GitHub Releases and the PowerShell installer.

6. Post a structured announcement in the repository's [Announcements
   discussion category](https://github.com/PVRLabs/aibadger/discussions/categories/announcements)
   after the release is public and the main distribution paths are updated.
   Use the changelog and release assets as the source of truth. Keep the post
   focused on user-visible changes and write it in this order:

   - Title: `AI Badger ${RELEASE_VERSION}: <two concise release themes>`
   - Short opening paragraph announcing the release and its overall benefit.
   - One `##` section per major feature area, with a short explanation and
     concrete behavior or configuration details.
   - `## Install or update` with links to the GitHub Release, relevant docs,
     Homebrew commands, and the tagged curl installer when applicable.
   - A brief closing invitation for feedback, discussions, or bug reports.

   For example, the v0.5.4 announcement used the themes **refreshable
   reviews** and **configurable limits**, then included the four supported
   settings and their accepted ranges, followed by GitHub Release, Homebrew,
   and curl installation instructions. Do not announce unreleased or
   speculative work, and link to the exact `${RELEASE_VERSION}` tag rather
   than a moving `latest` URL when documenting release-specific behavior.

The release workflow is manually dispatched from `main`; it is not triggered
by pushing a tag or publishing a GitHub Release. The Homebrew updater is a
separate manually dispatched workflow in `homebrew-tap`.

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

If the Homebrew workflow is unavailable, update the selected formula manually
in `homebrew-tap` using the published `.sha256` files, then commit and push the
narrow formula change. The normal workflow remains the preferred path.
