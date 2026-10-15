# Codebase Map

Codebase Map helps you see where code is concentrated and find a starting point
for a question or change. Generate one portable HTML report, then open it directly
from disk. No Git repository, provider configuration, server, or frontend runtime
is required.

[Try the live demo](https://pvrlabs.xyz/aibadger/codemap/) to explore a captured
map of the public AI Badger repository.

![AI Badger Codebase Map showing nested folders, Explore links, and folder details](images/codemap.webp)

*The Badger codebase in dark mode. Rectangle area represents each folder's share
of raw nonblank source lines, including comments.*

## Generate and open a report

Map the current directory:

```sh
badger map
```

Or supply a directory:

```sh
badger map /path/to/project
```

The report is written to your current working directory as
`badger-codemap-<repo_name>.html`. After generation, Badger prints an open command
for your operating system. For example, on macOS:

```text
Generated codebase map HTML report.
Use the following command to open:
open -- '/path/to/badger-codemap-project.html'
```

Linux uses `xdg-open`; Windows prints a separately labeled PowerShell
`Invoke-Item` command. Run the command, or open the HTML file yourself.
Successful regeneration replaces an existing regular report. Generation failures
preserve it. Regenerate the report when you want to reflect source changes.

## Read and explore the map

- **Area shows code concentration.** Counts are raw nonblank physical lines,
  including comments. They do not measure complexity or recommend an AI context size.
- **Click a folder to inspect it.** Details show exact line and file totals,
  language proportions, subfolders, and the largest direct files.
- **Use Explore → to drill down.** Larger folder tiles include this link;
  the details panel also has an Explore folder action. Double-click is a shortcut.
  Use breadcrumbs, Up, or Repository to return.
- **Search finds folders across the repository.** Visible-level controls change
  nesting; small regions stay compact to keep the map readable.
- **Files in this folder represents direct files.** Its totals exclude subfolders.
  Individual files are listed in details rather than drawn as rectangles.
- **Colors distinguish folder branches.** Language chips and detail bars use their
  own colors. Switch between dark and light mode with the header control.

A folder without subfolders containing nonblank source gets a summary view when
explored. Spacious terminal tiles show exact totals, their share of lines in the
current view, and the top language percentages. Direct-file lists initially show
up to 20 entries and can expand to the largest 50.

## Start a focused Badger task

Expand **Work on this folder with Badger** in the details panel to copy a startup
command and a Design / exploration or Review task. Run `badger` for Design
(the default) or `badger review` to review Git changes from the repository root,
then paste the task into Badger's goal editor.
Copy Prompt 1 into your AI chat. The review may finish with findings from that
initial prompt. Only if the AI requests files, paste its response into Badger
and copy Prompt 2 back to the same chat for additional context.

The selected folder is a starting point. Review includes repository-wide Git
changes unless separately restricted; selecting a folder in the map only guides
review attention. Related files elsewhere may still be needed, and the report
itself contains metadata rather than source context.
The standalone report is separate from Badger's interactive Map → Extract → Apply
workflow. See the [Quick Start](../README.md#quick-start) for the full workflow.

Prefer working in your IDE? Use the [Badger VS Code companion](https://marketplace.visualstudio.com/items?itemName=pvrlabs.ai-badger).
See the [VS Code review walkthrough](https://pvrlabs.xyz/articles/reviewing-ai-generated-code-vscode-aibadger.html) for an example.

## Local generation and limits

Generation and normal report use stay local. The HTML embeds relative paths,
aggregate metadata, and bounded filename lists; it does not embed source contents.
There are no external scripts, fonts, uploads, or automatic browser launches.
Documentation and product links open only when you choose them. Share the report
only if you intend to share its folder names, filenames, and counts.

Recognized source includes Markdown, JSX, and TSX; generic `.txt` and unknown
extensions do not count. Dependency, build, cache, VCS, and editor directories
are skipped. Directories named `bin` remain eligible. Symlinks are not followed,
and a supplied symlink root is rejected. Blank recognized files count toward file
totals even when their folders have no visible map area.

Hard limits are depth 50 below root, 20,000 directories including root,
100,000 recognized files, 50 MiB per source file, and 20 MiB for the complete HTML.
Invalid, unreadable, empty-source, or over-limit inputs fail without publishing a
partial report. Run `badger map --help` for command help.
