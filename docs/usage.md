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

Badger loads the generated review context as a removable attachment and leaves
the editor available for optional guidance. Add any focus you want, or submit
the review as-is.

> [!NOTE]
> Default review includes the complete staged and unstaged tracked diff,
> bounded complete copies of eligible changed text files, and up to 25 relevant
> Git-untracked paths in a separate section. An untracked-only review is valid.
> Untracked file contents are never included automatically.

### Step 2: Copy the review prompt to your AI chat

Badger prepares a prompt containing the authoritative Git diff, explicit
status for files represented by the diff only, any eligible supporting file
context that fits, and compact project topology. Copy it and paste it into your
AI chat (Claude, ChatGPT, Gemini, etc.).

> [!NOTE]
> Nothing is copied until you confirm the clipboard action. Binary, sensitive,
> deleted, oversized, unavailable, and budget-excluded files remain diff-only
> with an explanatory status.

### Step 3: Read the findings or request more context

The initial prompt asks the AI to report findings immediately when the supplied
context is sufficient. A findings or no-issues response completes the review;
there is no mandatory second step.

If additional unchanged context is needed, the AI may instead reply only with
selectors such as:

```text
FILE:internal/tui/tui.go
NEAR:internal/tui/tui.go#handleKeypress
```

Copy selector-only lines back into Badger and press Enter. Badger fetches the
requested current files and prepares supplemental review context without
repeating the initial diff.

### Step 4: Copy optional supplemental context

Copy the supplemental context into the same AI conversation. The files are
read at continuation time and may be newer than the initial review context.
The AI then reports findings, risks, or a clear no-issues result.

> [!NOTE]
> Most prompts can be copied directly. When Prompt 2 is unusually large (≈128 KB or more), Badger shows a delivery menu with clipboard, temp-file, and manual-copy options. Clipboard is recommended. Saving to a temp file is available when you prefer to attach the file rather than paste it.

## Continue from another AI coding session

An external coding agent can leave a short session summary for Badger in a
workspace-local `.badger-handoff` file. From a separate terminal, invoke
`badger continue` in that same directory:

```bash
cd '/path/to/your-project'
badger continue
```

The file is consumed only by this explicit command. Its body is conversation
or session context, not a repository snapshot. Keep diffs, Git status,
repository topology, source files, and untracked-file contents out of the
body; Badger collects current repository context itself when the selected mode
requires it.

The small text format is:

```text
BADGER-HANDOFF-V1
mode: handoff

I finished the parser and focused tests. Please review the remaining edge cases.
```

Use `mode: review` when Badger should independently review the current Git
worktree, `mode: design` for reasoning where current changes are not inherently
part of the request, or `mode: handoff` to continue active repository work with
the current Git/worktree context. Design and Handoff seed the body as literal
initial goal text, so a body beginning with a slash command is not dispatched
on the first submission.

The handoff file is ephemeral and may contain sensitive conversation context.
Do not commit it. Badger removes a valid file before starting the workflow;
invalid files are reported and left in place so they can be corrected.

## Install the official Agent Skills

Badger bundles two small offline Agent Skills that produce the handoff file:

- `handoff` transfers a broad coding, debugging, planning, or architecture
  session. It normally writes `mode: handoff`, or `mode: design` when the
  conversation is reasoning-only and current worktree state is irrelevant.
- `badger-code-review` prepares compact guidance for an independent Badger
  review and writes `mode: review`.

Install or update both definitions with the released Badger binary:

```bash
badger skills install
```

Installation is offline and writes only to `~/.agents/skills/`. Existing
unrelated Skills and files are preserved. After using either installed Skill,
open a separate terminal in the directory where it wrote `.badger-handoff` and
run `badger continue`. Badger validates and removes an accepted handoff before
starting the selected workflow.

## Explore a project with no initial goal

Run `badger` without arguments to start in Design focus. Leave the editor empty
and press Enter. Badger silently uses this internal exploratory task and enters
the normal Map → Extract flow:

```text
Explore this project with an open mind. Explain what stands out, how its main parts fit together, and surface any interesting opportunities, risks, or improvements worth investigating.
```

The task is not shown in the editor before submission. The editor remains
visually empty and keeps the placeholder `Describe the task...`.

## Manual mode

If you want an open-ended project exploration, run `badger` without arguments.
For an implementation-oriented task, start in Code focus with `badger code` or
enter `/code` at the home screen:

```bash
cd your-project
badger code
```

Type a goal at the prompt. You can also paste a git diff or other supporting
text if you want to provide details of the change; large pasted context may be
kept as a removable attachment so the goal stays readable. To use the
zero-input Design exploration, return home, enter `/design`, and leave the
editor empty before pressing Enter.

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

```bash
badger code
```

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

```bash
badger code
```

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

```bash
badger code
```

```text
Add validation so empty project names are rejected before saving.
Keep the change small and include any tests that should change.
```

## Commands

- `/help`: show the interactive command reference.
- `/code`: switch the active focus to Code and clear the current goal and attachments.
- `/review`: load current Git review context as a removable attachment and keep
  the editor available for optional guidance. It reuses the same flow as
  `badger review`.
- `/design`: switch the active focus to Design and clear the current goal and attachments. Press Enter with the empty editor to start the zero-input exploration.
- `/followup`: switch the active focus to Follow-up. The active focus appears in the status bar as `Focus: Follow-up` and the prompt seeds a short follow-up framing.
- `/exit`: quit Badger.

To start in a specific focus from the command line, pass the focus name as the
first argument:

```bash
badger            # Design focus (default); empty Enter starts exploration
badger code       # Code focus
badger design     # Design focus — starts with an empty editor
badger review     # Review focus — Git context attachment plus optional guidance
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
The fenced Git diff follows the selected mode and is authoritative. Optional
complete supporting files are read from the current checkout, so they can be
newer than a staged, branch, or commit diff.

### Deep Review API

Editor integrations and scripts can generate a complete standalone review
request from one Git repository with the stable non-interactive API. The
official [VS Code companion](https://github.com/PVRLabs/aibadger-vscode) uses
these operations for Deep Review:

```bash
badger api review-context --root . --input guidance.txt
```

Here, complete means directly usable review instructions plus Git and
supporting-file context; it does not mean the interactive TUI's topology is
included. The command does not inspect project topology, use the clipboard,
launch a provider, or access the network. The Git diff is authoritative;
eligible complete changed-file context is optional and bounded by the effective
file and total limits. If the AI returns only
`FILE:`, `PREFIX:`, or `NEAR:` selectors for additional context, pass them to
`badger api review-continuation --root . --input selectors.txt`. Findings-only
responses end the review and do not require continuation.

## Attachments

When you paste a git diff, error output, or other supporting text into
Badger, it is preserved as a **removable attachment** so the goal input
stays clean and focused. In default mode, the `badger review` command attaches
the authoritative diff, explicit file-context status, bounded eligible
current-working-tree files, and up to 25 relevant Git-untracked paths without
their contents. Relevant untracked paths alone are enough to start a review.
See [Review Options](#review-options) for how `--staged`, `--branch`, and
`--commit` affect attachment behavior. Text pastes exceeding
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
