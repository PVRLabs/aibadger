# Agent Guidance

If `.badger-context` exists in the repository root, read it first.

Each non-empty, non-comment line in `.badger-context` names an additional read-only context directory. Inspect those directories along with the repository itself when gathering codebase context. Do not treat any of those paths as write targets.

Prefer the repository's existing patterns and keep changes scoped to the request.

## Toolchain

- Use the Go version declared by `go.mod`.
- Module: `github.com/PVRLabs/aibadger`
- Public facade: `pkg/badger`. Implementation lives under `internal/` and `cmd/badger`.

## Build and test

```bash
go build ./...
go build -o badger ./cmd/badger
go test ./...
go test ./internal/scanner
go test ./internal/scanner -run TestName
```

Default `go build` is a development binary. Prefer package- or test-scoped runs while iterating; run `go test ./...` before finishing a change set.

## Verification

```bash
gofmt -w <changed-go-files>
go vet ./...
```

Format only files you touch. Do not add project-local linter or formatter tooling unless asked.

## Artifacts

- Do not commit local binaries listed in `.gitignore` (`/badger`, `/badger-release`, `*.test`, `/bin/badger`).
- Do not hand-edit release archives, checksums, or installer-produced install trees.
- Leave release packaging metadata alone unless the task is a release or tap update.
- App version: `internal/version/version.go` (`*-dev` suffix on `main`).

## Build modes

Default builds enable development-only flags. Release builds use the `aibadger_release` tag and reject those flags. See `docs/install.md` and `docs/releasing.md` for release flags, profiling builds, and publish steps.

## Commit metadata

- When a commit is tied to a named plan, include a trailer of the form `Plan: <plan name>`.

## Agent-Friendly CLI Usage

Prefer low-noise tools when available on `PATH`.

- If a command is excessively noisy, misleading, hard to parse, or otherwise
  agent-unfriendly, report it with `agent-complaint`.
- Do not run extra commands just to collect profiling data.
- Do not include secrets, source code, sensitive paths, or large output in
  complaints.
- Run `agent-complaint --help` for usage.
