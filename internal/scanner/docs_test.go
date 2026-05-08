package scanner

import (
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestScanIncludesRootContextDocs(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/docs\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "app", "main.go"), "package app\n\nfunc Main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "README.md"), "# Project\n")
	writeTestFile(t, filepath.Join(tmpDir, "AGENTS.md"), "# Agent guidance\n")
	writeTestFile(t, filepath.Join(tmpDir, "SECURITY.md"), "# Security\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}

	rootPkg := findPackage(topology.Modules[0], "")
	if rootPkg == nil {
		t.Fatalf("root documentation package missing from source roots: %+v", topology.Modules[0].SourceRoots)
	}
	for _, path := range []string{"AGENTS.md", "README.md", "SECURITY.md"} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
	for _, file := range rootPkg.TopFiles {
		if file.Kind != model.FileKindSource {
			t.Fatalf("doc file kind = %q, want existing source kind for %+v", file.Kind, file)
		}
	}
}

func TestScanIncludesHighSignalDocsFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/docs\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "app", "main.go"), "package app\n\nfunc Main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "architecture-overview.md"), "# Architecture\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "design-notes.md"), "# Design\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "spec.md"), "# Spec\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "notes.md"), "# Notes\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "spec.txt"), "Spec text\n")
	writeTestFile(t, filepath.Join(tmpDir, "doc", "api.md"), "# API\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}

	docsPkg := findPackage(topology.Modules[0], "docs")
	if docsPkg == nil {
		t.Fatalf("docs package missing from source roots: %+v", topology.Modules[0].SourceRoots)
	}
	for _, path := range []string{
		filepath.Join("docs", "architecture-overview.md"),
		filepath.Join("docs", "design-notes.md"),
		filepath.Join("docs", "spec.md"),
		filepath.Join("docs", "notes.md"),
	} {
		if !hasTopFile(docsPkg.TopFiles, path) {
			t.Fatalf("docsPkg.TopFiles = %+v, missing %s", docsPkg.TopFiles, path)
		}
	}
	if hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "spec.txt")) {
		t.Fatalf("docsPkg.TopFiles = %+v, should not include docs/spec.txt", docsPkg.TopFiles)
	}

	docPkg := findPackage(topology.Modules[0], "doc")
	if docPkg == nil {
		t.Fatalf("doc package missing from source roots: %+v", topology.Modules[0].SourceRoots)
	}
	if !hasTopFile(docPkg.TopFiles, filepath.Join("doc", "api.md")) {
		t.Fatalf("docPkg.TopFiles = %+v, missing doc/api.md", docPkg.TopFiles)
	}
}

func TestScanExcludesDeepDocs(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/docs\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "shallow.md"), "# Shallow\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "deep", "nested.md"), "# Deep\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "archive", "old.md"), "# Old\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	docsPkg := findPackage(topology.Modules[0], "docs")
	if docsPkg == nil {
		t.Fatalf("docs package missing")
	}

	if !hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "shallow.md")) {
		t.Errorf("missing docs/shallow.md")
	}
	if hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "deep", "nested.md")) {
		t.Errorf("should not include deep/nested.md")
	}
	if hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "archive", "old.md")) {
		t.Errorf("should not include archive/old.md")
	}
}
