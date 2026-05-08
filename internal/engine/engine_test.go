package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestGenerateContextIncludesClaudeStyleFileSelections(t *testing.T) {
	root := t.TempDir()
	files := []string{
		"cmd/badger/main.go",
		"pkg/badger/config.go",
		"pkg/badger/headless.go",
		"internal/engine/engine.go",
		"internal/model/topology.go",
		"internal/brand/brand.go",
	}
	for _, file := range files {
		path := filepath.Join(root, file)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("package main\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	eng := FromTopology(root, &model.ProjectTopology{
		Languages: []string{"Go"},
		Stack:     []string{"Go Modules"},
		Structure: "Single Module",
	})
	commands := eng.ParseCommands(strings.Join([]string{
		"FILE:cmd/badger/main.go",
		"FILE:pkg/badger/config.go",
		"FILE:pkg/badger/headless.go",
		"FILE:internal/engine/engine.go",
		"FILE:internal/model/topology.go",
		"FILE:internal/brand/brand.go",
	}, "\n"))

	schema, metadata, err := eng.GenerateContext("what is this project about?", commands)
	if err != nil {
		t.Fatalf("GenerateContext() error = %v", err)
	}
	if !strings.Contains(schema, "Active Extractions: 6 files") {
		t.Fatalf("schema missing active extraction count:\n%s", schema)
	}
	if len(metadata) != 6 {
		t.Fatalf("metadata = %d entries, want 6", len(metadata))
	}
	for _, file := range files {
		if !strings.Contains(schema, "--- File: "+file+" (Full File) ---") {
			t.Fatalf("schema missing file %s:\n%s", file, schema)
		}
	}
}

func TestGenerateContextFallsBackForMissingSelectorAnchors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pkg/badger/headless.go")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package badger\n\nfunc RunHeadless() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	eng := FromTopology(root, &model.ProjectTopology{
		Languages: []string{"Go"},
	})
	commands := eng.ParseCommands("PREFIX:pkg/badger/headless.go#func Start")

	schema, metadata, err := eng.GenerateContext("find crash paths", commands)
	if err != nil {
		t.Fatalf("GenerateContext() error = %v, want nil for fallback", err)
	}
	if len(metadata) != 1 {
		t.Fatalf("metadata = %d entries, want 1", len(metadata))
	}
	if !strings.Contains(schema, "--- File: pkg/badger/headless.go (Full File) ---") {
		t.Fatalf("schema missing extracted file blocks:\n%s", schema)
	}
	if !strings.Contains(schema, "package badger") {
		t.Fatalf("schema missing fallback file content:\n%s", schema)
	}
}

func TestNewReturnsErrorWhenDisabledFileExists(t *testing.T) {
	root := t.TempDir()
	disablePath := filepath.Join(root, DisableFileName)
	if err := os.WriteFile(disablePath, []byte("reason\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := New(root)
	if err != ErrProjectDisabled {
		t.Fatalf("New() error = %v, want ErrProjectDisabled", err)
	}
}

func TestNewProceedsNormallyWhenNoDisabledFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	eng, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if eng == nil {
		t.Fatal("eng = nil, want non-nil")
	}
}

func TestCheckDisabledReturnsNilWhenNoDisabledFile(t *testing.T) {
	root := t.TempDir()
	if err := CheckDisabled(root); err != nil {
		t.Fatalf("CheckDisabled() error = %v, want nil", err)
	}
}

func TestCheckDisabledReturnsNilForDirectoryNamedBadgerDisable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, DisableFileName), 0755); err != nil {
		t.Fatal(err)
	}

	if err := CheckDisabled(root); err != nil {
		t.Fatalf("CheckDisabled() error = %v, want nil for directory", err)
	}
}

func TestCheckDisabledReturnsErrorForExistentFile(t *testing.T) {
	root := t.TempDir()
	disablePath := filepath.Join(root, DisableFileName)
	if err := os.WriteFile(disablePath, []byte("opt-out\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CheckDisabled(root); err != ErrProjectDisabled {
		t.Fatalf("CheckDisabled() error = %v, want ErrProjectDisabled", err)
	}
}
