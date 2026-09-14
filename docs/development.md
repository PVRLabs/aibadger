# Development Notes

The public facade is:

```go
github.com/PVRLabs/aibadger/pkg/badger
```

Most implementation packages remain under `internal/` so the CLI and facade can evolve without exposing scanner, extractor, protocol, writer, or TUI internals as public API.

For release publishing and artifact details, see [releasing.md](releasing.md).

## CLI smoke tests

The opt-in smoke suite builds and invokes the real `badger` executable against
an isolated temporary Git fixture. Run it locally with:

```bash
BADGER_CLI_SMOKE=1 go test ./tests/cli -run '^TestCLISmoke$' -count=1
```

The ordinary `go test ./...` command compiles the smoke package but leaves its
external-process scenarios skipped.
