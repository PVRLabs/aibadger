# Usage

AIBadger runs from the root of the project you want to inspect:

```bash
badger
```

## Workflow

1. Map: AIBadger scans your project and prepares a topology prompt.
2. Extract: You paste the AI's requested file commands back into AIBadger, and AIBadger prepares selected source context.
3. Apply: You paste the AI's final response, review the write plan, and confirm before files are written.

Prompt 1 contains project structure and file paths, not source code. Prompt 2 contains selected source context based on the extraction commands you approve.

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
