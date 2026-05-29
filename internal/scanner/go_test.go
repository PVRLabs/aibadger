package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestGoDetectorDetectsModuleSourceRoots(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/orders\n\ngo 1.23\n")
	writeTestFile(t, filepath.Join(tmpDir, "cmd", "server", "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "handler", "order.go"), "package handler\n\nfunc HandleOrder() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "model", "order.go"), "package model\n\ntype Order struct{}\n")
	writeTestFile(t, filepath.Join(tmpDir, "pkg", "common", "util.go"), "package common\n\nfunc Helper() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "handler", "order_test.go"), "package handler\n")
	writeTestFile(t, filepath.Join(tmpDir, "README.md"), "# intentionally larger than source files\n\nThis must not become heaviest.\n")

	detector := NewGoDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.Name != "example.com/orders" {
		t.Fatalf("module.Name = %q, want module path from go.mod", module.Name)
	}
	if module.Path != "" {
		t.Fatalf("module.Path = %q, want root-relative empty path for project root", module.Path)
	}
	if module.Language != "Go" {
		t.Fatalf("module.Language = %q, want Go", module.Language)
	}
	if module.FileCount != 5 {
		t.Fatalf("module.FileCount = %d, want 5 Go files including tests", module.FileCount)
	}
	if module.Heaviest.Path == "README.md" || filepath.Ext(module.Heaviest.Path) != ".go" {
		t.Fatalf("module.Heaviest.Path = %q, want a .go source file", module.Heaviest.Path)
	}

	if !hasSourceRoot(module, "cmd") {
		t.Fatalf("module.SourceRoots = %v, missing cmd", module.SourceRoots)
	}
	if !hasSourceRoot(module, "internal") {
		t.Fatalf("module.SourceRoots = %v, missing internal", module.SourceRoots)
	}
	if !hasSourceRoot(module, "pkg") {
		t.Fatalf("module.SourceRoots = %v, missing pkg", module.SourceRoots)
	}
	if !hasPackage(module, filepath.Join("internal", "handler")) {
		t.Fatalf("module packages missing internal/handler")
	}
	pkg := findPackage(module, filepath.Join("internal", "handler"))
	if pkg == nil {
		t.Fatalf("missing internal/handler package")
	}
	if pkg.FileCount != 2 {
		t.Fatalf("internal/handler FileCount = %d, want production and test files", pkg.FileCount)
	}
	if !hasTopFile(pkg.TopFiles, filepath.Join("internal", "handler", "order_test.go")) {
		t.Fatalf("internal/handler TopFiles = %+v, missing order_test.go", pkg.TopFiles)
	}
}

func TestGoDetectorIncludesTestOnlyPackages(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/testonly\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "handler", "order_test.go"), "package handler\n\nfunc TestOrder(t *testing.T) {}\n")

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	pkg := findPackage(modules[0], filepath.Join("internal", "handler"))
	if pkg == nil {
		t.Fatalf("missing internal/handler test-only package")
	}
	if pkg.FileCount != 1 {
		t.Fatalf("pkg.FileCount = %d, want test file count", pkg.FileCount)
	}
	if !hasTopFile(pkg.TopFiles, filepath.Join("internal", "handler", "order_test.go")) {
		t.Fatalf("pkg.TopFiles = %+v, missing order_test.go", pkg.TopFiles)
	}
}

func TestGoDetectorDetectsRootPackage(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/cli\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "helper.go"), "package main\n\nfunc helper() {}\n")

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.FileCount != 2 {
		t.Fatalf("module.FileCount = %d, want root main package files", module.FileCount)
	}
	if !hasSourceRoot(module, "") {
		t.Fatalf("module.SourceRoots = %v, missing root source root", module.SourceRoots)
	}
	if !hasPackage(module, "") {
		t.Fatalf("module packages missing root package")
	}
}

func TestGoDetectorDetectsRootLibraryPackage(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/math\n")
	writeTestFile(t, filepath.Join(tmpDir, "math.go"), "package math\n\nfunc Add(a, b int) int { return a + b }\n")

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want root library package file", module.FileCount)
	}
	if !hasSourceRoot(module, "") {
		t.Fatalf("module.SourceRoots = %v, missing root source root", module.SourceRoots)
	}
	if !hasPackage(module, "") {
		t.Fatalf("module packages missing root package")
	}
	if !hasTopFile(module.TopFiles, "math.go") {
		t.Fatalf("module.TopFiles = %+v, missing math.go", module.TopFiles)
	}
}

func TestScannerDetectsGoWorkspaceModules(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.work"), "go 1.21\n\nuse (\n\t./module1\n\t./module2\n)\n")
	writeTestFile(t, filepath.Join(tmpDir, "module1", "go.mod"), "module example.com/module1\n\ngo 1.21\n")
	writeTestFile(t, filepath.Join(tmpDir, "module1", "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "module2", "go.mod"), "module example.com/module2\n\ngo 1.21\n")
	writeTestFile(t, filepath.Join(tmpDir, "module2", "math.go"), "package module2\n\nfunc Add(a, b int) int { return a + b }\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if topology.Structure != "Multi-Module" {
		t.Fatalf("topology.Structure = %q, want Multi-Module", topology.Structure)
	}
	if len(topology.Modules) != 2 {
		t.Fatalf("len(topology.Modules) = %d, want 2", len(topology.Modules))
	}

	module2 := findModule(topology.Modules, "example.com/module2")
	if module2 == nil {
		t.Fatalf("topology.Modules = %+v, missing module2", topology.Modules)
	}
	if !hasPackage(*module2, "module2") {
		t.Fatalf("module2 packages missing root package")
	}
	if !hasTopFile(module2.TopFiles, filepath.Join("module2", "math.go")) {
		t.Fatalf("module2.TopFiles = %+v, missing math.go", module2.TopFiles)
	}
}

func TestGoDetectorRecordsTopFiles(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/top\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "service", "small.go"), "package service\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "service", "large.go"), "package service\n\nfunc Large() {\n\tprintln(\"large\")\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "service", "medium.go"), "package service\n\nfunc Medium() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "service", "tiny.go"), "package service")

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	pkg := findPackage(modules[0], filepath.Join("internal", "service"))
	if pkg == nil {
		t.Fatalf("missing internal/service package")
	}
	if len(pkg.TopFiles) != 3 {
		t.Fatalf("len(pkg.TopFiles) = %d, want 3", len(pkg.TopFiles))
	}
	if pkg.TopFiles[0].Size < pkg.TopFiles[1].Size || pkg.TopFiles[1].Size < pkg.TopFiles[2].Size {
		t.Fatalf("pkg.TopFiles not sorted by descending size: %+v", pkg.TopFiles)
	}
	if pkg.Heaviest.Path != pkg.TopFiles[0].Path {
		t.Fatalf("pkg.Heaviest.Path = %q, want top file %q", pkg.Heaviest.Path, pkg.TopFiles[0].Path)
	}
}

func TestGoDetectorUsesRootModuleTopFileBudget(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/root\n")
	for i := 0; i < maxRootPackageTopFiles+1; i++ {
		writeTestFile(t, filepath.Join(tmpDir, string(rune('a'+i))+".go"), "package root\n")
	}

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if len(modules[0].TopFiles) != maxRootPackageTopFiles {
		t.Fatalf("len(module.TopFiles) = %d, want %d", len(modules[0].TopFiles), maxRootPackageTopFiles)
	}
}

func TestScannerUsesGoDetectorForGoModProject(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/badger\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "scanner", "scanner.go"), "package scanner\n\nfunc Scan() {}\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if topology.PrimaryLanguage != "Go" {
		t.Fatalf("topology.PrimaryLanguage = %q, want Go", topology.PrimaryLanguage)
	}
	if len(topology.Languages) != 1 || topology.Languages[0] != "Go" {
		t.Fatalf("topology.Languages = %v, want [Go]", topology.Languages)
	}
	if topology.Structure != "Single Module" {
		t.Fatalf("topology.Structure = %q, want Single Module", topology.Structure)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	if topology.Modules[0].Language != "Go" {
		t.Fatalf("module language = %q, want Go detector output", topology.Modules[0].Language)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func hasSourceRoot(module model.Module, path string) bool {
	for _, sr := range module.SourceRoots {
		if sr.Path == path {
			return true
		}
	}
	return false
}

func hasPackage(module model.Module, path string) bool {
	return findPackage(module, path) != nil
}

func findModule(modules []model.Module, name string) *model.Module {
	for _, module := range modules {
		if module.Name == name {
			module := module
			return &module
		}
	}
	return nil
}

func findPackage(module model.Module, path string) *model.Package {
	for _, sr := range module.SourceRoots {
		for _, pkg := range sr.Packages {
			if pkg.Path == path {
				pkg := pkg
				return &pkg
			}
		}
	}
	return nil
}

func hasTopFile(files []model.FileSummary, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func moduleHasPackageTopFile(module model.Module, path string) bool {
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			if hasTopFile(pkg.TopFiles, path) {
				return true
			}
		}
	}
	return false
}
