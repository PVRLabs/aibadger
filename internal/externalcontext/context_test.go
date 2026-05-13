package externalcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestLoadMissingAndEmptyContextPreserveBehavior(t *testing.T) {
	root := t.TempDir()

	contexts, err := Load(root)
	if err != nil {
		t.Fatalf("Load(missing) error = %v", err)
	}
	if len(contexts) != 0 {
		t.Fatalf("Load(missing) = %d contexts, want 0", len(contexts))
	}

	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte("\n# comment\n\n"), 0644); err != nil {
		t.Fatal(err)
	}
	contexts, err = Load(root)
	if err != nil {
		t.Fatalf("Load(empty) error = %v", err)
	}
	if len(contexts) != 0 {
		t.Fatalf("Load(empty) = %d contexts, want 0", len(contexts))
	}
}

func TestLoadContextNormalizesDeduplicatesAndSummarizes(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "project")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(external, "plans"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".git", "node_modules", "target", "build"} {
		if err := os.MkdirAll(filepath.Join(external, dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"spec.md", "ui-spec.md"} {
		if err := os.WriteFile(filepath.Join(external, name), []byte("# "+name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{".DS_Store", "Thumbs.db", "go.sum", "package-lock.json", "app.jar"} {
		if err := os.WriteFile(filepath.Join(external, name), []byte("noise\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	config := strings.Join([]string{
		"# Extra read-only context paths",
		"../badger-sidecar/docs/.",
		"../badger-sidecar/docs",
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}

	contexts, err := Load(root)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("Load() = %d contexts, want 1", len(contexts))
	}
	if contexts[0].Path != "../badger-sidecar/docs" {
		t.Fatalf("context path = %q, want normalized relative path", contexts[0].Path)
	}
	got := topNames(contexts[0].Top)
	want := []string{"spec.md", "ui-spec.md", "plans/"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("top names = %v, want prefix %v", got, want)
		}
	}
}

func TestLoadContextRejectsInvalidEntries(t *testing.T) {
	for _, tt := range []struct {
		name    string
		setup   func(root string) string
		wantErr string
	}{
		{
			name: "missing path",
			setup: func(root string) string {
				return "../missing/docs"
			},
			wantErr: "path does not exist",
		},
		{
			name: "file path",
			setup: func(root string) string {
				if err := os.WriteFile(filepath.Join(root, "spec.md"), []byte("# spec\n"), 0644); err != nil {
					t.Fatal(err)
				}
				return "spec.md"
			},
			wantErr: "path is not a directory",
		},
		{
			name: "glob",
			setup: func(root string) string {
				return "docs/*"
			},
			wantErr: "invalid path format",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			entry := tt.setup(root)
			if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte(entry+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			_, err := Load(root)
			if err == nil {
				t.Fatal("Load() error = nil, want validation error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestContainsFileRejectsCommonExcludedExternalPaths(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "docs")
	if err := os.MkdirAll(filepath.Join(external, ".git"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, ".DS_Store"), []byte("noise\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, ".git", "config"), []byte("config\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "spec.md"), []byte("# Spec\n"), 0644); err != nil {
		t.Fatal(err)
	}

	contexts := []model.ExternalContext{{Path: "../docs", AbsPath: external}}
	for _, path := range []string{
		filepath.Join(external, ".DS_Store"),
		filepath.Join(external, ".git", "config"),
	} {
		if _, _, ok := ContainsFile(root, contexts, path); ok {
			t.Fatalf("ContainsFile(%q) = true, want false for excluded external context path", path)
		}
	}
	if _, _, ok := ContainsFile(root, contexts, filepath.Join(external, "spec.md")); !ok {
		t.Fatal("ContainsFile(spec.md) = false, want true for allowed external context file")
	}
}

func topNames(items []model.ExternalContextItem) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		name := item.Name
		if item.IsDir {
			name += "/"
		}
		names = append(names, name)
	}
	return names
}
