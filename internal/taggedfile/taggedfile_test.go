package taggedfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseTaggedReferences(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr string
	}{
		{
			name:  "multiple refs",
			input: "review @cmd/badger/main.go and @\"docs/plan notes.md\" please",
			want:  []string{"cmd/badger/main.go", "docs/plan notes.md"},
		},
		{
			name:  "repeated refs",
			input: "keep @src/main.go and again @src/main.go",
			want:  []string{"src/main.go", "src/main.go"},
		},
		{
			name:  "email like text",
			input: "contact name@example.com for updates",
			want:  []string{},
		},
		{
			name:    "malformed quoted ref",
			input:   "fix @\"docs/spec.md",
			want:    []string{},
			wantErr: "missing closing quote",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refs, errs := Parse(tt.input)
			got := make([]string, 0, len(refs))
			for _, ref := range refs {
				got = append(got, ref.Path)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("Parse() refs = %v, want %v", got, tt.want)
			}
			if tt.wantErr == "" {
				if len(errs) != 0 {
					t.Fatalf("Parse() errors = %v, want none", errs)
				}
				return
			}
			if len(errs) == 0 {
				t.Fatal("Parse() errors = nil, want validation error")
			}
			if !strings.Contains(errs[0].Error(), tt.wantErr) {
				t.Fatalf("Parse() error = %q, want %q", errs[0].Error(), tt.wantErr)
			}
		})
	}
}

func TestActiveTokenAt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		cursor int
		want   string
		ok     bool
	}{
		{
			name:   "start of input",
			input:  "@src/main.go",
			cursor: len("@src/main.go"),
			want:   "src/main.go",
			ok:     true,
		},
		{
			name:   "after punctuation boundary",
			input:  "fix, @src/main.go",
			cursor: len("fix, @src/main.go"),
			want:   "src/main.go",
			ok:     true,
		},
		{
			name:   "after completed token",
			input:  "@src/main.go ",
			cursor: len("@src/main.go "),
			ok:     false,
		},
		{
			name:   "email like text",
			input:  "name@example.com",
			cursor: len("name@example.com"),
			ok:     false,
		},
		{
			name:   "quoted token with spaces",
			input:  "keep @\"docs/plan notes.md\" handy",
			cursor: len("keep @\"docs/plan notes.md\""),
			want:   "docs/plan notes.md",
			ok:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := ActiveTokenAt(tt.input, tt.cursor)
			if ok != tt.ok {
				t.Fatalf("ActiveTokenAt() ok = %v, want %v", ok, tt.ok)
			}
			if !ok {
				return
			}
			if ref.Path != tt.want {
				t.Fatalf("ActiveTokenAt() path = %q, want %q", ref.Path, tt.want)
			}
		})
	}
}

func TestResolveTaggedReferenceRejectsTraversalAndSymlinkEscape(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	resolved, err := Resolve(root, "main.go")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Path != "main.go" {
		t.Fatalf("Resolve() path = %q, want main.go", resolved.Path)
	}

	for _, path := range []string{"../escape.go", "src/../../escape.go"} {
		if _, err := Resolve(root, path); err == nil {
			t.Fatalf("Resolve(%q) error = nil, want traversal rejection", path)
		}
	}

	parent := filepath.Dir(root)
	outside := filepath.Join(parent, "outside.txt")
	if err := os.WriteFile(outside, []byte("outside\n"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unsupported on this platform: %v", err)
	}
	if _, err := Resolve(root, "escape.txt"); err == nil {
		t.Fatal("Resolve(escape.txt) error = nil, want symlink escape rejection")
	}
}

func TestCompleteTaggedReferencesIsBoundedAndTraversesDirectories(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite := func(path string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("docs/alpha.md")
	mustWrite("docs/beta.md")
	mustWrite("docs/nested/child.go")
	mustWrite("docs/secret.key")
	mustWrite("src/main.go")

	skip := func(relPath string, isDir bool) bool {
		return relPath == "docs/secret.key"
	}

	completions, err := Complete(root, "docs/", 2, skip)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(completions) != 2 {
		t.Fatalf("len(Complete()) = %d, want 2", len(completions))
	}
	if completions[0].Path != "docs/alpha.md" || completions[1].Path != "docs/beta.md" {
		t.Fatalf("Complete() paths = %v, want docs/alpha.md and docs/beta.md", []string{completions[0].Path, completions[1].Path})
	}

	nested, err := Complete(root, "docs/n", 8, skip)
	if err != nil {
		t.Fatalf("Complete(nested) error = %v", err)
	}
	if len(nested) != 1 || nested[0].Path != "docs/nested/" || !nested[0].IsDir {
		t.Fatalf("Complete(nested) = %+v, want docs/nested/ directory suggestion", nested)
	}

	if _, err := Complete(root, "../", 8, skip); err == nil {
		t.Fatal("Complete(traversal) error = nil, want rejection")
	}
}

func TestCompleteTaggedReferencesRanksBasenameMatches(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite := func(path string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("usage.md")
	mustWrite("docs/usage.md")
	mustWrite("docs/user-guide.md")
	mustWrite("examples/usage.md")
	mustWrite("docs/nested/child.go")

	completions, err := Complete(root, "us", 8, nil)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(completions) < 4 {
		t.Fatalf("len(Complete()) = %d, want at least 4", len(completions))
	}
	if completions[0].Path != "usage.md" {
		t.Fatalf("first completion = %q, want root basename match usage.md", completions[0].Path)
	}
	for _, want := range []string{"docs/usage.md", "docs/user-guide.md", "examples/usage.md"} {
		if !containsSuggestion(completions, want) {
			t.Fatalf("Complete() missing %q in ranked results: %v", want, suggestionsToPaths(completions))
		}
	}
	for _, suggestion := range completions {
		if suggestion.Path == "docs/nested/child.go" {
			t.Fatalf("Complete() included deeper file %q: %v", suggestion.Path, suggestionsToPaths(completions))
		}
	}
}

func TestCompleteTaggedReferencesMatchesRepoRelativePathPrefix(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite := func(path string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("docs/usage.md")
	mustWrite("docs/user-guide.md")
	mustWrite("docs/nested/child.go")

	completions, err := Complete(root, "docs/us", 8, nil)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(completions) == 0 {
		t.Fatal("Complete() returned no matches for repo-relative path prefix")
	}
	if completions[0].Path != "docs/usage.md" {
		t.Fatalf("first completion = %q, want docs/usage.md", completions[0].Path)
	}
	for _, suggestion := range completions {
		if suggestion.Path == "docs/nested/child.go" {
			t.Fatalf("Complete() included deeper file %q: %v", suggestion.Path, suggestionsToPaths(completions))
		}
	}
}

func TestCompleteTaggedReferencesRespectsBoundedLimitAndSkip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite := func(path string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("usage.md")
	mustWrite("docs/usage.md")
	mustWrite("docs/user-guide.md")
	mustWrite("examples/usage.md")

	skip := func(relPath string, isDir bool) bool {
		return relPath == "docs/user-guide.md"
	}

	completions, err := Complete(root, "us", 2, skip)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if len(completions) != 2 {
		t.Fatalf("len(Complete()) = %d, want 2", len(completions))
	}
	if completions[0].Path != "usage.md" {
		t.Fatalf("first completion = %q, want usage.md", completions[0].Path)
	}
	if completions[1].Path != "docs/usage.md" {
		t.Fatalf("second completion = %q, want docs/usage.md", completions[1].Path)
	}
}

func containsSuggestion(suggestions []Suggestion, want string) bool {
	for _, suggestion := range suggestions {
		if suggestion.Path == want {
			return true
		}
	}
	return false
}

func suggestionsToPaths(suggestions []Suggestion) []string {
	paths := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		paths = append(paths, suggestion.Path)
	}
	return paths
}
