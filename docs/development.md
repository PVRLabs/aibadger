# Development Notes

The public facade is:

```go
github.com/PVRLabs/aibadger/pkg/badger
```

Most implementation packages remain under `internal/` so the CLI and facade can evolve without exposing scanner, extractor, protocol, writer, or TUI internals as public API.

For release publishing and artifact details, see [releasing.md](releasing.md).
