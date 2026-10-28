package extractor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileMarkdownChatGPTClipboardSamples(t *testing.T) {
	for _, sample := range []struct {
		name  string
		paths []string
	}{
		{"lines", []string{
			"cmd/badger/main.go", "internal/tui/tui.go", "internal/extractor/extractor.go",
			"internal/extractor/extractor_test.go", "internal/taggedfile/taggedfile.go",
			"internal/taggedfile/taggedfile_test.go", "internal/clipboard/clipboard.go",
			"internal/clipboard/clipboard_windows_test.go", "tests/cli/platform_smoke_test.go",
			"scripts/windows-smoke.ps1",
		}},
		{"inline", []string{
			"README.md", "AGENTS.md", "go.mod", "cmd/badger/main.go", "cmd/badger/main_test.go",
			"tests/cli/platform_smoke_test.go", "tests/cli/smoke_test.go", "internal/diagnose/diagnose.go",
			"internal/diagnose/command_windows_test.go", "internal/clipboard/clipboard_windows_test.go",
			"internal/tui/tui.go", "internal/brand/banner.go", "internal/brand/brand.go",
			".github/workflows/ci.yml", "scripts/windows-smoke.ps1", "scripts/windows-installer-smoke.ps1",
		}},
	} {
		t.Run(sample.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/chatgpt-" + sample.name + ".txt")
			if err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			for _, path := range sample.paths {
				writeChatGPTFile(t, root, path)
			}
			e := NewExtractor(root, nil)
			// Exercise actual escaped bytes and a separate unescaped representation;
			// clipboard text need not contain the escapes visible in Markdown source.
			inputs := []string{string(data), strings.ReplaceAll(string(data), `\_`, `_`), strings.ReplaceAll(string(data), "\n", "\r\n")}
			for _, input := range inputs {
				parsed := e.ParseCommandsDetailed(input)
				if len(parsed.Failures) != 0 || len(parsed.Commands) != len(sample.paths) {
					t.Fatalf("parse = %+v", parsed)
				}
				results, err := e.Extract(parsed.Commands)
				if err != nil || len(results) != len(sample.paths) {
					t.Fatalf("Extract = %+v, %v", results, err)
				}
				for i, result := range results {
					if filepath.ToSlash(result.Path) != sample.paths[i] || result.Content != sample.paths[i] || !result.FullFile {
						t.Fatalf("reference %d = %+v, want %q", i, result, sample.paths[i])
					}
				}
			}
		})
	}
}

func writeChatGPTFile(t *testing.T, root, path string) {
	t.Helper()
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(path), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestFileMarkdownChatGPTEscapesAndLineBreaks(t *testing.T) {
	root := t.TempDir()
	const path = "internal/diagnose/command_windows_test.go"
	writeChatGPTFile(t, root, path)
	e := NewExtractor(root, nil)
	input := "FILE:internal/diagnose/command\\_windows\\_test.go\\\r\n"
	parsed := e.ParseCommandsDetailed(input)
	const escapedPath = `internal/diagnose/command\_windows\_test.go`
	if len(parsed.Failures) != 0 || len(parsed.Commands) != 1 || parsed.Commands[0].Path != escapedPath {
		t.Fatalf("parse = %+v, want preserved separators and underscore escapes without trailing backslash", parsed)
	}
	results, err := e.Extract(parsed.Commands)
	if err != nil || len(results) != 1 {
		t.Fatalf("Extract = %+v, %v", results, err)
	}
	if filepath.ToSlash(results[0].Path) != path || results[0].Content != path || !results[0].FullFile {
		t.Fatalf("result = %+v, want %q", results[0], path)
	}
}

func TestFileMarkdownChatGPTMixedDelimitersAndInvalidReferences(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"docs/my file.md", "main_test.go", "other.go"} {
		writeChatGPTFile(t, root, path)
	}
	e := NewExtractor(root, nil)
	input := "FILE:docs/my file.md FILE:main\\_test.go\\\r\nFILE:other.go FILE:missing\\_test.go\nFILE:\\"
	parsed := e.ParseCommandsDetailed(input)
	if len(parsed.Commands) != 4 || len(parsed.Failures) != 1 {
		t.Fatalf("parse = %+v", parsed)
	}
	results, err := e.Extract(parsed.Commands)
	if err == nil || len(results) != 3 || results[0].Path != "docs/my file.md" || results[1].Path != "main_test.go" || results[2].Path != "other.go" {
		t.Fatalf("Extract = %+v, %v", results, err)
	}
	// Recovery must not remove arbitrary escapes or discard trailing prose to
	// manufacture an existing path, even when a shorter candidate exists.
	for _, input := range []string{`FILE:main\*test.go`, `FILE:main\_test.go extra words`, `FILE:missing\_test.go`, `FILE:docs/my file.md extra words`} {
		if results, err := e.Extract(e.ParseCommands(input)); err == nil || len(results) != 0 {
			t.Fatalf("Extract(%q) = %+v, %v", input, results, err)
		}
	}
}
