# Usage

AIBadger runs from the root of the project you want to inspect:

```bash
badger
```

## Workflow

1. Map: AIBadger scans your project and prepares Prompt 1: Topology.
2. Extract: You paste the AI's requested `FILE:`, `PREFIX:`, or `NEAR:` commands back into AIBadger, and AIBadger prepares Prompt 2: Code Context.
3. Apply: You paste the AI's final response, review the write plan, and confirm before files are written.

Prompt 1 contains project structure, file paths, and your goal, not source code. Prompt 2 contains selected source context based on the extraction commands you approve.

## Example Tasks

Type or paste one of these as the initial goal in AIBadger.

### Review A Change Before Committing

For small or focused changes, paste a review goal and diff as the initial task:

```text
Review my current change for bugs.

[Paste git diff here]
```

```text
Review my current change for bugs, performance issues, and missing tests.

[Paste git diff here]
```

### Find Bugs Or Performance Issues

```text
Look for correctness bugs, edge cases, and performance issues in the request extraction flow. Start from the relevant entrypoints and ask for only the files or spans needed.
```

### Troubleshoot Build Errors

```text
Help me fix this Maven test output.

mvn clean test
WARNING: java.lang.System::load has been called by org.fusesource.jansi.internal.JansiLoader
WARNING: Use --enable-native-access=ALL-UNNAMED to avoid a warning for callers in this module
WARNING: sun.misc.Unsafe::objectFieldOffset has been called by com.google.common.util.concurrent.AbstractFuture$UnsafeAtomicHelper
WARNING: sun.misc.Unsafe::objectFieldOffset will be removed in a future release
```

### Understand Unfamiliar Code

```text
Explain how authentication is wired in this project. Start with the main entrypoints and request only the files needed to trace the login flow.
```

### Make A Focused Implementation Request

```text
Add validation so empty project names are rejected before saving. Keep the change small and include any tests that should change.
```

## Commands

- `/help`: show the interactive command reference.
- `/review`: show diff-review guidance. This does not run git, read local diffs, copy to the clipboard, or enter a separate review mode.
- `/exit`: quit AIBadger.

## External Context

You can add read-only external directories by creating a `.badger-context` file in the project root, one path per line.

Example:

```text
../badger-sidecar/docs
```

See [privacy.md](privacy.md) for the read-only and safety rules around external context.

## Supported Projects

AIBadger includes first-class scanning for:

- Go
- Java
- JavaScript
- TypeScript
- Python

It also includes generic project scanning for common source and configuration files when a first-class detector does not apply.
