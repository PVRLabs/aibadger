# Usage

AIBadger runs from the root of the project you want to inspect:

```bash
badger
```

## Walkthrough

This example traces a full session end-to-end using a code review task.

### Step 1: State your goal

```bash
cd your-project
badger
```

Type a goal and paste a git diff:

```text
Review my uncommitted changes for bugs, edge cases, and performance issues.

diff --git a/internal/tui/tui.go b/internal/tui/tui.go
index abc..def 100644
--- a/internal/tui/tui.go
+++ b/internal/tui/tui.go
@@ -42,6 +42,8 @@ func (m *Model) handleKeypress(key string) {
+    if key == "ctrl+c" {
+        return m.quit()
+    }
```

Press Enter.

### Step 2: Copy Prompt 1 (Topology) to your AI chat

AIBadger scans the project and shows **Prompt 1: Topology** — a compact map of the project's structure, key files, and your goal. Copy it and paste into your AI chat (Claude, ChatGPT, Gemini, etc.).

Prompt 1 contains file paths and structure only — your source code stays local.

### Step 3: Paste the AI's extraction commands back

The AI reads the topology and asks for specific files:

```text
To review the change, I need to see the TUI event handler:
FILE:internal/tui/tui.go
NEAR:internal/tui/tui.go#handleKeypress
```

Copy those lines, paste them into AIBadger, and press Enter. AIBadger fetches only those files and prepares **Prompt 2: Code Context**.

### Step 4: Copy Prompt 2 back to the AI chat

Copy Prompt 2 and paste it into your AI chat. The AI now has both the project structure and the actual source code. It responds with its analysis and any suggested changes.

### Step 5: Apply the AI's changes

Paste the AI's full response into AIBadger. AIBadger parses any file changes, shows you a write plan listing what will be written to each file, and asks for confirmation. Review the plan and confirm to apply.

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
