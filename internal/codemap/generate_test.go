package codemap

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReportFilename(t *testing.T) {
	for _, name := range []string{"project", "my project", "日本語", "CON", "nul", "résumé", "�"} {
		want := "badger-codemap-" + name + ".html"
		if got := reportFilename(name); got != want {
			t.Errorf("%q: got %q want %q", name, got, want)
		}
	}
	for _, name := range []string{"", "/", `\`, ".", ".."} {
		if got := reportFilename(name); got != "badger-codemap-root.html" {
			t.Errorf("root %q: %q", name, got)
		}
	}
	for _, name := range []string{`bad:name*?<>"|`, "tab\tname", "line\nname", "name. ", "...", "invalid\xff", strings.Repeat("界", 100), strings.Repeat("x", 255)} {
		got := reportFilename(name)
		if len(got) > 255 || !utf8.ValidString(got) || strings.ContainsAny(got, `<>:"/\|?*`) || strings.ContainsAny(got, "\t\n\r") {
			t.Errorf("unsafe filename %q", got)
		}
		if got != reportFilename(name) {
			t.Fatal("nondeterministic fallback")
		}
	}
	if reportFilename("bad:name") == reportFilename("bad?name") {
		t.Fatal("sanitized names collide")
	}
}

func TestGenerateCOBOLReportAccounting(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "COBOL-only"
		if mixed {
			name = "mixed languages"
		}
		t.Run(name, func(t *testing.T) {
			source, caller := t.TempDir(), t.TempDir()
			writeTestFile(t, source, "programs/MAIN.CBL", "       IDENTIFICATION DIVISION.\n\n       PROGRAM-ID. MAIN.\n")
			writeTestFile(t, source, "programs/copybooks/record.cpy", "       01 RECORD-DATA PIC X.\n \t\n")
			writeTestFile(t, source, "programs/copybooks/blank.CPY", " \n")
			wantLines, wantFiles, wantLanguages := 3, 3, 1
			if mixed {
				writeTestFile(t, source, "main.go", "package main\n")
				wantLines++
				wantFiles++
				wantLanguages++
			}
			t.Chdir(caller)
			path, err := Generate(source)
			if err != nil {
				t.Fatal(err)
			}
			page, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			_, data, ok := strings.Cut(string(page), `<script id="map-data" type="application/json">`)
			if !ok {
				t.Fatal("report data missing")
			}
			data, _, _ = strings.Cut(data, "</script>")
			var root directory
			if err := json.Unmarshal([]byte(data), &root); err != nil {
				t.Fatal(err)
			}
			if root.Lines != wantLines || root.Files != wantFiles || len(root.Languages) != wantLanguages || root.Languages["COBOL"] != (languageStats{Lines: 3, Files: 3}) {
				t.Fatalf("report totals: %+v", root)
			}
			if mixed && root.Languages["Go"] != (languageStats{Lines: 1, Files: 1}) {
				t.Fatalf("mixed report languages: %v", root.Languages)
			}
			if len(root.Children) != 1 {
				t.Fatalf("report folders: %v", root.Children)
			}
			programs := root.Children[0]
			if programs.Path != "programs" || programs.Lines != 3 || programs.Files != 3 || programs.DirectLines != 2 || programs.DirectFiles != 1 || programs.DirectByLang["COBOL"] != (languageStats{Lines: 2, Files: 1}) || len(programs.Children) != 1 {
				t.Fatalf("program folder accounting: %+v", programs)
			}
			copies := programs.Children[0]
			if copies.Path != "programs/copybooks" || copies.Lines != 1 || copies.Files != 2 || copies.DirectByLang["COBOL"] != (languageStats{Lines: 1, Files: 2}) || len(copies.FileList) != 2 || copies.FileList[0] != (sourceFile{Name: "record.cpy", Lines: 1}) || copies.FileList[1] != (sourceFile{Name: "blank.CPY", Lines: 0}) {
				t.Fatalf("copybook folder accounting: %+v", copies)
			}
		})
	}
}

func assertOnlyReport(t *testing.T, caller, destination, content string) {
	t.Helper()
	entries, err := os.ReadDir(caller)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || filepath.Join(caller, entries[0].Name()) != destination {
		t.Fatalf("unexpected output artifacts: %v", entries)
	}
	bytes, err := os.ReadFile(destination)
	if err != nil || string(bytes) != content {
		t.Fatalf("report changed: %v", err)
	}
}

func TestGenerateInCallerDirectoryAndReplace(t *testing.T) {
	caller, sourceParent := t.TempDir(), t.TempDir()
	source := filepath.Join(sourceParent, "日本語 project")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	sourceText := "// PRIVATE_SOURCE_SENTINEL\ncode\n"
	writeTestFile(t, source, "src/main.go", sourceText)
	// Use relative input and a trailing separator, without Git metadata.
	relative, err := filepath.Rel(caller, source)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(caller)
	cwdBefore, _ := os.Getwd()
	got, err := Generate(relative + string(filepath.Separator))
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(caller, "badger-codemap-日本語 project.html")
	if got != want {
		t.Fatalf("path=%q want=%q", got, want)
	}
	page, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), source) || strings.Contains(string(page), sourceText) || strings.Contains(string(page), "PRIVATE_SOURCE_SENTINEL") {
		t.Fatal("source contents or absolute source path embedded")
	}
	start := strings.Index(string(page), `<script id="map-data" type="application/json">`) + len(`<script id="map-data" type="application/json">`)
	end := start + strings.Index(string(page[start:]), "</script>")
	var root directory
	if err := json.Unmarshal(page[start:end], &root); err != nil {
		t.Fatal(err)
	}
	if root.Path != "." || root.Children[0].Path != "src" || root.Children[0].FileList[0].Name != "main.go" || root.Lines != 2 {
		t.Fatal("unexpected report metadata")
	}
	assertOnlyReport(t, caller, got, string(page))
	writeTestFile(t, source, "src/main.go", "// changed\ncode\nextra\n")
	second, err := Generate(source)
	if err != nil || second != got {
		t.Fatalf("replace: %q %v", second, err)
	}
	changed, err := os.ReadFile(got)
	if err != nil || string(changed) == string(page) {
		t.Fatal("report not replaced")
	}
	assertOnlyReport(t, caller, got, string(changed))
	cwdAfter, _ := os.Getwd()
	if cwdBefore != cwdAfter {
		t.Fatal("generator changed process directory")
	}
	current, err := os.ReadFile(filepath.Join(source, "src/main.go"))
	if err != nil || string(current) != "// changed\ncode\nextra\n" {
		t.Fatal("generation mutated source")
	}
}

func TestGenerationErrorsPreserveExistingReport(t *testing.T) {
	for _, kind := range []string{"empty", "blank", "missing", "not-directory", "scan-budget", "render-budget", "render-depth", "publish"} {
		t.Run(kind, func(t *testing.T) {
			caller := t.TempDir()
			source := filepath.Join(t.TempDir(), "project")
			if err := os.Mkdir(source, 0o700); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(caller, reportFilename("project"))
			if err := os.WriteFile(destination, []byte("old report"), 0o600); err != nil {
				t.Fatal(err)
			}
			scan, render, publish := scanProject, renderHTML, publishHTML
			want := "nonblank"
			switch kind {
			case "blank":
				writeTestFile(t, source, "blank.go", " \n\t\n")
			case "missing":
				source += "-missing"
				want = "scan source directory"
			case "not-directory":
				writeTestFile(t, source, "file.go", "code\n")
				source = filepath.Join(source, "file.go")
				want = "not a directory"
			case "scan-budget":
				writeTestFile(t, source, "one.go", "code\n")
				scan = func(path string) (*directory, error) {
					return scanProjectWithLimits(path, scanLimits{0, 1, 0, maxSourceBytes})
				}
				want = "file limit"
			case "render-budget":
				writeTestFile(t, source, "one.go", "code\n")
				render = func(root *directory) ([]byte, error) { return renderHTMLTemplate(root, pageTemplate, 1) }
				want = "HTML size limit"
			case "render-depth":
				scan = func(path string) (*directory, error) {
					root := &directory{Name: "project", Path: ".", Lines: 1}
					node := root
					for i := 0; i <= maxDirectoryDepth; i++ {
						node.Children = []*directory{{Name: "deep"}}
						node = node.Children[0]
					}
					return root, nil
				}
				want = "depth limit"
			case "publish":
				writeTestFile(t, source, "one.go", "code\n")
				publish = func(string, []byte) error { return errors.New("injected publication failure") }
				want = "publication failure"
			}
			got, err := generateIn(source, caller, scan, render, publish)
			if got != "" || err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("unexpected error: %q %v", got, err)
			}
			assertOnlyReport(t, caller, destination, "old report")
		})
	}
	if _, err := generateIn("", t.TempDir(), scanProject, renderHTML, publishHTML); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatal("empty operand accepted")
	}
}

type faultyStage struct {
	*os.File
	kind   string
	closed bool
}

func (f *faultyStage) Write(page []byte) (int, error) {
	if f.kind == "write" {
		n, _ := f.File.Write(page[:len(page)/2])
		return n, errors.New("injected write failure")
	}
	if f.kind == "short-write" {
		return f.File.Write(page[:len(page)/2])
	}
	return f.File.Write(page)
}
func (f *faultyStage) Close() error {
	f.closed = true
	err := f.File.Close()
	if f.kind == "close" {
		return errors.Join(err, errors.New("injected close failure"))
	}
	return err
}

func TestPublicationFailuresPreserveOldOutputAndCleanStage(t *testing.T) {
	for _, kind := range []string{"inspect", "create", "write", "short-write", "close", "replace", "remove"} {
		t.Run(kind, func(t *testing.T) {
			caller := t.TempDir()
			destination := filepath.Join(caller, "report.html")
			if err := os.WriteFile(destination, []byte("old report"), 0o600); err != nil {
				t.Fatal(err)
			}
			var stage *faultyStage
			replaced := false
			ops := publicationOps{
				lstat: os.Lstat,
				create: func(dir string) (stagedReport, error) {
					if dir != caller {
						t.Fatal("stage must be beside destination")
					}
					if kind == "create" {
						return nil, errors.New("injected create failure")
					}
					file, err := os.CreateTemp(dir, ".badger-map-*.tmp")
					if err != nil {
						return nil, err
					}
					stage = &faultyStage{File: file, kind: kind}
					return stage, nil
				},
				replace: func(source, target string) error {
					replaced = true
					if stage == nil || !stage.closed {
						t.Fatal("publication before successful close")
					}
					return errors.New("injected replace failure")
				},
				remove: func(path string) error {
					err := os.Remove(path)
					if kind == "remove" {
						return errors.Join(err, errors.New("injected remove failure"))
					}
					return err
				},
			}
			if kind == "inspect" {
				ops.lstat = func(string) (os.FileInfo, error) { return nil, errors.New("injected inspect failure") }
			}
			err := publishWithOps(destination, []byte("new report"), ops)
			if err == nil {
				t.Fatal("failure was not propagated")
			}
			if kind == "short-write" && !errors.Is(err, io.ErrShortWrite) {
				t.Fatalf("missing short-write error: %v", err)
			}
			if kind == "remove" && !strings.Contains(err.Error(), "remove staged report") {
				t.Fatal("cleanup error hidden")
			}
			if stage != nil && !stage.closed {
				t.Fatal("stage left open")
			}
			if replaced && kind != "replace" && kind != "remove" {
				t.Fatal("published after an earlier failure")
			}
			assertOnlyReport(t, caller, destination, "old report")
		})
	}
}

func TestPublicationInvalidDestinationAndOversize(t *testing.T) {
	caller := t.TempDir()
	destination := filepath.Join(caller, "report.html")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := publishHTML(destination, []byte("page")); err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("directory overwrite: %v", err)
	}
	if err := publishHTML(filepath.Join(caller, "missing", "report.html"), []byte("page")); err == nil {
		t.Fatal("missing output directory accepted")
	}
	invoked := false
	err := publishWithOps(destination, make([]byte, maxHTMLBytes+1), publicationOps{lstat: func(string) (os.FileInfo, error) { invoked = true; return nil, nil }})
	if err == nil || invoked {
		t.Fatal("oversize must fail before touching output")
	}
}

func TestRepeatedGenerationInsideSourceExcludesOnlyDestination(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested output"}[nested], func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "project")
			if err := os.Mkdir(source, 0o700); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, source, "main.go", "// source\ncode\n")
			writeTestFile(t, source, "page.html", "<h1>ordinary HTML counts</h1>\n")
			writeTestFile(t, source, "badger-codemap-other.html", "<h1>other report filename still counts</h1>\n")
			caller := source
			if nested {
				caller = filepath.Join(source, "reports")
				if err := os.Mkdir(caller, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(caller)
			input := "."
			if nested {
				input = ".."
			}
			first, err := Generate(input)
			if err != nil {
				t.Fatal(err)
			}
			initial, err := os.ReadFile(first)
			if err != nil {
				t.Fatal(err)
			}
			second, err := Generate(input)
			if err != nil || second != first {
				t.Fatalf("second generation: %q %v", second, err)
			}
			again, err := os.ReadFile(second)
			if err != nil || string(again) != string(initial) {
				t.Fatal("own report contaminated subsequent scan")
			}
			root, err := scanProjectExcludingReport(source, first)
			if err != nil || root.Files != 3 || root.Lines != 4 || root.Languages["HTML"].Files != 2 {
				t.Fatalf("ordinary HTML/report-like source was excluded: %+v %v", root, err)
			}
		})
	}
}

func TestReportExclusionPrecedesQuotas(t *testing.T) {
	source := t.TempDir()
	destination := filepath.Join(source, reportFilename(filepath.Base(source)))
	if err := os.WriteFile(destination, []byte("old report exceeds source byte budget\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, source, "main.go", "x\n")
	root, err := scanProjectWithReportExclusion(source, scanLimits{0, 1, 1, 2}, (*os.File).ReadDir, destination)
	if err != nil || root.Files != 1 || root.Lines != 1 {
		t.Fatalf("report consumed quota: %+v %v", root, err)
	}
}
