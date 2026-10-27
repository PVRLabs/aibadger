package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/codemap"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

func TestMapCommandValidationAndHelp(t *testing.T) {
	for _, args := range [][]string{{""}, {"a", "b"}, {"--output"}, {"--help", "extra"}} {
		var out bytes.Buffer
		err := runMapCommandWithGenerator(args, &out, func(string, codemap.Options) (string, error) {
			t.Fatal("invalid arguments invoked generator")
			return "", nil
		})
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
		for _, want := range []string{"Usage: badger map [path]", "badger-codemap-<repo_name>.html", "current directory", "50 MiB", "20 MiB"} {
			if !strings.Contains(out.String(), want) {
				t.Fatalf("help missing %q", want)
			}
		}
	}
}

func TestMapCommandGeneratorResult(t *testing.T) {
	for _, failure := range []error{nil, errors.New("publish failed")} {
		var out bytes.Buffer
		err := runMapCommandWithGenerator([]string{"./source"}, &out, func(path string, _ codemap.Options) (string, error) {
			if path != "./source" {
				t.Fatalf("source = %q", path)
			}
			return "/caller/badger-codemap-source.html", failure
		})
		if !errors.Is(err, failure) {
			t.Fatalf("error = %v, want %v", err, failure)
		}
		want := "Generated codebase map HTML report.\nUse the following command to open:\n" + mapOpenCommand("/caller/badger-codemap-source.html", runtime.GOOS) + "\n"
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
	if out.String() != "Generated codebase map HTML report.\nUse the following command to open:\n"+mapOpenCommand(destination, runtime.GOOS)+"\n" {
		t.Fatalf("output = %q", out.String())
	}
	page, err := os.ReadFile(destination)
	if err != nil || !bytes.Contains(page, []byte("Codebase Map")) {
		t.Fatalf("report missing: %v", err)
	}
}

func TestMapCommandDefaultsToCurrentDirectory(t *testing.T) {
	caller := t.TempDir()
	t.Chdir(caller)
	if err := os.WriteFile("main.go", []byte("package main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	handled, err := dispatchEarlyCommand([]string{"map"}, &out, &stderr)
	if !handled || err != nil || stderr.Len() != 0 {
		t.Fatalf("handled %v, error %v, stderr %q", handled, err, stderr.String())
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(cwd, "badger-codemap-"+filepath.Base(cwd)+".html")
	if out.String() != "Generated codebase map HTML report.\nUse the following command to open:\n"+mapOpenCommand(destination, runtime.GOOS)+"\n" {
		t.Fatalf("output = %q", out.String())
	}
	page, err := os.ReadFile(destination)
	if err != nil || !bytes.Contains(page, []byte("Codebase Map")) {
		t.Fatalf("report missing: %v", err)
	}
}

func TestMapOpenCommand(t *testing.T) {
	for _, tc := range []struct{ goos, path, want string }{
		{"darwin", "/repo/a b.html", "open -- '/repo/a b.html'"},
		{"linux", "/repo/a b.html", "xdg-open '/repo/a b.html'"},
		{"darwin", "/repo/a'$(touch nope).html", "open -- '/repo/a'\"'\"'$(touch nope).html'"},
		{"windows", "C:/repo/a'b & $x.html", "PowerShell:\nInvoke-Item -LiteralPath 'C:/repo/a''b & $x.html'"},
	} {
		if got := mapOpenCommand(tc.path, tc.goos); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.goos, got, tc.want)
		}
	}
}

func TestMapOptions(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		target   string
		patterns []string
		hidden   bool
	}{
		{nil, ".", nil, false},
		{[]string{"--exclude", "vendor", "repo", "--exclude=**/*.min.js", "--exclude", "vendor"}, "repo", []string{"vendor", "**/*.min.js", "vendor"}, false},
		{[]string{"--no-repro-command", "--exclude=-secret", "--", "-repo"}, "-repo", []string{"-secret"}, true},
		{[]string{"repo", "--no-repro-command"}, "repo", nil, true},
		{[]string{"--exclude= a'b $();{} ", "repo"}, "repo", []string{" a'b $();{} "}, false},
	} {
		target, opts, err := parseMapOptions(tc.args)
		if err != nil || target != tc.target || opts.NoReproCommand != tc.hidden || !reflect.DeepEqual(opts.ExcludePatterns, tc.patterns) {
			t.Fatalf("%q: %q %+v %v", tc.args, target, opts, err)
		}
	}
	for _, args := range [][]string{
		{"--exclude"}, {"--exclude", "--no-repro-command"}, {"--exclude="}, {"--exclude", ""}, {"--exclude=C:foo"},
		{"--exclude=../foo"}, {"--exclude=a?"}, {"--no-repro-command=true"}, {"a", "b"}, {"--", "a", "b"},
	} {
		called := false
		err := runMapCommandWithGenerator(args, &bytes.Buffer{}, func(string, codemap.Options) (string, error) { called = true; return "", nil })
		if err == nil || called {
			t.Fatalf("invalid args %q: err=%v called=%v", args, err, called)
		}
	}
	var out bytes.Buffer
	err := runMapCommandWithGenerator([]string{"--exclude=vendor", "repo"}, &out, func(target string, opts codemap.Options) (string, error) {
		if target != "repo" || opts.Version != badger.Version || !reflect.DeepEqual(opts.ExcludePatterns, []string{"vendor"}) {
			t.Fatalf("settings: %s %+v", target, opts)
		}
		return "/report.html", nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
