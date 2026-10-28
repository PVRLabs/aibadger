package extractor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFileMarkdownIssue171(t *testing.T) {
	data, err := os.ReadFile("testdata/issue171-selectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	input := string(data)
	if strings.HasSuffix(input, "\r\n") {
		input = strings.TrimSuffix(input, "\r\n")
	} else {
		input = strings.TrimSuffix(input, "\n")
	}
	if strings.Count(input, "FILE:") != 21 || strings.Count(input, `\_`) != 10 || strings.ContainsAny(input, "\r\n") {
		t.Fatal("issue fixture must retain 21 references and 10 escapes on one line")
	}
	// Separate the fixture's native Windows separators from Markdown escapes.
	// Exercise the same portable formatting input on every platform; native
	// separator resolution has dedicated coverage below.
	input = strings.ReplaceAll(input, `\`, "/")
	input = strings.ReplaceAll(input, "/_", `\_`)
	testFileMarkdownIssue171(t, input)
}

func testFileMarkdownIssue171(t *testing.T, input string) {
	t.Helper()
	paths := []string{
		"README.md", "AGENTS.md", "cmd/badger/main.go", "cmd/badger/main_test.go",
		"internal/clipboard/clipboard.go", "internal/clipboard/clipboard_windows_test.go",
		"internal/clipboard/clipboard_test.go", "internal/diagnose/diagnose.go",
		"internal/diagnose/command_windows_test.go", "internal/tui/attachments_test.go",
		"internal/tui/tui.go", "pkg/badger/api.go", "pkg/badger/config.go",
		"internal/defaults/constants.go", "tests/cli/platform_smoke_test.go", "tests/cli/smoke_test.go",
		".github/workflows/ci.yml", "scripts/windows-smoke.ps1", "scripts/windows-installer-smoke.ps1",
		"docs/usage.md", "docs/api.md",
	}
	root := t.TempDir()
	for _, path := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, []byte("fixture content: "+path+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := NewExtractor(root, nil)
	var previous []string
	for _, selectors := range []string{input, strings.ReplaceAll(input, `\_`, `_`)} {
		parsed := e.ParseCommandsDetailed(selectors)
		if len(parsed.Failures) != 0 || len(parsed.Commands) != 21 {
			t.Fatalf("parse = %+v", parsed)
		}
		results, err := e.Extract(parsed.Commands)
		if err != nil || len(results) != 21 {
			t.Fatalf("extraction count = %d, error = %v", len(results), err)
		}
		var contents []string
		for i, result := range results {
			if filepath.ToSlash(result.Path) != paths[i] {
				t.Fatalf("reference %d path = %q, want %q", i+1, result.Path, paths[i])
			}
			want := "fixture content: " + paths[i] + "\n"
			if result.Content != want {
				t.Fatalf("reference %d content = %q, want %q", i+1, result.Content, want)
			}
			if previous != nil && result.Content != previous[i] {
				t.Fatalf("corrected reference %d differs", i+1)
			}
			contents = append(contents, result.Content)
		}
		previous = contents
	}
}

func TestFileMarkdownTrailingBackslashes(t *testing.T) {
	e := NewExtractor("", nil)
	for _, suffix := range []string{`\`, `\\`, `\\\`} {
		want := "main.go"
		for _, input := range []string{
			"FILE:main.go" + suffix,
			"`FILE:main.go" + suffix + "`",
			"- FILE:main.go" + suffix,
			"```text\nFILE:main.go" + suffix + "\n```",
		} {
			for _, parse := range []func(string) CommandParseResult{e.ParseCommandsDetailed, e.ParseStrictCommandsDetailed} {
				got := parse(input)
				if len(got.Failures) != 0 || len(got.Commands) != 1 || got.Commands[0].Path != want {
					t.Fatalf("parse %q = %+v, want path %q", input, got, want)
				}
			}
		}
		got := e.ParseCommands("FILE:main.go" + suffix + " FILE:other.go")
		if len(got) != 2 || got[0].Path != want || got[1].Path != "other.go" {
			t.Fatalf("embedded parse = %+v, want first path %q", got, want)
		}
	}
}

func TestFileMarkdownUnderscoreRecovery(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewExtractor(root, nil)
	commands := e.ParseCommands(`FILE:main\_test.go FILE:main_test.go\`)
	got, err := e.Extract(commands)
	if err != nil || len(got) != 2 {
		t.Fatalf("Extract = %+v, %v", got, err)
	}
	if _, _, _, err := e.processCommand(Command{Type: "PREFIX", Path: `main\_test.go`, Pattern: "package"}); err == nil {
		t.Fatal("PREFIX unexpectedly recovered an escaped underscore")
	}
	if _, _, _, err := e.processCommand(Command{Type: "FILE", Path: `missing\_test.go`}); err == nil {
		t.Fatal("missing repaired file unexpectedly resolved")
	}
}

func TestFileMarkdownRecoveryIgnoresLiteralDirectory(t *testing.T) {
	root := t.TempDir()
	literal := `main\_test.go`
	if err := os.MkdirAll(filepath.Join(root, literal), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main_test.go"), []byte("intended file"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewExtractor(root, nil)
	results, err := e.Extract(e.ParseCommands("FILE:" + literal))
	if err != nil || len(results) != 1 || results[0].Path != "main_test.go" || results[0].Content != "intended file" {
		t.Fatalf("directory recovery = %+v, %v", results, err)
	}
}

func TestFileMarkdownRecoveryPreservesWindowsLiteralPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows path separators")
	}
	testFileMarkdownLiteralPriority(t, `folder\_helpers.go`, filepath.Join("folder", "_helpers.go"))
}

func TestFileMarkdownRecoveryPreservesUnixBackslashFilename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires literal backslashes in filenames")
	}
	testFileMarkdownLiteralPriority(t, `folder\_helpers.go`, `folder\_helpers.go`)
}

func testFileMarkdownLiteralPriority(t *testing.T, selector, literalFile string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, literalFile)), 0700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{literalFile: "literal", "folder_helpers.go": "repaired"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	e := NewExtractor(root, nil)
	results, err := e.Extract(e.ParseCommands("FILE:" + selector))
	if err != nil || len(results) != 1 || results[0].Content != "literal" {
		t.Fatalf("literal extraction = %+v, %v", results, err)
	}
}
