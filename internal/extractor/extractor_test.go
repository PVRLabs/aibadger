package extractor

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestParseCommands(t *testing.T) {
	e := NewExtractor("", nil)
	input := `
FILE:src/main/java/App.java
PREFIX:src/main/java/Service.java#public class Service
NEAR:src/main/java/Utils.java#helperMethod
INVALID:something
`
	commands := e.ParseCommands(input)

	if len(commands) != 3 {
		t.Fatalf("Expected 3 commands, got %d", len(commands))
	}

	if commands[0].Type != "FILE" || commands[0].Path != "src/main/java/App.java" {
		t.Errorf("Incorrect FILE command: %+v", commands[0])
	}
	if commands[1].Type != "PREFIX" || commands[1].Path != "src/main/java/Service.java" || commands[1].Pattern != "public class Service" {
		t.Errorf("Incorrect PREFIX command: %+v", commands[1])
	}
	if commands[2].Type != "NEAR" || commands[2].Path != "src/main/java/Utils.java" || commands[2].Pattern != "helperMethod" {
		t.Errorf("Incorrect NEAR command: %+v", commands[2])
	}
}

func TestParseCommandsRecoversClaudeStyleEmbeddedFileTokens(t *testing.T) {
	e := NewExtractor("", nil)
	input := `Claude responded: FILE:CLAUDE.FILE:CLAUDE.md
FILE:AGENTS.md
FILE:.mvn/jvm.config
FILE:pom.xml
`
	commands := e.ParseCommands(input)

	want := []Command{
		{Type: "FILE", Path: "CLAUDE"},
		{Type: "FILE", Path: "CLAUDE.md"},
		{Type: "FILE", Path: "AGENTS.md"},
		{Type: "FILE", Path: ".mvn/jvm.config"},
		{Type: "FILE", Path: "pom.xml"},
	}
	if len(commands) != len(want) {
		t.Fatalf("ParseCommands() count = %d, want %d: %+v", len(commands), len(want), commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("ParseCommands()[%d] = %+v, want %+v", i, commands[i], want[i])
		}
	}
}

func TestParseCommandsSplitsAdjacentFileTokens(t *testing.T) {
	e := NewExtractor("", nil)
	commands := e.ParseCommands("FILE:CLAUDE.FILE:CLAUDE.md")

	want := []Command{
		{Type: "FILE", Path: "CLAUDE"},
		{Type: "FILE", Path: "CLAUDE.md"},
	}
	if len(commands) != len(want) {
		t.Fatalf("ParseCommands() count = %d, want %d: %+v", len(commands), len(want), commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("ParseCommands()[%d] = %+v, want %+v", i, commands[i], want[i])
		}
	}
}

func TestParseCommandsIgnoresFileTypos(t *testing.T) {
	e := NewExtractor("", nil)
	commands := e.ParseCommands("FLLE:pom.xml")

	if len(commands) != 0 {
		t.Fatalf("ParseCommands() = %+v, want no commands", commands)
	}
}

func TestParseCommandsDoesNotRecoverFileTokensInsidePrefixOrNearPatterns(t *testing.T) {
	e := NewExtractor("", nil)
	input := `PREFIX:src/service.go#literal FILE:example.FILE:other
NEAR:src/service.go#another FILE:example.FILE:other
`
	commands := e.ParseCommands(input)

	want := []Command{
		{Type: "PREFIX", Path: "src/service.go", Pattern: "literal FILE:example.FILE:other"},
		{Type: "NEAR", Path: "src/service.go", Pattern: "another FILE:example.FILE:other"},
	}
	if len(commands) != len(want) {
		t.Fatalf("ParseCommands() count = %d, want %d: %+v", len(commands), len(want), commands)
	}
	for i := range want {
		if commands[i] != want[i] {
			t.Fatalf("ParseCommands()[%d] = %+v, want %+v", i, commands[i], want[i])
		}
	}
}

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected Command
		ok       bool
	}{
		{
			name: "file command",
			line: "FILE:path/to/file.go",
			expected: Command{
				Type: "FILE",
				Path: "path/to/file.go",
			},
			ok: true,
		},
		{
			name: "prefix command",
			line: "prefix: path/to/file.go # func main ",
			expected: Command{
				Type:    "PREFIX",
				Path:    "path/to/file.go",
				Pattern: "func main",
			},
			ok: true,
		},
		{
			name: "invalid command",
			line: "NOTE:path/to/file.go",
			ok:   false,
		},
		{
			name: "empty path",
			line: "FILE:",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseCommandLine(tt.line)
			if ok != tt.ok {
				t.Fatalf("parseCommandLine() ok = %v, want %v", ok, tt.ok)
			}
			if ok && got != tt.expected {
				t.Fatalf("parseCommandLine() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestExtractReturnsErrorForMissingRequestedFile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "present.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "present.go"},
		{Type: "FILE", Path: "missing.go"},
	})

	if err == nil {
		t.Fatal("Extract() error = nil, want missing-file error")
	}
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("Extract() partial missing-file error did not allow proceeding")
	}
	if !strings.Contains(err.Error(), "missing.go") {
		t.Fatalf("error does not name missing file: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one successful extraction", len(results))
	}
	if results[0].Path != "present.go" {
		t.Fatalf("result path = %q, want present.go", results[0].Path)
	}
}

func TestExtractFilePrefersLocalFileOverExternalContext(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "usage.md"), []byte("local usage\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "usage.md"), []byte("external usage\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})
	results, err := e.Extract([]Command{{Type: "FILE", Path: "docs/usage.md"}})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one extraction", len(results))
	}
	if results[0].Content != "local usage\n" {
		t.Fatalf("content = %q, want local file content", results[0].Content)
	}
}

func TestExtractFileFallsBackToTolerantExternalContextMatches(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(external, "plans"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "mvp-roadmap.md"), []byte("root roadmap\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "plans", "release-plan.md"), []byte("nested plan\n"), 0644); err != nil {
		t.Fatal(err)
	}
	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})

	for _, tt := range []struct {
		name    string
		path    string
		content string
	}{
		{name: "root display suffix", path: "docs/mvp-roadmap.md", content: "root roadmap\n"},
		{name: "unique basename", path: "release-plan.md", content: "nested plan\n"},
		{name: "nested suffix", path: "docs/plans/release-plan.md", content: "nested plan\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			results, err := e.Extract([]Command{{Type: "FILE", Path: tt.path}})
			if err != nil {
				t.Fatalf("Extract() error = %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("results = %d, want one extraction", len(results))
			}
			if results[0].Content != tt.content {
				t.Fatalf("content = %q, want %q", results[0].Content, tt.content)
			}
			if !results[0].FullFile {
				t.Fatal("expected external FILE extraction to be marked fullFile")
			}
		})
	}
}

func TestExtractFileReportsAmbiguousExternalContextMatchWithPartialSuccess(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	ctxA := filepath.Join(parent, "badger-sidecar", "docs")
	ctxB := filepath.Join(parent, "other-context", "docs")
	for _, dir := range []string{root, ctxA, ctxB} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "present.md"), []byte("present\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctxA, "mvp-roadmap.md"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ctxB, "mvp-roadmap.md"), []byte("b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{Path: "../badger-sidecar/docs", AbsPath: ctxA},
			{Path: "../other-context/docs", AbsPath: ctxB},
		},
	})
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "present.md"},
		{Type: "FILE", Path: "mvp-roadmap.md"},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want ambiguity warning")
	}
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("ambiguous external match with usable result should allow proceeding")
	}
	for _, want := range []string{
		"Ambiguous file reference: mvp-roadmap.md",
		"../badger-sidecar/docs/mvp-roadmap.md",
		"../other-context/docs/mvp-roadmap.md",
		"Use a more specific path to disambiguate.",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q:\n%v", want, err)
		}
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one successful extraction", len(results))
	}
	if results[0].Path != "present.md" || results[0].Content != "present\n" {
		t.Fatalf("result = %+v, want present.md content", results[0])
	}
}

func TestExtractFileRejectsExternalContextEscapes(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{root, external, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.md"), []byte("secret\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})
	_, err := e.Extract([]Command{{Type: "FILE", Path: "../outside/secret.md"}})
	if err == nil {
		t.Fatal("Extract() error = nil, want unresolved escape")
	}
	if !strings.Contains(err.Error(), "file not found: ../outside/secret.md") {
		t.Fatalf("Extract() error = %q, want file-not-found escape", err.Error())
	}
}

func TestExtractFileRejectsAbsoluteExternalPathWithoutFuzzyLocalFallback(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(filepath.Join(root, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "architecture.md"), []byte("local architecture\n"), 0644); err != nil {
		t.Fatal(err)
	}
	externalPath := filepath.Join(external, "architecture.md")
	if err := os.WriteFile(externalPath, []byte("external architecture\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		Modules: []model.Module{{
			SourceRoots: []model.SourceRoot{{Path: "src"}},
		}},
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})
	_, err := e.Extract([]Command{{Type: "FILE", Path: filepath.ToSlash(externalPath)}})
	if err == nil {
		t.Fatal("Extract() error = nil, want unsupported absolute external path")
	}
	if !strings.Contains(err.Error(), "file not found: "+filepath.ToSlash(externalPath)) {
		t.Fatalf("Extract() error = %q, want file-not-found absolute path", err.Error())
	}
	if strings.Contains(err.Error(), "local architecture") || strings.Contains(err.Error(), "external architecture") {
		t.Fatalf("Extract() error leaks file content: %v", err)
	}
}

func TestExtractFileRejectsExternalContextSymlinkEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{root, external, outside} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	outsideFile := filepath.Join(outside, "secret.md")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(external, "linked-secret.md")
	if err := os.Symlink(outsideFile, linkPath); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})
	_, err := e.Extract([]Command{{Type: "FILE", Path: "linked-secret.md"}})
	if err == nil {
		t.Fatal("Extract() error = nil, want unresolved symlink escape")
	}
	if !strings.Contains(err.Error(), "file not found: linked-secret.md") {
		t.Fatalf("Extract() error = %q, want file-not-found symlink escape", err.Error())
	}
}

func TestExtractExternalContextBinaryAndAssetSafety(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	for _, dir := range []string{root, external} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "present.md"), []byte("present\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "payload.bin"), []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "hero.png"), []byte("not decoded as source"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{{Path: "../badger-sidecar/docs", AbsPath: external}},
	})
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "present.md"},
		{Type: "FILE", Path: "payload.bin"},
		{Type: "FILE", Path: "hero.png"},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want binary exclusion warning")
	}
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("binary exclusion with usable results should allow proceeding")
	}
	if len(extractionErr.Excluded) != 1 || !strings.Contains(extractionErr.Excluded[0], "payload.bin") {
		t.Fatalf("Excluded = %v, want payload.bin exclusion", extractionErr.Excluded)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want present file and asset summary", len(results))
	}
	if results[0].Path != "present.md" || results[0].Content != "present\n" {
		t.Fatalf("first result = %+v, want present.md content", results[0])
	}
	if results[1].Path != "hero.png" {
		t.Fatalf("second result path = %q, want hero.png", results[1].Path)
	}
	if !strings.Contains(results[1].Content, "Binary file: hero.png (21B, kind: asset)") {
		t.Fatalf("asset summary = %q, want binary summary", results[1].Content)
	}
}

func TestExtractPreservesCommandOrder(t *testing.T) {
	tempDir := t.TempDir()
	files := map[string]string{
		"first.go":  "package first\n",
		"second.go": "package second\n",
		"third.go":  "package third\n",
	}
	for path, content := range files {
		if err := os.WriteFile(filepath.Join(tempDir, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "first.go"},
		{Type: "FILE", Path: "second.go"},
		{Type: "FILE", Path: "third.go"},
	})

	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	got := []string{results[0].Path, results[1].Path, results[2].Path}
	want := []string{"first.go", "second.go", "third.go"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("result order = %v, want %v", got, want)
		}
	}
}

func TestExtractFallsBackToWholeFileWhenNearPatternMissesInSmallFile(t *testing.T) {
	tempDir := t.TempDir()
	content := "package main\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "NEAR", Path: "main.go", Pattern: "panic"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil for whole-file fallback", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one extraction", len(results))
	}
	if !results[0].FullFile {
		t.Fatal("expected whole-file fallback to be marked fullFile")
	}
	if results[0].Content != content {
		t.Fatalf("content = %q, want whole file %q", results[0].Content, content)
	}
}

func TestExtractFallsBackToCompactWindowWhenPrefixPatternMissesInLargeFile(t *testing.T) {
	tempDir := t.TempDir()
	content := strings.Join([]string{
		"package main",
		"line 1",
		"line 2",
		"line 3",
		"line 4",
		"line 5",
		"line 6",
		"line 7",
		"line 8",
		"line 9",
		"line 10",
		"line 11",
		"line 12",
		"line 13",
		"line 14",
		"line 15",
		"line 16",
		"line 17",
		"line 18",
		"line 19",
		"line 20",
		"line 21",
		"line 22",
		"line 23",
		"line 24",
		"line 25",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tempDir, "headless.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "PREFIX", Path: "headless.go", Pattern: "func Start"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil for compact-window fallback", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one extraction", len(results))
	}
	if strings.TrimSpace(results[0].Content) == strings.TrimSpace(content) {
		t.Fatalf("results = whole file, want compact window")
	}
}

func TestExtractFallsBackToExactWholeFileForSmallFile(t *testing.T) {
	e := NewExtractor("", nil)
	content := "line1\r\nline2\r\n"

	got, fullFile, err := e.extractBlock(content, "NEAR", "line1")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if !fullFile {
		t.Fatal("expected whole-file fallback to be marked fullFile")
	}
	if got != content {
		t.Fatalf("whole-file fallback changed content:\n got %q\nwant %q", got, content)
	}
}

func TestExtractPrefersSingleWholeFileWhenFileRequestedAlongsideSelectors(t *testing.T) {
	tempDir := t.TempDir()
	content := "package main\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(tempDir, "main.go"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "PREFIX", Path: "main.go", Pattern: "func main"},
		{Type: "FILE", Path: "main.go"},
		{Type: "NEAR", Path: "main.go", Pattern: "main"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want one whole-file extraction", len(results))
	}
	if !results[0].FullFile {
		t.Fatal("expected FILE extraction to be marked fullFile")
	}
	if results[0].Path != "main.go" {
		t.Fatalf("result path = %q, want main.go", results[0].Path)
	}
	if results[0].Content != content {
		t.Fatalf("content = %q, want whole file %q", results[0].Content, content)
	}
}

func TestExtractFallsBackToCompactWindowWhenSmallByLinesButNotBytes(t *testing.T) {
	e := NewExtractor("", nil)
	blob := strings.Repeat("x", 380)
	content := strings.Join([]string{
		blob + " 1",
		blob + " 2",
		blob + " 3",
		blob + " 4",
		blob + " 5 needle",
		blob + " 6",
		blob + " 7",
		blob + " 8",
		blob + " 9",
		blob + " 10",
		blob + " 11",
		blob + " 12",
	}, "\n") + "\n"

	got, fullFile, err := e.extractBlock(content, "NEAR", "needle")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fullFile {
		t.Fatal("expected compact window to be marked partial")
	}
	if strings.TrimSpace(got) == strings.TrimSpace(content) {
		t.Fatalf("expected compact window, got whole file")
	}
	if !strings.Contains(got, "needle") {
		t.Fatalf("compact window does not include anchor:\n%s", got)
	}
}

func TestExtractSkipsPrompt2SensitiveAndBinaryFiles(t *testing.T) {
	tempDir := t.TempDir()
	files := map[string][]byte{
		".env":                []byte("SECRET=1\n"),
		".env.local":          []byte("LOCAL=1\n"),
		"keys/id_rsa":         []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"),
		"bin/native":          []byte{0x00, 0x01, 0x02, 0x03},
		"src/main.go":         []byte("package main\n"),
		"src/config/app.go":   []byte("package config\n"),
		"src/notes.txt":       []byte("not secret\n"),
		"nested/.azure/token": []byte("should not matter\n"),
	}
	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: ".env"},
		{Type: "FILE", Path: ".env.local"},
		{Type: "FILE", Path: "keys/id_rsa"},
		{Type: "FILE", Path: "bin/native"},
		{Type: "FILE", Path: "src/main.go"},
		{Type: "FILE", Path: "src/config/app.go"},
		{Type: "FILE", Path: "src/notes.txt"},
		{Type: "FILE", Path: "nested/.azure/token"},
	})
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError for safety exclusions", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("Extract() safety-exclusion warning did not allow proceeding")
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3 safe files", len(results))
	}
	gotPaths := []string{results[0].Path, results[1].Path, results[2].Path}
	wantPaths := []string{"src/main.go", "src/config/app.go", "src/notes.txt"}
	for i := range wantPaths {
		if gotPaths[i] != wantPaths[i] {
			t.Fatalf("safe result order = %v, want %v", gotPaths, wantPaths)
		}
	}
	for _, result := range results {
		if strings.Contains(result.Content, "SECRET") || strings.Contains(result.Content, "PRIVATE KEY") {
			t.Fatalf("excluded content leaked into results: %+v", result)
		}
	}
}

func TestExtractIgnoresSafetyExclusionsWhenUsableFilesExist(t *testing.T) {
	tempDir := t.TempDir()
	files := map[string][]byte{
		".env":        []byte("SECRET=1\n"),
		"bin/native":  []byte{0x00, 0x01, 0x02, 0x03},
		"src/main.go": []byte("package main\n"),
	}
	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: ".env"},
		{Type: "FILE", Path: "bin/native"},
		{Type: "FILE", Path: "src/main.go"},
	})
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError for safety exclusions", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("Extract() safety-exclusion warning did not allow proceeding")
	}
	if len(results) != 1 || results[0].Path != "src/main.go" {
		t.Fatalf("results = %+v, want only usable file", results)
	}
}

func TestExtractAllowsMissingFilesWhenUsableFilesAndSafetyExclusionsExist(t *testing.T) {
	tempDir := t.TempDir()
	files := map[string][]byte{
		".env":        []byte("SECRET=1\n"),
		"src/main.go": []byte("package main\n"),
	}
	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, content, 0644); err != nil {
			t.Fatal(err)
		}
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: ".env"},
		{Type: "FILE", Path: "missing.go"},
		{Type: "FILE", Path: "src/main.go"},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want partial extraction warning")
	}
	var extractionErr *ExtractionError
	if !errors.As(err, &extractionErr) {
		t.Fatalf("Extract() error = %T, want *ExtractionError", err)
	}
	if !extractionErr.CanProceed {
		t.Fatal("Extract() partial missing-file error did not allow proceeding")
	}
	if len(extractionErr.Failures) != 1 || !strings.Contains(extractionErr.Failures[0], "missing.go") {
		t.Fatalf("Failures = %v, want missing.go only", extractionErr.Failures)
	}
	if len(extractionErr.Excluded) != 1 || !strings.Contains(extractionErr.Excluded[0], ".env") {
		t.Fatalf("Excluded = %v, want .env exclusion tracked separately", extractionErr.Excluded)
	}
	if len(results) != 1 || results[0].Path != "src/main.go" {
		t.Fatalf("results = %+v, want only usable file", results)
	}
}

func TestExtractReturnsErrorWhenAllPrompt2FilesAreExcluded(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".env"), []byte("SECRET=1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "keys"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "keys", "id_rsa"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "binary.bin"), []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	_, err := e.Extract([]Command{
		{Type: "FILE", Path: ".env"},
		{Type: "FILE", Path: "keys/id_rsa"},
		{Type: "FILE", Path: "binary.bin"},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want prompt 2 exclusion error")
	}
	if !errors.Is(err, ErrNoSafePrompt2Files) {
		t.Fatalf("Extract() error = %v, want ErrNoSafePrompt2Files", err)
	}
	if !strings.Contains(err.Error(), "no safe files available for Prompt 2") {
		t.Fatalf("error message = %q, want clear prompt 2 exclusion message", err.Error())
	}
}

func TestExtractBlockRobustLocalSpans(t *testing.T) {
	e := NewExtractor("", nil)

	tests := []struct {
		name     string
		content  string
		cmdType  string
		pattern  string
		expected string
		fullFile bool
	}{
		{
			name: "Go delayed brace signature",
			content: `package main

func Run(
	ctx context.Context,
	name string,
) error {
	return nil
}
`,
			cmdType: "PREFIX",
			pattern: "func Run",
			expected: `func Run(
	ctx context.Context,
	name string,
) error {
	return nil
}
`,
			fullFile: false,
		},
		{
			name: "Go comment attached to declaration",
			content: `package main

// Run handles input.
func Run() error {
	return nil
}
`,
			cmdType: "NEAR",
			pattern: "Run handles input",
			expected: `// Run handles input.
func Run() error {
	return nil
}
`,
			fullFile: false,
		},
		{
			name: "Java annotated method with delayed brace",
			content: `package example;

/**
 * Helper docs.
 */
@Override
public void helper(
    int value,
    String name
) {
    if (value > 0) {
        return;
    }
}
`,
			cmdType: "NEAR",
			pattern: "Helper docs",
			expected: `/**
 * Helper docs.
 */
@Override
public void helper(
    int value,
    String name
) {
    if (value > 0) {
        return;
    }
}
`,
			fullFile: false,
		},
		{
			name: "Java single-line field",
			content: `package example;

public class Config {
    private String name = "badger";
}
`,
			cmdType: "NEAR",
			pattern: "name =",
			expected: `    private String name = "badger";
`,
			fullFile: false,
		},
		{
			name: "JS delayed block arrow function",
			content: `export const run = (
  value,
) => {
  return value;
};
`,
			cmdType: "PREFIX",
			pattern: "export const run",
			expected: `export const run = (
  value,
) => {
  return value;
};
`,
			fullFile: false,
		},
		{
			name: "JS single-line export",
			content: `export const value = 1;
`,
			cmdType: "PREFIX",
			pattern: "export const value",
			expected: `export const value = 1;
`,
			fullFile: false,
		},
		{
			name: "Python def indentation block",
			content: `# Run handles input.
def run(
    value,
):
    if value:
        return value
    return None
`,
			cmdType: "NEAR",
			pattern: "Run handles input",
			expected: `# Run handles input.
def run(
    value,
):
    if value:
        return value
    return None
`,
			fullFile: false,
		},
		{
			name: "Python class indentation block",
			content: `class App:
    def run(self):
        pass
`,
			cmdType: "PREFIX",
			pattern: "class App",
			expected: `class App:
    def run(self):
        pass
`,
			fullFile: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, fullFile, err := e.extractBlock(tt.content, tt.cmdType, tt.pattern)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if fullFile != tt.fullFile {
				t.Fatalf("fullFile = %v, want %v", fullFile, tt.fullFile)
			}
			if strings.TrimSpace(got) != strings.TrimSpace(tt.expected) {
				t.Errorf("Got:\n%s\nExpected:\n%s", got, tt.expected)
			}
		})
	}
}

func TestExtractBlockFallsBackToCompactWindowForAmbiguousText(t *testing.T) {
	e := NewExtractor("", nil)
	content := strings.Join([]string{
		"alpha",
		"beta",
		"gamma",
		"delta",
		"epsilon",
		"zeta",
		"eta",
		"theta",
		"iota",
		"kappa",
		"lambda",
		"mu",
		"nu",
		"xi",
		"omicron anchor",
		"pi",
		"rho",
		"sigma",
		"tau",
		"upsilon",
		"phi",
		"chi",
		"psi",
		"omega",
		"one",
		"two",
		"three",
	}, "\n") + "\n"

	got, fullFile, err := e.extractBlock(content, "NEAR", "omicron")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fullFile {
		t.Fatal("expected compact window to be marked partial")
	}

	if strings.Count(strings.TrimSpace(got), "\n") < 3 {
		t.Fatalf("fallback window too small:\n%s", got)
	}
	if !strings.Contains(got, "omicron anchor") {
		t.Fatalf("fallback window does not include anchor:\n%s", got)
	}
	if strings.Contains(got, "alpha") || strings.Contains(got, "omega") {
		t.Fatalf("fallback window unexpectedly expanded to whole file:\n%s", got)
	}
}

func TestExtractBlockFallsBackForConfigLikeText(t *testing.T) {
	e := NewExtractor("", nil)
	content := strings.Join([]string{
		"title: example",
		"mode: local",
		"alpha",
		"beta",
		"gamma",
		"delta",
		"epsilon",
		"server:",
		"  host: localhost",
		"  port: 8080",
		"  enabled: true",
		"zeta",
		"eta",
		"theta",
		"iota",
		"kappa",
		"lambda",
		"mu",
		"nu",
		"xi",
		"omicron",
		"pi",
		"rho",
		"sigma",
		"tau",
	}, "\n") + "\n"

	got, fullFile, err := e.extractBlock(content, "NEAR", "host: localhost")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if fullFile {
		t.Fatal("expected compact window to be marked partial")
	}
	if strings.Count(strings.TrimSpace(got), "\n") < 3 {
		t.Fatalf("config fallback window too small:\n%s", got)
	}
	if !strings.Contains(got, "host: localhost") {
		t.Fatalf("config fallback window does not include anchor:\n%s", got)
	}
	if strings.Contains(got, "title: example") || strings.Contains(got, "tau") {
		t.Fatalf("config fallback window unexpectedly expanded too far:\n%s", got)
	}
}

func TestResolveFuzzyPath(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Heaviest: model.HeaviestFile{
					Name: "Heaviest.java",
					Path: "src/main/java/Heaviest.java",
				},
				SourceRoots: []model.SourceRoot{
					{
						Path: "src/main/java",
					},
				},
			},
		},
	}

	// Create a temp directory to simulate the project root
	tempDir := t.TempDir()
	e := NewExtractor(tempDir, topology)

	// Create a file in the expected fuzzy path
	targetPath := filepath.Join(tempDir, "src/main/java/Found.java")
	err := os.MkdirAll(filepath.Dir(targetPath), 0755)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(targetPath, []byte("content"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		target   string
		expected string
	}{
		{
			name:     "Find via heaviest file",
			target:   "Heaviest.java",
			expected: "src/main/java/Heaviest.java",
		},
		{
			name:     "Find via source root scan",
			target:   "Found.java",
			expected: "src/main/java/Found.java",
		},
		{
			name:     "Not found",
			target:   "Missing.java",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := e.resolveFuzzyPath(tt.target)
			if got != tt.expected {
				t.Errorf("Got %s, expected %s", got, tt.expected)
			}
		})
	}
}

func TestProcessCommandExcludesBinaryFileFromPrompt2(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "badger"), []byte{0, 1, 2, 3}, 0755); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	content, fullFile, err := e.processCommand(Command{Type: "FILE", Path: "badger"})
	if !errors.Is(err, errPrompt2Excluded) {
		t.Fatalf("processCommand() error = %v, want errPrompt2Excluded", err)
	}
	if fullFile {
		t.Fatalf("processCommand() fullFile = %v, want false for excluded binary", fullFile)
	}
	if content != "" {
		t.Fatalf("processCommand() content = %q, want empty for excluded binary", content)
	}
}

func TestProcessCommandSummarizesAssetForPrefixAndNear(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "hero.png"), []byte("not decoded as source"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	for _, cmdType := range []string{"PREFIX", "NEAR"} {
		content, fullFile, err := e.processCommand(Command{Type: cmdType, Path: "hero.png", Pattern: "source"})
		if err != nil {
			t.Fatalf("processCommand(%s) error = %v", cmdType, err)
		}
		if fullFile {
			t.Fatalf("expected asset summary for %s to not be marked fullFile", cmdType)
		}
		if !strings.Contains(content, "Binary file: hero.png (21B, kind: asset)") {
			t.Fatalf("expected asset summary for %s, got %q", cmdType, content)
		}
	}
}

func TestLineMatchesCommand(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		cmdType string
		pattern string
		want    bool
	}{
		{name: "prefix match", line: "func main() {", cmdType: "PREFIX", pattern: "func main", want: true},
		{name: "near match", line: "log.Println(\"hello\")", cmdType: "NEAR", pattern: "hello", want: true},
		{name: "unsupported type", line: "func main() {", cmdType: "FILE", pattern: "func", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lineMatchesCommand(tt.line, tt.cmdType, tt.pattern); got != tt.want {
				t.Fatalf("lineMatchesCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsWithinProjectRoot(t *testing.T) {
	tests := []struct {
		name    string
		root    string
		absPath string
		want    bool
	}{
		{
			name:    "file under root",
			root:    "/home/user/proj",
			absPath: "/home/user/proj/src/main.go",
			want:    true,
		},
		{
			name:    "root itself",
			root:    "/home/user/proj",
			absPath: "/home/user/proj",
			want:    true,
		},
		{
			name:    "root with trailing slash",
			root:    "/home/user/proj/",
			absPath: "/home/user/proj/internal/foo.go",
			want:    true,
		},
		{
			name:    "parent traversal escape",
			root:    "/home/user/proj",
			absPath: "/home/user/other/file.go",
			want:    false,
		},
		{
			name:    "absolute path injection",
			root:    "/home/user/proj",
			absPath: "/etc/passwd",
			want:    false,
		},
		{
			name:    "sibling directory",
			root:    "/home/user/proj",
			absPath: "/home/user/other",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isWithinProjectRoot(tt.root, tt.absPath); got != tt.want {
				t.Fatalf("isWithinProjectRoot(%q, %q) = %v, want %v", tt.root, tt.absPath, got, tt.want)
			}
		})
	}
}

func TestProcessCommandRejectsParentTraversal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "legit.go"), []byte("package legit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a file one level above the project root
	outsideContent := "this should not be readable\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(tempDir), "outside.txt"), []byte(outsideContent), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)

	tests := []struct {
		name string
		cmd  Command
	}{
		{
			name: "parent traversal via ..",
			cmd:  Command{Type: "FILE", Path: "../outside.txt"},
		},
		{
			name: "deep parent traversal",
			cmd:  Command{Type: "FILE", Path: "../../etc/passwd"},
		},
		{
			name: "absolute path",
			cmd:  Command{Type: "FILE", Path: "/etc/passwd"},
		},
		{
			name: "absolute with traversal",
			cmd:  Command{Type: "FILE", Path: "/etc/../etc/passwd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, fullFile, err := e.processCommand(tt.cmd)
			if err == nil {
				t.Fatalf("processCommand() error = nil, want error")
			}
			if content != "" {
				t.Fatalf("processCommand() content = %q, want empty", content)
			}
			if fullFile {
				t.Fatalf("processCommand() fullFile = true, want false")
			}
			if strings.Contains(err.Error(), outsideContent) {
				t.Fatalf("processCommand() error leaks content: %v", err)
			}
		})
	}
}

func TestProcessCommandAllowsLegitPath(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "legit.go"), []byte("package legit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	content, fullFile, err := e.processCommand(Command{Type: "FILE", Path: "legit.go"})
	if err != nil {
		t.Fatalf("processCommand() error = %v, want nil", err)
	}
	if !fullFile {
		t.Fatal("processCommand() fullFile = false, want true")
	}
	if content != "package legit\n" {
		t.Fatalf("processCommand() content = %q, want %q", content, "package legit\n")
	}
}

func TestExtractRejectsTraversalPath(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "legit.go"), []byte("package legit\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	_, err := e.Extract([]Command{
		{Type: "FILE", Path: "legit.go"},
		{Type: "FILE", Path: "../outside.txt"},
	})
	if err == nil {
		t.Fatal("Extract() error = nil, want error for traversal path")
	}
	if !strings.Contains(err.Error(), "../outside.txt") {
		t.Fatalf("Extract() error = %q, should name the traversal path", err.Error())
	}
}

func TestExtractAllowAllWithinRoot(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "a.go"), []byte("package a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "sub", "b.go"), []byte("package b\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(tempDir, nil)
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "a.go"},
		{Type: "FILE", Path: "sub/b.go"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
}

func TestExtractAllowsExplicitExternalContextFile(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(external, "spec.md")
	if err := os.WriteFile(specPath, []byte("# Spec\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{
				Path:    "../badger-sidecar/docs",
				AbsPath: external,
			},
		},
	})
	results, err := e.Extract([]Command{
		{Type: "FILE", Path: "../badger-sidecar/docs/spec.md"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Path != "../badger-sidecar/docs/spec.md" {
		t.Fatalf("result path = %q, want requested external path", results[0].Path)
	}
	if results[0].Content != "# Spec\n" {
		t.Fatalf("content = %q, want external file content", results[0].Content)
	}
}

func TestExtractAllowsExplicitExternalContextNear(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(external, "spec.md")
	content := "# Spec\n\n## sidecar-boundary\n\nKeep private planning docs out of OSS output.\n"
	if err := os.WriteFile(specPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{
				Path:    "../badger-sidecar/docs",
				AbsPath: external,
			},
		},
	})
	results, err := e.Extract([]Command{
		{Type: "NEAR", Path: "../badger-sidecar/docs/spec.md", Pattern: "sidecar-boundary"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Path != "../badger-sidecar/docs/spec.md" {
		t.Fatalf("result path = %q, want requested external path", results[0].Path)
	}
	if !strings.Contains(results[0].Content, "sidecar-boundary") {
		t.Fatalf("content = %q, want extracted external span near anchor", results[0].Content)
	}
}

func TestExtractAllowsExternalContextNearRelativeToContextRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "spec.md"), []byte("# Spec\n\nsidecar-boundary\n"), 0644); err != nil {
		t.Fatal(err)
	}

	e := NewExtractor(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{
				Path:    "../badger-sidecar/docs",
				AbsPath: external,
			},
		},
	})
	results, err := e.Extract([]Command{
		{Type: "NEAR", Path: "spec.md", Pattern: "sidecar-boundary"},
	})
	if err != nil {
		t.Fatalf("Extract() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !strings.Contains(results[0].Content, "sidecar-boundary") {
		t.Fatalf("content = %q, want extracted external span near anchor", results[0].Content)
	}
}
