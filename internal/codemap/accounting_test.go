package codemap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLanguageContract(t *testing.T) {
	groups := map[string]string{
		"Ada":           "ads adb ada",
		"JCL":           "jcl",
		"SystemVerilog": "sv svh",
		"VHDL":          "vhd vhdl",
		"Fortran":       "f77 f90 f95 f03 f08 fpp ftn",
		"PL/I":          "pli pl1",
		"RPG":           "rpgle sqlrpgle rpgleinc sqlrpg",
		"IBM CL":        "clle clp clp38",
		"ABAP":          "abap",
		"JSON":          "json webmanifest",
		"YAML":          "yaml yml",
		"TOML":          "toml",
		"XML":           "xml",
		"INI":           "ini",
		"Config":        "conf",
		"Properties":    "properties",
		"COBOL":         "cbl cob cobol ccp cpy",
		"Go":            "go", "Java": "java", "Kotlin": "kt kts", "JavaScript": "js mjs cjs jsx",
		"TypeScript": "ts mts cts tsx", "Python": "py pyw", "Rust": "rs", "C": "c", "C/C++": "h",
		"C++": "cc cpp cxx hh hpp hxx", "C#": "cs", "Swift": "swift", "Objective-C": "m", "Objective-C++": "mm",
		"Ruby": "rb", "PHP": "php", "Dart": "dart", "Scala": "scala sc", "Shell": "sh bash zsh fish",
		"SQL": "sql", "HTML": "html htm", "CSS": "css", "SCSS": "scss", "Sass": "sass", "Vue": "vue",
		"Svelte": "svelte", "Markdown": "md markdown mdx", "Lua": "lua", "R": "r", "Perl": "pl pm",
		"Elixir": "ex exs", "Erlang": "erl hrl", "Haskell": "hs lhs", "Clojure": "clj", "ClojureScript": "cljs",
		"Groovy": "groovy gradle", "F#": "fs fsx", "Visual Basic": "vb", "Zig": "zig", "Solidity": "sol",
		"Terraform": "tf", "Protocol Buffers": "proto",
	}
	root := t.TempDir()
	expected := map[string]languageStats{}
	extensions := 0
	for language, list := range groups {
		for _, ext := range strings.Fields(list) {
			extensions++
			writeTestFile(t, root, "lower."+ext, "source\n")
			writeTestFile(t, root, "nested/upper."+strings.ToUpper(ext), "source\n")
			expected[language] = languageStats{Lines: expected[language].Lines + 2, Files: expected[language].Files + 2}
		}
	}
	writeTestFile(t, root, "ignored.txt", "ignored\n")
	writeTestFile(t, root, "ignored.unknown", "ignored\n")
	writeTestFile(t, root, "Makefile", "ignored\n")
	got, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(extensionLanguages) != extensions {
		t.Fatalf("extension table has %d entries, want %d", len(extensionLanguages), extensions)
	}
	assertTotals := func(node *directory) {
		t.Helper()
		if node.Lines != extensions*2 || node.Files != extensions*2 || node.DirectLines != extensions || node.DirectFiles != extensions || !reflect.DeepEqual(node.Languages, expected) {
			t.Fatalf("recursive totals: %+v, want languages=%v", node, expected)
		}
		if len(node.Children) != 1 || node.Children[0].Path != "nested" || node.Children[0].Lines != extensions || node.Children[0].Files != extensions {
			t.Fatalf("nested totals: %+v", node.Children)
		}
		for language, total := range expected {
			want := languageStats{Lines: total.Lines / 2, Files: total.Files / 2}
			if node.DirectByLang[language] != want || node.Children[0].Languages[language] != want {
				t.Errorf("%s direct/nested totals: %v / %v, want %v", language, node.DirectByLang[language], node.Children[0].Languages[language], want)
			}
		}
	}
	assertTotals(got)
	page, err := renderHTML(got)
	if err != nil {
		t.Fatal(err)
	}
	_, data, ok := strings.Cut(string(page), `<script id="map-data" type="application/json">`)
	if !ok {
		t.Fatal("report data missing")
	}
	data, _, _ = strings.Cut(data, "</script>")
	var decoded directory
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		t.Fatal(err)
	}
	assertTotals(&decoded)
}

func TestDependencyLockfilesExcludedBeforeQuotas(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"package-lock.json", "pnpm-lock.yaml", "nested/PACKAGE-LOCK.JSON", "nested/PNPM-LOCK.YAML"} {
		writeTestFile(t, root, name, strings.Repeat("generated\n", 100))
	}
	// Ordinary configuration and near matches remain eligible.
	for _, name := range []string{"config.json", "nested/config.yaml", "custom-package-lock.json", "nested/custom-pnpm-lock.yaml"} {
		writeTestFile(t, root, name, "{}\n")
	}
	// Lockfiles exceed both the remaining file quota and per-file byte limit.
	got, err := scanProjectWithLimits(root, scanLimits{1, 2, 4, 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 4 || got.Lines != 4 || got.DirectFiles != 2 || got.DirectLines != 2 || len(got.FileList) != 2 || len(got.Children) != 1 {
		t.Fatalf("root accounting: %+v", got)
	}
	want := map[string]languageStats{"JSON": {Lines: 2, Files: 2}, "YAML": {Lines: 2, Files: 2}}
	if !reflect.DeepEqual(got.Languages, want) {
		t.Fatalf("language totals: %v, want %v", got.Languages, want)
	}
	nested := got.Children[0]
	if nested.Path != "nested" || nested.Files != 2 || nested.Lines != 2 || len(nested.FileList) != 2 {
		t.Fatalf("nested accounting: %+v", nested)
	}
}

func TestExclusionContract(t *testing.T) {
	names := strings.Fields(".git .hg .svn .idea .vscode .gradle .next .nuxt .svelte-kit .terraform __pycache__ .venv venv .tox .nox bower_components build coverage dist node_modules obj out target vendor")
	if len(skippedDirectories) != len(names) {
		t.Fatalf("exclusion count = %d", len(skippedDirectories))
	}
	root := t.TempDir()
	for _, name := range names {
		writeTestFile(t, root, name+"/ignored.go", "ignored\n")
		writeTestFile(t, root, "src/"+name+"/ignored.go", "ignored\n")
		writeTestFile(t, root, "src/"+name+"_tools/kept.go", "kept\n")
	}
	got, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != len(names) || got.Lines != len(names) {
		t.Fatalf("exclusions/near matches: %#v", got)
	}
}

func TestBlankDescendantsDoNotBecomeDirectLanguages(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/main.go", "code\n")
	writeTestFile(t, root, "src/blank/nested/empty.go", "\u2003\n")
	writeTestFile(t, root, "src/blank/empty.py", "\t\n")
	got, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	leaf := got.Children[0]
	if leaf.Files != 3 || leaf.DirectFiles != 1 || len(leaf.Children) != 0 {
		t.Fatalf("leaf: %#v", leaf)
	}
	want := map[string]languageStats{"Go": {Lines: 1, Files: 1}}
	if !reflect.DeepEqual(leaf.DirectByLang, want) || len(got.DirectByLang) != 0 {
		t.Fatalf("direct languages contaminated: %#v", leaf.DirectByLang)
	}
	if leaf.Languages["Go"].Files != 2 || leaf.Languages["Python"].Files != 1 {
		t.Fatalf("recursive languages: %#v", leaf.Languages)
	}
	// The report's embedded metadata must preserve the same distinction.
	data, err := json.Marshal(leaf)
	if err != nil {
		t.Fatal(err)
	}
	var decoded directory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded.DirectByLang, want) {
		t.Fatal("direct languages lost in report data")
	}
}

func TestBlankAndEmptyTrees(t *testing.T) {
	root := t.TempDir()
	got, err := scanProject(root)
	if err != nil || got.Lines != 0 || got.Files != 0 {
		t.Fatalf("empty tree: %v %v", got, err)
	}
	writeTestFile(t, root, "blank.go", " \n\t\r\n\u2003\u00a0")
	writeTestFile(t, root, "child/blank.py", "")
	got, err = scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines != 0 || got.Files != 2 || got.DirectFiles != 1 || len(got.Children) != 0 {
		t.Fatalf("blank tree: %#v", got)
	}
}

func TestPhysicalLines(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "// comment\r\n\u2003\u00a0\t\r\ncode\rstill same line\nlast")
	got, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lines != 3 {
		t.Fatalf("lines=%d", got.Lines)
	}
}

func TestTopFiftyTiesAndDeterministicChildren(t *testing.T) {
	root := t.TempDir()
	for i := 59; i >= 0; i-- {
		writeTestFile(t, root, fmt.Sprintf("%02d.go", i), "code\n")
	}
	for _, name := range []string{"z", "a", "big"} {
		contents := "code\n"
		if name == "big" {
			contents += "more\n"
		}
		writeTestFile(t, root, name+"/main.go", contents)
	}
	got, err := scanProject(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.DirectFiles != 60 || got.DirectLines != 60 || len(got.FileList) != 50 {
		t.Fatalf("totals/list: %#v", got)
	}
	for i, file := range got.FileList {
		if file.Name != fmt.Sprintf("%02d.go", i) {
			t.Fatalf("file %d: %v", i, file)
		}
	}
	for i, name := range []string{"big", "a", "z"} {
		if got.Children[i].Name != name {
			t.Fatalf("child %d: %s", i, got.Children[i].Name)
		}
	}
}

func TestSymlinksAndInvalidRoots(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeTestFile(t, outside, "external.go", "outside\n")
	writeTestFile(t, root, "real.go", "inside\n")
	for name, target := range map[string]string{"linked.go": filepath.Join(outside, "external.go"), "linked-dir": outside, "cycle": root, "broken.go": filepath.Join(outside, "missing")} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
	}
	got, err := scanProjectWithLimits(root, scanLimits{0, 1, 1, 7})
	if err != nil || got.Files != 1 || got.Lines != 1 {
		t.Fatalf("symlink scan: %v %v", got, err)
	}
	for _, path := range []string{filepath.Join(root, "linked-dir"), filepath.Join(root, "linked-dir") + string(filepath.Separator)} {
		got, err := scanProject(path)
		if err == nil || got != nil || !strings.Contains(err.Error(), "symlink root") {
			t.Fatalf("root %q: %v %v", path, got, err)
		}
	}
	for _, path := range []string{filepath.Join(root, "missing"), filepath.Join(root, "real.go")} {
		got, err := scanProject(path)
		if err == nil || got != nil || !strings.Contains(err.Error(), path) {
			t.Fatalf("invalid root: %v %v", got, err)
		}
	}
}
