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

	_, err := New(root, 0)
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

	eng, err := New(root, 0)
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

	eng, err := New(root, 0)
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

func TestGenerateMapDetailedIncludesTaggedFilesAndWarnings(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"docs/usage.md",
		"docs/user-guide.md",
	} {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	eng := FromTopology(root, &model.ProjectTopology{
		Languages: []string{"Go"},
		Modules:   []model.Module{{Name: "docs", FileCount: 2}},
	})
	goal := "review @docs/usage.md and @docs/missing.md and @docs/usage.md"

	schema, warnings := eng.GenerateMapDetailed(goal)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 unresolved tagged-file warning", warnings)
	}
	if !strings.Contains(warnings[0], "docs/missing.md") {
		t.Fatalf("warnings = %v, want missing path warning", warnings)
	}
	if !strings.Contains(schema, "[USER TAGGED FILES]") {
		t.Fatalf("Prompt 1 missing tagged-files section:\n%s", schema)
	}
	if strings.Count(schema, "FILE:docs/usage.md") != 1 {
		t.Fatalf("Prompt 1 should dedupe repeated tagged files:\n%s", schema)
	}
	if !strings.Contains(schema, "[TASK]\n"+goal+"\n\n[CONSTRAINT]") {
		t.Fatalf("Prompt 1 did not preserve goal text:\n%s", schema)
	}
}

func TestGenerateMapDetailedIgnoresTaggedSyntaxInFencedDiff(t *testing.T) {
	root := t.TempDir()
	eng := FromTopology(root, &model.ProjectTopology{
		Languages: []string{"CSS"},
	})
	goal := "Review this change.\n\nAttached git diff:\n```diff\n" +
		"+@import \"./theme.css\";\n+@keyframes pulse {}\n+@media (width > 1px) {}\n" +
		"```"

	schema, warnings := eng.GenerateMapDetailed(goal)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %v, want none for fenced diff content", warnings)
	}
	if strings.Contains(schema, "[USER TAGGED FILES]") {
		t.Fatalf("Prompt 1 unexpectedly included tagged files:\n%s", schema)
	}
	if !strings.Contains(schema, "[TASK]\n"+goal+"\n\n[CONSTRAINT]") {
		t.Fatalf("Prompt 1 did not preserve fenced diff content:\n%s", schema)
	}
}

func TestNewRejectsInvalidExternalContext(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte("../missing/docs\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := New(root, 0)
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

func TestGenerateMapDetailedExternalFallback(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "aibadger")
	ext1 := filepath.Join(parent, "ext1")
	ext2 := filepath.Join(parent, "ext2")

	for _, dir := range []string{root, ext1, ext2} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite := func(dir, path string) {
		t.Helper()
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite(root, "local.txt")
	mustWrite(ext1, "external.txt")
	mustWrite(ext1, "shared.txt")
	mustWrite(ext2, "shared.txt")
	mustWrite(root, "priority.txt")
	mustWrite(ext1, "priority.txt")

	eng := FromTopology(root, &model.ProjectTopology{
		ExternalContext: []model.ExternalContext{
			{Path: "ext1", AbsPath: ext1},
			{Path: "ext2", AbsPath: ext2},
		},
	})

	// 1. Local match wins
	goal1 := "use @priority.txt"
	_, warnings1 := eng.GenerateMapDetailed(goal1)
	if len(warnings1) != 0 {
		t.Fatalf("unexpected warnings for local priority: %v", warnings1)
	}

	// 2. External fallback
	goal2 := "use @external.txt"
	schema2, warnings2 := eng.GenerateMapDetailed(goal2)
	if len(warnings2) != 0 {
		t.Fatalf("unexpected warnings for external fallback: %v", warnings2)
	}
	if !strings.Contains(schema2, "FILE:external.txt") {
		t.Fatalf("schema missing external fallback file:\n%s", schema2)
	}

	// 3. Ambiguity
	goal3 := "use @shared.txt"
	_, warnings3 := eng.GenerateMapDetailed(goal3)
	if len(warnings3) != 1 || !strings.Contains(warnings3[0], "ambiguous") {
		t.Fatalf("expected ambiguity warning, got %v", warnings3)
	}

	// 4. Missing
	goal4 := "use @missing.txt"
	_, warnings4 := eng.GenerateMapDetailed(goal4)
	if len(warnings4) != 1 || !strings.Contains(warnings4[0], "does not exist") {
		t.Fatalf("expected missing warning, got %v", warnings4)
	}
}
