# AIBadger Agent Guidance

## Scope and context

Prefer existing repository patterns and keep changes scoped to the request.

Use `repo-map` to locate related repositories only when work crosses repository
boundaries.

## Go and verification

Use the Go version declared in `go.mod`. The public facade is `pkg/badger`;
implementation code is under `internal/` and `cmd/badger`.

During iteration, prefer focused package or test runs. Before finishing a
change, format touched Go files and run verification appropriate to its scope.
For Go test execution, follow the `lite-tools` skill. Run `go vet ./...` when
the change affects the repository broadly. Do not add project-local formatter
or linter tooling unless asked.

Useful commands:

```bash
go build ./...
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
