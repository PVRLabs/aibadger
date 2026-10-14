package codemap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanProjectCountsAndAggregates(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n\n// comment\nfunc main() {}\n")
	writeTestFile(t, root, "README.md", "# Project\n\nSome text.\n")
	writeTestFile(t, root, "notes.txt", "this must be ignored\n")
	writeTestFile(t, root, "unknown.xyz", "this must be ignored too\n")
	writeTestFile(t, root, "empty.py", "  \n\t\n")
	writeTestFile(t, root, "web/app.tsx", "\nexport function App() {\n  return <main />;\n}\n")
	writeTestFile(t, root, "web/styles.scss", "$ink: #111;\n\nbody { color: $ink; }\n")
	writeTestFile(t, root, "web/nested/query.sql", "select 1;\n\n-- retained comment\n")
	writeTestFile(t, root, "node_modules/pkg/index.js", "one\ntwo\nthree\n")
	writeTestFile(t, root, "target/generated.rs", "one\ntwo\n")

	got, err := scanProject(root)
	if err != nil {
		t.Fatalf("scanProject: %v", err)
	}
	if got.Lines != 12 {
		t.Fatalf("root lines = %d, want 12", got.Lines)
	}
	if got.Files != 6 {
		t.Fatalf("root files = %d, want 6 (including an all-blank recognized file)", got.Files)
	}
	if got.DirectLines != 5 || got.DirectFiles != 3 {
		t.Fatalf("direct totals = %d lines/%d files, want 5/3", got.DirectLines, got.DirectFiles)
	}

	wantLanguages := map[string]languageStats{
		"Go":         {Lines: 3, Files: 1},
		"Markdown":   {Lines: 2, Files: 1},
		"Python":     {Lines: 0, Files: 1},
		"TypeScript": {Lines: 3, Files: 1},
		"SCSS":       {Lines: 2, Files: 1},
		"SQL":        {Lines: 2, Files: 1},
	}
	if len(got.Languages) != len(wantLanguages) {
		t.Fatalf("languages = %#v", got.Languages)
	}
	for language, want := range wantLanguages {
		if got.Languages[language] != want {
			t.Errorf("%s = %#v, want %#v", language, got.Languages[language], want)
		}
	}

	if len(got.Children) != 1 || got.Children[0].Path != "web" {
		t.Fatalf("root children = %#v, want only web", got.Children)
	}
	web := got.Children[0]
	if web.Lines != 7 || web.Files != 3 {
		t.Fatalf("web totals = %d lines/%d files, want 7/3", web.Lines, web.Files)
	}
	if len(web.Children) != 1 || web.Children[0].Path != "web/nested" || web.Children[0].Lines != 2 {
		t.Fatalf("web children = %#v", web.Children)
	}
}

func TestSupportedExtensions(t *testing.T) {
	want := []string{
		".go", ".java", ".kt", ".js", ".ts", ".jsx", ".tsx", ".py", ".rs",
		".c", ".cpp", ".cs", ".swift", ".m", ".rb", ".php", ".dart", ".scala",
		".sh", ".sql", ".html", ".css", ".scss", ".sass", ".vue", ".svelte", ".md",
	}
	for _, extension := range want {
		if extensionLanguages[extension] == "" {
			t.Errorf("extension %s is not recognized", extension)
		}
	}
	if _, ok := extensionLanguages[".txt"]; ok {
		t.Error(".txt must not be recognized")
	}
}

func writeTestFile(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", relative, err)
	}
}

func TestFileListKeepsLargestFiftyAndFullTotals(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 60; i++ {
		writeTestFile(t, root, fmt.Sprintf("file-%02d.go", i), strings.Repeat("code\n", i))
	}
	writeTestFile(t, root, "child/inside.go", "child\n")
	writeTestFile(t, root, "ignored.txt", strings.Repeat("ignored\n", 100))
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if tree.DirectFiles != 60 || tree.Files != 61 || tree.DirectLines != 1770 {
		t.Fatalf("incorrect totals: %#v", tree)
	}
	if len(tree.FileList) != 50 {
		t.Fatalf("file list length = %d", len(tree.FileList))
	}
	for i, file := range tree.FileList {
		if file.Name != fmt.Sprintf("file-%02d.go", 59-i) || file.Lines != 59-i {
			t.Fatalf("file %d = %#v", i, file)
		}
	}
	if len(tree.Children[0].FileList) != 1 || tree.Children[0].FileList[0].Name != "inside.go" {
		t.Fatal("child file missing")
	}
	data, err := json.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	var decoded directory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.FileList) != 50 || decoded.FileList[0].Name != "file-59.go" {
		t.Fatal("file details missing in serialized data")
	}
}

func TestFileListIncludesBlankFilesAndSortsTiesByName(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "z.go", "code\n")
	writeTestFile(t, root, "a.go", "code\n")
	writeTestFile(t, root, "empty.go", " \n")
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.FileList) != 3 || tree.FileList[0].Name != "a.go" || tree.FileList[1].Name != "z.go" || tree.FileList[2].Lines != 0 {
		t.Fatalf("files = %#v", tree.FileList)
	}
}

func TestScanIncludesSourceInBinDirectories(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "bin/start.sh", "#!/bin/sh\necho hello\n")
	writeTestFile(t, root, "tools/bin/helper.py", "print('hello')\n")
	writeTestFile(t, root, "bin/program.exe", "not source\n")
	writeTestFile(t, root, "node_modules/bin/ignored.js", "ignored\n")
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Lines != 3 || tree.Files != 2 {
		t.Fatalf("totals = %d lines / %d files", tree.Lines, tree.Files)
	}
	if len(tree.Children) != 2 || tree.Children[0].Path != "bin" || tree.Children[1].Children[0].Path != "tools/bin" {
		t.Fatalf("bin source directories not preserved: %#v", tree.Children)
	}
}

func TestLeafWithBlankDescendantsKeepsRecursiveAndDirectCountsDistinct(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "package main\n")
	writeTestFile(t, root, "src/blank/empty.go", " \n\t\n")
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	leaf := tree.Children[0]
	if len(leaf.Children) != 0 || leaf.Files != 2 || leaf.DirectFiles != 1 || len(leaf.FileList) != 1 || leaf.FileList[0].Name != "main.go" {
		t.Fatalf("incorrect leaf accounting: %#v", leaf)
	}

}

func TestPythonEnvironmentsAreExcludedAtAnyDepth(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{".venv", "venv", ".tox", ".nox"} {
		writeTestFile(t, root, name+"/lib/dependency.py", "dependency\n")
		writeTestFile(t, root, "src/"+name+"/lib/dependency.py", "dependency\n")
	}
	writeTestFile(t, root, "src/app.py", "print('app')\n")
	writeTestFile(t, root, "src/venv_tools/helper.py", "print('helper')\n")
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Lines != 2 || tree.Files != 2 || tree.Languages["Python"].Lines != 2 {
		t.Fatalf("environment source leaked into totals: %#v", tree)
	}
	if len(tree.Children) != 1 || len(tree.Children[0].Children) != 1 || tree.Children[0].Children[0].Name != "venv_tools" {
		t.Fatal("wrong visible directories")
	}
}

func TestAmbiguousHeadersUseSharedLanguageLabel(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"shared.h", "upper.H", "impl.c", "impl.cpp", "types.hpp", "types.hh", "types.hxx"} {
		writeTestFile(t, root, name, "code\n")
	}
	tree, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if tree.Lines != 7 || tree.Files != 7 {
		t.Fatal("header labeling changed geometry totals")
	}
	for name, want := range map[string]int{"C/C++": 2, "C": 1, "C++": 4} {
		if tree.Languages[name].Lines != want || tree.Languages[name].Files != want {
			t.Fatalf("%s: %#v", name, tree.Languages[name])
		}
	}
}
