# AIBadger Agent Guidance

## Scope and context

A `.badger-context` file may identify related read-only context directories.
Do not search them proactively. Consult only the specific files needed when the
user, task, or repository points there, or when this repository lacks required
information. Never include private context contents in public source or docs.

Prefer existing repository patterns and keep changes scoped to the request.

## Go and verification

Use the Go version declared in `go.mod`. The public facade is `pkg/badger`;
implementation code is under `internal/` and `cmd/badger`.

During iteration, prefer focused package or test runs. Before finishing a
change, format touched Go files and run verification appropriate to its scope;
use `go vet ./...` and `go test ./...` when the change affects the repository
broadly. Do not add project-local formatter or linter tooling unless asked.

Useful commands:

```bash
go build ./...
go test ./internal/scanner
go test ./internal/scanner -run TestName
gofmt -w <changed-go-files>
go vet ./...
```

## Artifacts and releases

Do not commit ignored local binaries (`/badger`, `/badger-release`, `*.test`,
`/bin/badger`), hand-edit release archives/checksums/installer trees, or change
release packaging metadata unless the task is a release or tap update.

Default builds are development builds. Release builds use the
`aibadger_release` tag and profiling builds use `aibadger_profile`; see
`docs/releasing.md` for release-specific details.

When a commit is tied to a named plan, include `Plan: <plan name>`.
