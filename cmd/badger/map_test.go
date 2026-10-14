package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMapCommandValidationAndHelp(t *testing.T) {
	for _, args := range [][]string{nil, {""}, {"a", "b"}, {"--output"}, {"--help", "extra"}} {
		var out bytes.Buffer
		err := runMapCommandWithGenerator(args, &out, func(string) (string, error) { t.Fatal("invalid arguments invoked generator"); return "", nil })
		if err == nil || out.Len() != 0 {
			t.Fatalf("args %q: error %v, output %q", args, err, out.String())
		}
	}
	for _, help := range []string{"-h", "--help"} {
		var out, stderr bytes.Buffer
		handled, err := dispatchEarlyCommand([]string{"map", help}, &out, &stderr)
		if !handled || err != nil || stderr.Len() != 0 {
			t.Fatalf("handled %v, error %v, stderr %q", handled, err, stderr.String())
		}
		for _, want := range []string{"Usage: badger map <path>", "badger-codemap-<repo_name>.html", "current directory", "50 MiB", "20 MiB"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("help missing %q", want)
			}
		}
	}
}

func TestMapCommandGeneratorResult(t *testing.T) {
	for _, failure := range []error{nil, errors.New("publish failed")} {
		var out bytes.Buffer
		err := runMapCommandWithGenerator([]string{"./source"}, &out, func(path string) (string, error) {
			if path != "./source" {
				t.Fatalf("source = %q", path)
			}
			return "/caller/badger-codemap-source.html", failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want %v", err, failure)
		}
		want := "/caller/badger-codemap-source.html\n"
		if failure != nil {
			want = ""
		}
		if out.String() != want {
			t.Fatalf("output %q, want %q", out.String(), want)
		}
	}
}

func TestDispatchEarlyMapInvalidInputs(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "source.go")
	if err := os.WriteFile(file, []byte("package source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join(root, "missing"), file, t.TempDir()} {
		var out, stderr bytes.Buffer
		handled, err := dispatchEarlyCommand([]string{"map", path}, &out, &stderr)
		if !handled || err == nil || out.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("path %q: handled %v, err %v, stdout %q, stderr %q", path, handled, err, out.String(), stderr.String())
		}
	}
}

func TestMapPreservesInteractiveDispatch(t *testing.T) {
	for _, args := range [][]string{nil, {"code"}, {"design"}, {"review"}, {"continue"}} {
		handled, err := dispatchEarlyCommand(args, &bytes.Buffer{}, &bytes.Buffer{})
		if handled || err != nil {
			t.Fatalf("args %q: handled %v, error %v", args, handled, err)
		}
	}
}

func TestDispatchEarlyMapSuccess(t *testing.T) {
	caller := t.TempDir()
	source := filepath.Join(t.TempDir(), "project")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "main.go"), []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(caller)
	var out, stderr bytes.Buffer
	handled, err := dispatchEarlyCommand([]string{"map", source}, &out, &stderr)
	if !handled || err != nil || stderr.Len() != 0 {
		t.Fatalf("handled %v, error %v, stderr %q", handled, err, stderr.String())
	}
	// Resolve the caller as the generator does (temporary paths may contain symlinks).
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(cwd, "badger-codemap-project.html")
	if out.String() != destination+"\n" {
		t.Fatalf("output = %q", out.String())
	}
	page, err := os.ReadFile(destination)
	if err != nil || !bytes.Contains(page, []byte("Codebase Map")) {
		t.Fatalf("report missing: %v", err)
	}
}
