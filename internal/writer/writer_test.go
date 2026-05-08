package writer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectIndentStyle(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    indentStyle
	}{
		{
			name:    "tab dominant",
			content: "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"x\")\n\t}\n}\n",
			want:    indentStyle{useTabs: true},
		},
		{
			name:    "space dominant two",
			content: "package main\n\nfunc main() {\n  if true {\n    println(\"x\")\n  }\n}\n",
			want:    indentStyle{spaceWidth: 2},
		},
		{
			name:    "space dominant four",
			content: "package main\n\nfunc main() {\n    if true {\n        println(\"x\")\n    }\n}\n",
			want:    indentStyle{spaceWidth: 4},
		},
		{
			name:    "mixed ties prefer tabs",
			content: "package main\n\nfunc main() {\n\tif true {\n    println(\"x\")\n\t}\n}\n",
			want:    indentStyle{useTabs: true},
		},
		{
			name:    "empty",
			content: "",
			want:    indentStyle{useTabs: true},
		},
		{
			name:    "no indentation",
			content: "package main\n\nfunc main() {}\n",
			want:    indentStyle{useTabs: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectIndentStyle(tt.content)
			if got.useTabs != tt.want.useTabs {
				t.Fatalf("useTabs = %v, want %v", got.useTabs, tt.want.useTabs)
			}
			if got.spaceWidth != tt.want.spaceWidth {
				t.Fatalf("spaceWidth = %d, want %d", got.spaceWidth, tt.want.spaceWidth)
			}
		})
	}
}

func TestNormalizeLine(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		target     indentStyle
		sourceUnit int
		stripAll   bool
		want       string
	}{
		{
			name:       "tab to tab",
			line:       "\treturn 1",
			target:     indentStyle{useTabs: true},
			sourceUnit: 1,
			want:       "\treturn 1",
		},
		{
			name:       "spaces to tab",
			line:       "    return 1",
			target:     indentStyle{useTabs: true},
			sourceUnit: 4,
			want:       "\treturn 1",
		},
		{
			name:       "spaces to spaces",
			line:       "    return 1",
			target:     indentStyle{spaceWidth: 2},
			sourceUnit: 4,
			want:       "  return 1",
		},
		{
			name:       "empty line unchanged",
			line:       "",
			target:     indentStyle{useTabs: true},
			sourceUnit: 4,
			want:       "",
		},
		{
			name:       "whitespace only collapses to empty",
			line:       "   \t  ",
			target:     indentStyle{useTabs: true},
			sourceUnit: 4,
			want:       "",
		},
		{
			name:       "strip all mode",
			line:       "    return 1",
			target:     indentStyle{useTabs: true},
			sourceUnit: 4,
			stripAll:   true,
			want:       "\treturn 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeLine(tt.line, tt.target, tt.sourceUnit, tt.stripAll)
			if got != tt.want {
				t.Fatalf("normalizeLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeContent(t *testing.T) {
	existing := "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"x\")\n\t}\n}\n"
	incoming := "package main\n\nfunc main() {\n    if true {\n        println(\"x\")\n    }\n}\n"

	got := normalizeContent(incoming, existing, WhitespaceModeSmart)
	if got != existing {
		t.Fatalf("normalizeContent() = %q, want %q", got, existing)
	}
}

func TestNormalizeContentPreservesTopLevelJavaDocAlignment(t *testing.T) {
	existing := "package com.example;\n\n/**\n * Hello world!\n */\npublic class App {\n\n    private static final String MESSAGE = \"Hello World!\";\n\n    public String getMessage() {\n        return MESSAGE;\n    }\n}\n"

	got := normalizeContent(existing, existing, WhitespaceModeSmart)
	if got != existing {
		t.Fatalf("normalizeContent() = %q, want %q", got, existing)
	}
}

func TestNormalizeContentPreservesIndentedJavaDocAlignment(t *testing.T) {
	existing := "package com.example;\n\npublic class App {\n\n    /**\n     * Hello world!\n     */\n    public String getMessage() {\n        return \"Hello World!\";\n    }\n}\n"

	got := normalizeContent(existing, existing, WhitespaceModeSmart)
	if got != existing {
		t.Fatalf("normalizeContent() = %q, want %q", got, existing)
	}
}

func TestNormalizeContentReturnsIncomingForNewFile(t *testing.T) {
	incoming := "package main\n\nfunc main() {\n    println(\"x\")\n}\n"

	got := normalizeContent(incoming, "", WhitespaceModeSmart)
	if got != incoming {
		t.Fatalf("normalizeContent() = %q, want %q", got, incoming)
	}
}

func TestWriteFileSmartModeReusesExistingIndentation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("internal", "main.go")
	fullPath := filepath.Join(root, path)

	existing := "package main\n\nfunc main() {\n\tif true {\n\t\tprintln(\"x\")\n\t}\n}\n"
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(existing), 0644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	incoming := "package main\n\nfunc main() {\n    if true {\n        println(\"x\")\n    }\n}\n"
	if err := WriteFile(root, FileUpdate{Path: path, Content: incoming}, WhitespaceModeSmart); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != existing {
		t.Fatalf("written content = %q, want %q", string(got), existing)
	}
}

func TestWriteFileExactModeKeepsIncomingIndentation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("internal", "main.go")
	fullPath := filepath.Join(root, path)

	existing := "package main\n\nfunc main() {\n\tprintln(\"x\")\n}\n"
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(existing), 0644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	incoming := "package main\n\nfunc main() {\n    println(\"x\")\n}\n"
	if err := WriteFile(root, FileUpdate{Path: path, Content: incoming}, WhitespaceModeExact); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != incoming {
		t.Fatalf("written content = %q, want %q", string(got), incoming)
	}
}

func TestWriteFileIgnoreModeStripsIncomingWhitespace(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("internal", "main.go")
	fullPath := filepath.Join(root, path)

	existing := "package main\n\nfunc main() {\n\tprintln(\"x\")\n}\n"
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte(existing), 0644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	incoming := "package main\n\nfunc main() {\n        println(\"x\")\n}\n"
	want := "package main\n\nfunc main() {\n\tprintln(\"x\")\n}\n"
	if err := WriteFile(root, FileUpdate{Path: path, Content: incoming}, WhitespaceModeIgnore); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != want {
		t.Fatalf("written content = %q, want %q", string(got), want)
	}
}

func TestDeleteFileExistingRegularFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("internal", "obsolete.go")
	fullPath := filepath.Join(root, path)

	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(fullPath, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile(existing) error = %v", err)
	}

	if err := WriteFile(root, FileUpdate{Path: path, Kind: UpdateKindDelete}, WhitespaceModeExact); err != nil {
		t.Fatalf("WriteFile(delete) error = %v", err)
	}
	if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
		t.Fatalf("Stat() error = %v, want not exist", err)
	}
}

func TestDeleteFileMissingReturnsError(t *testing.T) {
	root := t.TempDir()

	err := WriteFile(root, FileUpdate{Path: "missing.go", Kind: UpdateKindDelete}, WhitespaceModeExact)
	if err == nil {
		t.Fatal("WriteFile(delete missing) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "missing file") {
		t.Fatalf("error = %q, want missing file error", err)
	}
}

func TestDeleteFileDirectoryReturnsError(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join("internal", "obsolete")
	fullPath := filepath.Join(root, path)

	if err := os.MkdirAll(fullPath, 0755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	err := WriteFile(root, FileUpdate{Path: path, Kind: UpdateKindDelete}, WhitespaceModeExact)
	if err == nil {
		t.Fatal("WriteFile(delete dir) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "non-regular file") {
		t.Fatalf("error = %q, want non-regular file error", err)
	}
}

func TestDeleteFileOutsideProjectRootReturnsError(t *testing.T) {
	root := t.TempDir()

	err := WriteFile(root, FileUpdate{Path: filepath.Join("..", "escape.go"), Kind: UpdateKindDelete}, WhitespaceModeExact)
	if err == nil {
		t.Fatal("WriteFile(outside root) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "parent traversal") && !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("error = %q, want outside-root validation error", err)
	}
}

func TestWriteFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	linkPath := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	err := WriteFile(root, FileUpdate{Path: filepath.Join("linked", "escape.go"), Content: "package main\n"}, WhitespaceModeExact)
	if err == nil {
		t.Fatal("WriteFile(symlink escape) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("error = %q, want project-root escape error", err)
	}
}

func TestDeleteFileRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	linkPath := filepath.Join(root, "linked")
	if err := os.Symlink(outside, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	if err := os.WriteFile(filepath.Join(outside, "escape.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile(outside) error = %v", err)
	}

	err := WriteFile(root, FileUpdate{Path: filepath.Join("linked", "escape.go"), Kind: UpdateKindDelete}, WhitespaceModeExact)
	if err == nil {
		t.Fatal("WriteFile(delete symlink escape) error = nil, want error")
	}
	if !strings.Contains(err.Error(), "escapes project root") {
		t.Fatalf("error = %q, want project-root escape error", err)
	}
}

func TestParseAIResponse_BasicFencedBlocks(t *testing.T) {
	input := "```go\n--- File: internal/service/order.go ---\nfunc handle() {}\n--- End File ---\n```\n--- File: internal/handler/order.go ---\n\treturn nil\n--- End File ---\n"

	updates := ParseAIResponse(input)
	if len(updates) != 2 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 2", len(updates))
	}

	if updates[0].Path != "internal/service/order.go" {
		t.Fatalf("first path = %q, want %q", updates[0].Path, "internal/service/order.go")
	}
	if updates[0].Content != "func handle() {}\n" {
		t.Fatalf("first content = %q", updates[0].Content)
	}

	if updates[1].Path != "internal/handler/order.go" {
		t.Fatalf("second path = %q, want %q", updates[1].Path, "internal/handler/order.go")
	}
	if updates[1].Content != "\treturn nil\n" {
		t.Fatalf("second content = %q", updates[1].Content)
	}
}

func TestParseAIResponse_MixedWriteAndDeleteBlocks(t *testing.T) {
	input := "Notes before\n--- File: internal/service/order.go ---\nfunc handle() {}\n--- End File ---\n--- Delete File: internal/service/old_order.go ---\nNotes after\n"

	result := ParseAIResponseDetailed(input)
	if len(result.Updates) != 2 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 2", len(result.Updates))
	}
	if result.Updates[0].Kind != UpdateKindWrite {
		t.Fatalf("first update kind = %q, want write", result.Updates[0].Kind)
	}
	if result.Updates[1].Kind != UpdateKindDelete {
		t.Fatalf("second update kind = %q, want delete", result.Updates[1].Kind)
	}
	if result.Updates[1].Path != "internal/service/old_order.go" {
		t.Fatalf("delete path = %q", result.Updates[1].Path)
	}
	if result.Text != "Notes before\nNotes after" {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestParseAIResponse_WordLikeMarkersInsideFileContent(t *testing.T) {
	input := "--- File: docs/spec.md ---\n# AIBadger Specification\n\n--- Delete File: docs/obsolete.md ---\n\n--- File: docs/example.md ---\nThis is literal content.\n--- End File ---\n"

	result := ParseAIResponseDetailed(input)
	if len(result.Updates) != 1 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 1", len(result.Updates))
	}
	if result.Updates[0].Path != "docs/spec.md" {
		t.Fatalf("path = %q, want %q", result.Updates[0].Path, "docs/spec.md")
	}
	want := "# AIBadger Specification\n\n--- Delete File: docs/obsolete.md ---\n\n--- File: docs/example.md ---\nThis is literal content.\n"
	if result.Updates[0].Content != want {
		t.Fatalf("content = %q, want %q", result.Updates[0].Content, want)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v, want none", result.Errors)
	}
}

func TestParseAIResponse_ProtocolExamplesInsideMarkdownContent(t *testing.T) {
	input := "--- File: docs/spec.md ---\n" +
		"```text\n" +
		"# AIBadger Specification\n\n" +
		"[CONTEXT]\n" +
		"--- File: <path> (Extracted Block) ---\n" +
		"<Exact text pulled from disk>\n" +
		"--- End File ---\n\n" +
		"Output format rules:\n" +
		"--- File: <path/from/project_root> ---\n" +
		"<full updated file contents>\n" +
		"--- End File ---\n\n" +
		"--- Delete File: <path/from/project_root> ---\n\n" +
		"More spec text.\n" +
		"```\n" +
		"--- End File ---\n"

	result := ParseAIResponseDetailed(input)
	if len(result.Updates) != 1 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 1", len(result.Updates))
	}
	if result.Updates[0].Path != "docs/spec.md" {
		t.Fatalf("path = %q, want %q", result.Updates[0].Path, "docs/spec.md")
	}
	if strings.Contains(result.Updates[0].Path, "<") {
		t.Fatalf("path = %q, want concrete markdown path", result.Updates[0].Path)
	}
	for _, update := range result.Updates {
		if update.Kind == UpdateKindDelete {
			t.Fatalf("unexpected delete update: %+v", update)
		}
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v, want none", result.Errors)
	}
	if !strings.Contains(result.Updates[0].Content, "--- Delete File: <path/from/project_root> ---") {
		t.Fatalf("markdown content did not preserve delete example: %q", result.Updates[0].Content)
	}
	if !strings.Contains(result.Updates[0].Content, "--- File: <path> (Extracted Block) ---") {
		t.Fatalf("markdown content did not preserve extracted block example: %q", result.Updates[0].Content)
	}
}

func TestParseAIResponse_MarkdownBlockClosesBeforeTrailingNotes(t *testing.T) {
	input := "--- File: docs/spec.md ---\n" +
		"```text\n" +
		"--- File: <path/from/project_root> ---\n" +
		"--- End File ---\n" +
		"```\n" +
		"--- End File ---\n" +
		"AI notes after the file block.\n"

	result := ParseAIResponseDetailed(input)
	if len(result.Updates) != 1 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 1", len(result.Updates))
	}
	if result.Updates[0].Path != "docs/spec.md" {
		t.Fatalf("path = %q, want %q", result.Updates[0].Path, "docs/spec.md")
	}
	if result.Text != "AI notes after the file block." {
		t.Fatalf("text = %q, want trailing notes", result.Text)
	}
}

func TestParseAIResponse_IgnoresPlaceholderWriteBlocks(t *testing.T) {
	result := ParseAIResponseDetailed("--- File: <path/from/project_root> ---\nplaceholder\n--- End File ---")
	if len(result.Updates) != 0 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 0", len(result.Updates))
	}
	if len(result.Errors) != 0 {
		t.Fatalf("ParseAIResponseDetailed() errors = %v, want none", result.Errors)
	}
}

func TestParseAIResponse_IgnoresPlaceholderDeleteBlocks(t *testing.T) {
	result := ParseAIResponseDetailed("--- Delete File: <path/from/project_root> ---")
	if len(result.Updates) != 0 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 0", len(result.Updates))
	}
	if len(result.Errors) != 0 {
		t.Fatalf("ParseAIResponseDetailed() errors = %v, want none", result.Errors)
	}
}

func TestParseAIResponse_IgnoresExtractedBlockContextHeader(t *testing.T) {
	result := ParseAIResponseDetailed("--- File: <path> (Extracted Block) ---\ncontent\n--- End File ---")
	if len(result.Updates) != 0 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 0", len(result.Updates))
	}
	if len(result.Errors) != 0 {
		t.Fatalf("ParseAIResponseDetailed() errors = %v, want none", result.Errors)
	}
}

func TestParseAIResponse_EmptyFileWriteStaysWrite(t *testing.T) {
	result := ParseAIResponseDetailed("--- File: empty.go ---\n--- End File ---")
	if len(result.Updates) != 1 {
		t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 1", len(result.Updates))
	}
	if result.Updates[0].Kind != UpdateKindWrite {
		t.Fatalf("update kind = %q, want write", result.Updates[0].Kind)
	}
	if result.Updates[0].Content != "" {
		t.Fatalf("content = %q, want empty", result.Updates[0].Content)
	}
}

func TestParseAIResponseRejectsMalformedDeleteBlock(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "empty path", input: "--- Delete File:  ---"},
		{name: "absolute path", input: "--- Delete File: /tmp/escape.go ---"},
		{name: "parent traversal", input: "--- Delete File: ../escape.go ---"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := ParseAIResponseDetailed(tc.input)
			if len(result.Updates) != 0 {
				t.Fatalf("ParseAIResponseDetailed() returned %d updates, want 0", len(result.Updates))
			}
			if len(result.Errors) == 0 {
				t.Fatal("ParseAIResponseDetailed() errors = 0, want validation error")
			}
		})
	}
}

func TestParseAIResponse_IgnoresPlainText(t *testing.T) {
	updates := ParseAIResponse("This is analysis only.\nNo files here.\n")
	if len(updates) != 0 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 0", len(updates))
	}
}

func TestParseAIResponse_EmptyInput(t *testing.T) {
	updates := ParseAIResponse("")
	if len(updates) != 0 {
		t.Fatalf("ParseAIResponse(\"\") returned %d updates, want 0", len(updates))
	}
}

func TestParseAIResponse_OnlyFenceMarkers(t *testing.T) {
	input := "```\n```\n```python\n```"
	updates := ParseAIResponse(input)
	if len(updates) != 0 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 0", len(updates))
	}
}

func TestParseAIResponse_FenceWithLanguage(t *testing.T) {
	input := "```python\n--- File: app.py ---\nprint('hello')\n--- End File ---\n```"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	if updates[0].Path != "app.py" {
		t.Fatalf("path = %q, want %q", updates[0].Path, "app.py")
	}
	if updates[0].Content != "print('hello')\n" {
		t.Fatalf("content = %q", updates[0].Content)
	}
}

func TestParseAIResponse_MultipleFencesAroundFile(t *testing.T) {
	input := "```\n```go\n--- File: main.go ---\nfunc main() {}\n--- End File ---\n```\n```"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
}

func TestParseAIResponse_MissingEndFileMarker(t *testing.T) {
	input := "--- File: incomplete.go ---\nfunc missing() {}\n"
	updates := ParseAIResponse(input)
	if len(updates) != 0 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 0 (missing End File)", len(updates))
	}
}

func TestParseAIResponse_IncompleteFileMarker(t *testing.T) {
	input := "--- File: partial\n--- File: also-partial ---"
	updates := ParseAIResponse(input)
	if len(updates) != 0 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 0", len(updates))
	}
}

func TestParseAIResponse_TrailingWhitespace(t *testing.T) {
	input := "--- File: spaced.go ---  \ncontent\n--- End File ---  "
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	if updates[0].Path != "spaced.go" {
		t.Fatalf("path = %q, want %q", updates[0].Path, "spaced.go")
	}
}

func TestParseAIResponse_FilePathWithSpaces(t *testing.T) {
	input := "--- File: path/with spaces/file.go ---\ncontent\n--- End File ---"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	if updates[0].Path != "path/with spaces/file.go" {
		t.Fatalf("path = %q", updates[0].Path)
	}
}

func TestParseAIResponse_DuplicatePaths(t *testing.T) {
	input := "--- File: duplicated.go ---\nfirst content\n--- End File ---\n--- File: duplicated.go ---\nsecond content\n--- End File ---"
	updates := ParseAIResponse(input)
	if len(updates) != 2 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 2", len(updates))
	}
	if updates[0].Content != "first content\n" {
		t.Fatalf("first content = %q", updates[0].Content)
	}
	if updates[1].Content != "second content\n" {
		t.Fatalf("second content = %q", updates[1].Content)
	}
}

func TestParseAIResponse_LargeInput(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("--- File: large.go ---\n")
	for i := 0; i < 1000; i++ {
		sb.WriteString("func function")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString("() {}\n")
	}
	sb.WriteString("--- End File ---")

	updates := ParseAIResponse(sb.String())
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	expectedLines := 1000
	actualLines := strings.Count(updates[0].Content, "\n")
	if actualLines != expectedLines {
		t.Fatalf("content has %d lines, want %d", actualLines, expectedLines)
	}
}

func TestParseAIResponse_OnlyFileMarkerNoContent(t *testing.T) {
	input := "--- File: empty.go ---\n--- End File ---"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	if updates[0].Content != "" {
		t.Fatalf("content = %q, want empty", updates[0].Content)
	}
	if updates[0].Kind != UpdateKindWrite {
		t.Fatalf("kind = %q, want write", updates[0].Kind)
	}
}

func TestParseAIResponse_ContentAfterLastEndFile(t *testing.T) {
	input := "--- File: first.go ---\nfirst\n--- End File ---\nSome analysis text after\n--- File: second.go ---\nsecond\n--- End File ---\nMore trailing text"
	updates := ParseAIResponse(input)
	if len(updates) != 2 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 2", len(updates))
	}
	if updates[0].Content != "first\n" {
		t.Fatalf("first content = %q", updates[0].Content)
	}
	if updates[1].Content != "second\n" {
		t.Fatalf("second content = %q", updates[1].Content)
	}
}

func TestParseAIResponseDetailed_PreservesTextOutsideFileBlocks(t *testing.T) {
	input := "Summary before\n--- File: first.go ---\nfirst\n--- End File ---\n@@\nMore notes after\n"

	result := ParseAIResponseDetailed(input)

	if len(result.Updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(result.Updates))
	}
	if result.Text != "Summary before\n@@\nMore notes after" {
		t.Fatalf("text = %q", result.Text)
	}
}

func TestParseAIResponse_NewlineVariations(t *testing.T) {
	input := "--- File: newlines.go ---\nline1\n\nline2\n\n\nline3\n--- End File ---"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	expected := "line1\n\nline2\n\n\nline3\n"
	if updates[0].Content != expected {
		t.Fatalf("content = %q, want %q", updates[0].Content, expected)
	}
}

func TestParseAIResponse_PreservedIndentation(t *testing.T) {
	input := "--- File: indent.go ---\n\tfunc indented() {\n\t\treturn 1\n\t}\n--- End File ---"
	updates := ParseAIResponse(input)
	if len(updates) != 1 {
		t.Fatalf("ParseAIResponse() returned %d updates, want 1", len(updates))
	}
	if !strings.Contains(updates[0].Content, "\t\t") {
		t.Fatalf("indentation not preserved: %q", updates[0].Content)
	}
}
