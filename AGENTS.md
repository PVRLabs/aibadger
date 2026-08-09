# Agent Guidance

A `.badger-context` file may list related read-only context directories used by
AI Badger.

Do not proactively read, inventory, or recursively search those directories.
Consult a listed directory only when:

- the user explicitly references material in it;
- a repository file or task points to a specific document there; or
- the current task requires information that cannot be found in this repository.

When external context is needed, read only the specific files relevant to the
task. Treat all listed directories as read-only and do not include their
contents in public documentation or source files.

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

Default builds identify themselves as development builds. Release builds use the `aibadger_release` tag, and profiling builds use `aibadger_profile`. See `docs/install.md` and `docs/releasing.md` for build flags, profiling, and publish steps.

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
