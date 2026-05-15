package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/writer"
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

func TestGenerateContextDetailedAllowsMissingFilesWhenSomeContentIsUsable(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "present.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	eng := FromTopology(root, &model.ProjectTopology{
		Languages: []string{"Go"},
	})
	schema, metadata, count, failedCommands, safetyExclusions, err := eng.GenerateContextDetailed("keep going", []extractor.Command{
		{Type: "FILE", Path: "present.go"},
		{Type: "FILE", Path: "missing.go"},
	})
	if err != nil {
		t.Fatalf("GenerateContextDetailed() error = %v, want nil for partial warning", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
	if len(failedCommands) != 1 || !strings.Contains(failedCommands[0], "missing.go") {
		t.Fatalf("failedCommands = %v, want missing.go warning", failedCommands)
	}
	if len(safetyExclusions) != 0 {
		t.Fatalf("safetyExclusions = %v, want none", safetyExclusions)
	}
	if len(metadata) != 1 {
		t.Fatalf("metadata = %d entries, want 1", len(metadata))
	}
	if !strings.Contains(schema, "present.go") {
		t.Fatalf("schema missing usable extraction:\n%s", schema)
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

func TestNewLoadsExternalContext(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "spec.md"), []byte("# Spec\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte("../badger-sidecar/docs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	eng, err := New(root)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(eng.Topology.ExternalContext) != 1 {
		t.Fatalf("external contexts = %d, want 1", len(eng.Topology.ExternalContext))
	}
	mapPrompt := eng.GenerateMap("use specs")
	if !strings.Contains(mapPrompt, "../badger-sidecar/docs [read-only]") {
		t.Fatalf("Prompt 1 missing external context:\n%s", mapPrompt)
	}
}

func TestNewRejectsInvalidExternalContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte("../missing/docs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := New(root)
	if err == nil {
		t.Fatal("New() error = nil, want invalid .badger-context error")
	}
	if !strings.Contains(err.Error(), "Invalid .badger-context: path does not exist: ../missing/docs") {
		t.Fatalf("New() error = %q, want invalid external context message", err.Error())
	}
}

func TestParseAndApplyRejectExternalContextUpdates(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	external := filepath.Join(parent, "badger-sidecar", "docs")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(external, 0755); err != nil {
		t.Fatal(err)
	}

	eng := FromTopology(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{Path: "../badger-sidecar/docs", AbsPath: external},
		},
	})
	input := "--- File: ../badger-sidecar/docs/spec.md ---\n# Changed\n--- End File ---"

	result := eng.ParseWritePlanDetailed(input)
	if len(result.Updates) != 0 {
		t.Fatalf("ParseWritePlanDetailed() updates = %d, want 0", len(result.Updates))
	}
	if len(result.Errors) == 0 {
		t.Fatal("ParseWritePlanDetailed() errors = 0, want external context error")
	}
	if !hasErrorContaining(result.Errors, "External context paths are read-only.") {
		t.Fatalf("errors = %v, want read-only external context error", result.Errors)
	}

	err := eng.ApplyUpdate(modelUpdate("../badger-sidecar/docs/spec.md"), writer.WhitespaceModeExact)
	if err == nil {
		t.Fatal("ApplyUpdate() error = nil, want external context guard")
	}
	if !strings.Contains(err.Error(), "Cannot apply patch outside project root: ../badger-sidecar/docs/spec.md") {
		t.Fatalf("ApplyUpdate() error = %q, want outside-root rejection", err.Error())
	}
}

func modelUpdate(path string) writer.FileUpdate {
	return writer.FileUpdate{Path: path, Content: "changed\n"}
}

func hasErrorContaining(errs []error, text string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), text) {
			return true
		}
	}
	return false
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
