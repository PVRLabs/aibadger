# Testing

Run the local test suite with `go test ./...` or `./run.sh test`.
Tests that create real Git repositories are skipped by default; scanner,
formatting, and other unit tests still run.

Git integration tests run automatically in GitHub CI for pull requests targeting
`main`, pushes to `main` (including merges), and manual workflow runs. To run them
locally, use:

```sh
BADGER_INTEGRATION=1 go test ./...
```

The separate CLI smoke suite remains opt-in with `BADGER_CLI_SMOKE=1` and runs
in CI as well.
