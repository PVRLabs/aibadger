# Usage

Badger runs from the root of the project you want to inspect:

```bash
badger
```

## Walkthrough

This example traces a full session end-to-end using `badger review`.
For a product-level explanation of what moves between Badger and the browser,
see the [Browser Handoff Guide](handoff.md).

### Step 1: Start a review

```bash
cd your-project
badger review
```

Badger opens your editor with a prompt pre-filled from the current Git working tree. Review it, make any tweaks, then save and close to submit.

> [!NOTE]
> Default review includes staged and unstaged tracked changes plus up to 25 relevant Git-untracked paths in a separate section. An untracked-only review is valid. Untracked file contents and unchanged source code stay local until you paste extraction commands.
> Large pasted review context may be preserved as a separate removable attachment so the editor stays focused on your instruction.

### Step 2: Copy Prompt 1 (Map) to your AI chat

Badger scans the project and shows **Prompt 1 (Map)** — a compact map of the project's structure, key files, and your goal. Copy it and paste into your AI chat (Claude, ChatGPT, Gemini, etc.).

> [!NOTE]
> Prompt 1 contains file paths and structure only — your source code stays local.

### Step 3: Paste the AI's extraction commands back

The AI reads the topology and replies with the files it needs:

```text
FILE:internal/tui/tui.go
NEAR:internal/tui/tui.go#handleKeypress
```

Copy those lines, paste them into Badger, and press Enter. Badger fetches only those files and prepares **Prompt 2 (Code Context)**.

### Step 4: Copy Prompt 2 back to the AI chat

Copy Prompt 2 and paste it into your AI chat. The AI now has both the project structure and the actual source code. It responds with its analysis and any suggested changes.

### Step 5: Apply the AI's changes

Paste the AI's full response into Badger. Badger parses any file changes, shows you a write plan listing what will be written to each file, and asks for confirmation. Review the plan and confirm to apply.

## Manual mode

If you're not doing a code review, run `badger` without arguments:

```bash
cd your-project
badger
```

Type a goal at the prompt. You can also paste a git diff or other supporting text if you want to provide details of the change; large pasted context may be kept as a removable attachment so the goal stays readable.

> [!TIP]
> Badger never reads your source code ahead of time — you explicitly provide context (like a diff or error output) so only what's needed leaves your machine.

If you want Prompt 1 to include a specific file, type a tagged reference like `@docs/usage.md` in the goal input. Press `Tab` to complete `@` references from the shallow file list. Tagged references also resolve against [external context](#external-context) directories when no local file exists.

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

Press Enter. Then follow Steps 2-5 above.

## Example Tasks

Type or paste one of these as the initial goal in Badger.

### Find Bugs Or Performance Issues

```text
Look for correctness bugs, edge cases, and performance issues in the request extraction flow.
Start from the relevant entrypoints and ask for only the files or spans needed.
```

### Design A Feature

```bash
badger design
```

```text
Design a caching layer for the API client. Focus on the interface, eviction policy,
and concurrency model. See @docs/api-spec.md for the current client contract.
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
Explain how authentication is wired in this project.
Start with the main entrypoints and request only the files needed to trace the login flow.
```

### Make A Focused Implementation Request

```text
Add validation so empty project names are rejected before saving.
Keep the change small and include any tests that should change.
```

## Commands

- `/help`: show the interactive command reference.
- `/review`: seed an editable review prompt from the current Git working tree. It reuses the same review flow as `badger review`.
- `/design`: switch the active focus to Design. The active focus appears in the status bar as `Focus: Design` and the prompt seeds a short, conversational brainstorm.
- `/followup`: switch the active focus to Follow-up. The active focus appears in the status bar as `Focus: Follow-up` and the prompt seeds a short follow-up framing.
- `/exit`: quit Badger.

To start in a specific focus from the command line, pass the focus name as the
first argument:

```bash
badger            # Code focus (default)
badger design     # Design focus — prompt seeds a short, conversational brainstorm
badger review     # Review focus — prompt is prefilled from the current Git working tree
badger followup   # Follow-up focus — prompt seeds a short follow-up framing
```

### Review Options

`badger review` accepts these flags to control the diff source:

```bash
badger review                        # unstaged + staged tracked changes + untracked paths
badger review --staged               # staged changes only
badger review --branch <ref>         # changes since branching off <ref>
badger review --commit <sha>         # a single commit
```

Flags are mutually exclusive. When `--staged`, `--branch`, or `--commit` is used, working-tree untracked files are excluded.

## Attachments

When you paste a git diff, error output, or other supporting text into
Badger, it is preserved as a **removable attachment** so the goal input
stays clean and focused. In default mode, the `badger review` command
attaches staged and unstaged tracked changes and lists up to 25 relevant
Git-untracked paths separately without including their contents. Relevant
untracked paths alone are enough to start a review. See [Review Options](#review-options) for how `--staged`, `--branch`, and `--commit` affect attachment behavior. Text pastes exceeding
16KB or 40 lines are automatically
converted into attachments.

Press **Tab** to switch focus between the goal editor and the attachment
list, then use the **arrow keys** to cycle through attachments — the focused
attachment's details are shown inline, and you can remove it with
**Backspace** or **Delete** before submitting.

## External Context

You can add read-only external directories by creating a `.badger-context` file in the project root, one path per line. Paths are relative to the `.badger-context` file location.

Example:

```text
../shared/docs
```

See [privacy.md](privacy.md) for the read-only and safety rules around external context.

## Supported Projects

Badger includes first-class scanning for:

- Go
- Java
- JavaScript
- TypeScript
- Python

It also includes generic project scanning for common source and configuration files when a first-class detector does not apply.
