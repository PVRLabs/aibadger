# Verify AI Badger's Local-Only Operation

## Scope and result

Source inspection on October 22, 2026 found **no reachable outbound networking
operations in Badger's Go code in the audited release configurations**. This is
source-level evidence, not a formal proof or a guarantee that the process tree
or operating system never uses a network. External programs and network-backed
filesystems are important exceptions below.

The audited source revision is
[`09d9be15c22184bf0baba8f5cfb3791ae349e0ae`](https://github.com/PVRLabs/aibadger/tree/09d9be15c22184bf0baba8f5cfb3791ae349e0ae),
using Go 1.27.1 on macOS amd64. Package selection was checked for all five
release targets with `CGO_ENABLED=0`, tag `aibadger_release`, and the standalone
module (`GOWORK=off`). The release workflow builds these targets with
`-trimpath -ldflags="-s -w"`:

| Target | Selected packages, including standard library | Network packages |
| --- | ---: | --- |
| darwin/amd64 | 209 | `net/url`, `net/netip` |
| darwin/arm64 | 209 | `net/url`, `net/netip` |
| linux/amd64 | 211 | `net/url`, `net/netip` |
| linux/arm64 | 211 | `net/url`, `net/netip` |
| windows/amd64 | 220 | `net`, `net/url`, `net/netip` |

These are package dependency closures, not function call graphs. No selected
package uses cgo; no target includes `net/http`. No cross-platform execution or
packet capture was performed. Other Go versions, build tags, dependencies, and
future revisions need a fresh review.

## Reproduce the inspection

Use a separate checkout at the revision above and the Go version in `go.mod`.
Obtaining Go, cloning the repository, and populating its module cache can require
network access. Once those prerequisites are present, the following commands
fail rather than download missing modules or toolchains:

```sh
export GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOWORK=off
export CGO_ENABLED=0
unset GOFLAGS

git rev-parse HEAD
go version
go list -m all
go mod graph

for target in darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64; do
  export GOOS=${target%/*} GOARCH=${target#*/}
  echo "$target"
  go list -deps -tags aibadger_release ./cmd/badger
  go list -deps -tags aibadger_release \
    -f '{{.ImportPath}} imports={{join .Imports ","}} cgo={{join .CgoFiles ","}}' ./cmd/badger
  go list -deps -tags aibadger_release \
    -f '{{.Dir}}{{range .GoFiles}} {{.}}{{end}}' ./cmd/badger
 done
```

The last command locates the actual selected source files, including transitive
modules and platform-specific files. Inspect those files, not just `go.mod`:
module graphs also contain packages used only by dependencies' tests or tools.
Use `go list -deps -json -tags aibadger_release ./cmd/badger` when machine-readable
file lists and imports are useful. Review package initializers as well as callers.

A starting search in the repository (using ripgrep) is:

```sh
rg -n 'net/|"net"|Dial|Listen|LookupHost|LookupIP|Socket|Connect|syscall|unsafe|import "C"|go:linkname|exec\.|StartProcess' \
  cmd internal pkg --glob '*.go' --glob '!**/*_test.go'
rg -n 'go:build|CGO_ENABLED|go build' cmd internal .github/workflows
```

Repeat the search over the selected dependency files. Follow actual imports,
aliases, function calls, initializers, and native API wrappers. Keyword matches
alone neither prove reachability nor establish its absence. Generated syscall
packages contain many unused networking wrappers; inspect their callers rather
than treating every wrapper definition as an executed operation.

## Findings and source anchors

- Badger's implementation contains no identified HTTP client, socket connection,
  DNS lookup, telemetry sender, or remote AI API client. The direct dependencies
  provide terminal UI, rendering, text handling, and OS facilities; the selected
  transitive dependencies include clipboard and terminal support.
- `net/url` is used by `charmbracelet/x/ansi` (`cwd.go`) to format terminal working
  directory URLs. URL formatting does not send a request. `net/netip` is address
  representation support, not evidence of a connection.
- Windows imports `net` through `golang.org/x/sys/windows` (`types_windows.go`),
  where `SocketAddress.IP` returns `net.IP`. That module also defines socket and
  DNS wrappers. No call path from Badger to those operations was identified.
- Low-level APIs used by Badger and its terminal dependencies handle console
  input/output, signals, file operations, and clipboard access. For example,
  `internal/codemap/replace_windows.go` uses `CreateFile` and
  `NtSetInformationFile` for report replacement; `internal/downloads` locates
  the OS Downloads folder and writes local files. Its name does not indicate
  an HTTP downloader.
- Development and profile tags select constants in `cmd/badger/dev_*.go`.
  The profile option in `main.go` writes a CPU profile through `runtime/pprof`,
  rather than starting an HTTP profiling server. The table above covers release
  selection only, with cgo disabled, matching `.github/workflows/release.yml`.

This was manual static inspection supported by Go's dependency selection and
source searches, not exhaustive automated call-graph analysis or a review of
all Go runtime and OS internals. Package presence alone does not show that a
connection operation is reachable, and its absence does not cover subprocesses
or network activity triggered by ordinary file access.

## External subprocess and environment considerations

Badger invokes programs directly through `os/exec`; it does not isolate them
from the network. Executables found through `PATH`, inherited environment,
repository configuration, and OS services remain part of the trust boundary.

| Source / purpose | Invocations and limitations |
| --- | --- |
| `internal/reviewtask`: Git review context | `git -C …` with `rev-parse`, `merge-base`, `diff`, `show`, `ls-files`, `symbolic-ref`, `show-ref`, and `hash-object`. No explicit fetch, pull, push, or clone. Diffs use `--no-ext-diff`, but Git configuration and attributes can still affect behavior, including text conversion helpers and on-demand object fetching in partial clones. |
| `internal/clipboard`: copying | `pbcopy` on macOS, `xclip -selection clipboard` on Linux, `clip` on Windows. X11 can use a remote display; clipboard managers, OS sync, and remote sessions can transfer clipboard content. |
| Bubbles → `atotto/clipboard`: pasting | macOS `pbpaste`; Linux selection of Wayland, X11, Termux, or WSL helpers (`wl-paste`, `xclip`, `xsel`, `termux-clipboard-get`, or `powershell.exe Get-Clipboard`). Windows uses native clipboard APIs. These are separate from Badger's copy helper. |
| `internal/diagnose`: environment report | Version probes for Git, Go, Java, Maven, Gradle, Node, npm, Python, pip, .NET, CMake, and C/C++ compilers. npm update notification and several .NET telemetry/update settings are disabled for those probes. This does not guarantee all installed tools or wrappers stay offline; Go toolchain selection and tool initialization can cause downloads or other network activity. |
| `internal/tui/prompt_reveal.go`: reveal prompt file | `open -R`, `explorer /select,`, or `xdg-open` on the containing directory. The file manager's behavior is external to Badger. |
| `charmbracelet/colorprofile`: terminal detection | Its detection code can run `tmux info` when applicable. tmux uses a server socket and its configuration is external. |

Use local disks and trusted executables/configuration for an offline deployment.
UNC paths, mounted network shares, cloud-synced folders, redirected home or
Downloads folders, and remote displays can cause network activity through OS
file or display services without a Go networking call. This audit does not
establish whole-machine or process-tree network isolation. Enforce that
separately with your organization's network controls if required.

## Separate boundaries

- **CLI runtime:** prepares context and applies reviewed changes locally, subject
  to the external-program and filesystem limitations above.
- **Installation and builds:** installers, package managers, Git, and Go can
  download releases, toolchains, or modules. They are outside the runtime claim.
- **VS Code:** the extension, editor, other extensions, and editor services have
  their own behavior and were not audited here.
- **AI chats:** the provider receives context you choose to share. Its handling
  of that context is governed separately. Review it for secrets before sharing;
  local preparation does not make disclosure safe automatically.
- **Codebase Map HTML:** standalone generated reports and hosted website/demo
  behavior are separate from this Go CLI audit; clicking an external link can
  open a network-connected browser.
