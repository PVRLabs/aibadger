package scanner

import (
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestJavaDetectorDetectsMavenModule(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), `<project><groupId>com.example</groupId><artifactId>orders</artifactId></project>`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "com", "example", "OrderService.java"), "package com.example;\n\npublic class OrderService {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "com", "example", "model", "Order.java"), "package com.example.model;\n\npublic class Order {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "test", "java", "com", "example", "OrderServiceTest.java"), "package com.example;\n\npublic class OrderServiceTest {}\n")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.Language != "Java" {
		t.Fatalf("module.Language = %q, want Java", module.Language)
	}
	if module.FileCount < 3 {
		t.Fatalf("module.FileCount = %d, want at least 3 Java files", module.FileCount)
	}

	if !hasJavaSourceRoot(module, "src/main/java") {
		t.Fatalf("module.SourceRoots = %v, missing src/main/java", module.SourceRoots)
	}
	if !hasJavaSourceRoot(module, "src/test/java") {
		t.Fatalf("module.SourceRoots = %v, missing src/test/java", module.SourceRoots)
	}
}

func TestJavaDetectorDetectsGradleModule(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "api", "build.gradle"), "plugins { id 'java' }")
	writeTestFile(t, filepath.Join(tmpDir, "api", "src", "main", "java", "com", "api", "RestController.java"), "package com.api;\n\npublic class RestController {}\n")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.Name != "api" {
		t.Fatalf("module.Name = %q, want api", module.Name)
	}
	if !hasJavaSourceRoot(module, filepath.Join("api", "src", "main", "java")) {
		t.Fatalf("module.SourceRoots = %v, missing api source root", module.SourceRoots)
	}
	if !hasTopFile(module.TopFiles, filepath.Join("api", "src", "main", "java", "com", "api", "RestController.java")) {
		t.Fatalf("module.TopFiles = %+v, missing RestController.java", module.TopFiles)
	}
}

func TestJavaDetectorFindsSourceRoot(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Main.java"), "class Main {}")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "resources", "app.properties"), "")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	module := modules[0]
	sr := findJavaSourceRoot(module, "src/main/java")
	if sr == nil {
		t.Fatalf("missing src/main/java source root")
	}
	if sr.Role != "Main Source" {
		t.Fatalf("sr.Role = %q, want Main Source", sr.Role)
	}
}

func TestJavaDetectorIncludesCoLocatedResourceFiles(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	pkgPath := filepath.Join("src", "main", "java", "com", "example")
	writeTestFile(t, filepath.Join(tmpDir, pkgPath, "OrderMapper.java"), "package com.example;\nclass OrderMapper {}\n")
	writeTestFile(t, filepath.Join(tmpDir, pkgPath, "OrderMapper.xml"), "<mapper namespace=\"com.example.OrderMapper\" />\n")
	writeTestFile(t, filepath.Join(tmpDir, pkgPath, "application.properties"), "orders.enabled=true\n")
	writeTestFile(t, filepath.Join(tmpDir, pkgPath, "archive.zip"), "binary")

	modules, err := NewJavaDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.FileCount != 3 {
		t.Fatalf("module.FileCount = %d, want Java source, XML mapper, and properties resource", module.FileCount)
	}

	pkg := findPackage(module, pkgPath)
	if pkg == nil {
		t.Fatalf("missing %s package", pkgPath)
	}
	if pkg.FileCount != 3 {
		t.Fatalf("pkg.FileCount = %d, want Java source, XML mapper, and properties resource", pkg.FileCount)
	}
	if !hasTopFile(pkg.TopFiles, filepath.Join(pkgPath, "OrderMapper.xml")) {
		t.Fatalf("pkg.TopFiles = %+v, missing OrderMapper.xml", pkg.TopFiles)
	}
	if !hasTopFile(pkg.TopFiles, filepath.Join(pkgPath, "application.properties")) {
		t.Fatalf("pkg.TopFiles = %+v, missing application.properties", pkg.TopFiles)
	}
	if hasTopFile(pkg.TopFiles, filepath.Join(pkgPath, "archive.zip")) || hasAuxFile(pkg.AuxFiles, filepath.Join(pkgPath, "archive.zip")) {
		t.Fatalf("pkg surfaced excluded archive.zip: top=%+v aux=%+v", pkg.TopFiles, pkg.AuxFiles)
	}
}

func TestJavaDetectorDoesNotPromoteSrcMainWhenResourcesExist(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "com", "example", "OrderService.java"), "package com.example;\nclass OrderService {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "resources", "schema.sql"), "create table orders (id integer primary key);\n")

	modules, err := NewJavaDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if hasJavaSourceRoot(module, filepath.Join("src", "main")) {
		t.Fatalf("module.SourceRoots = %+v, should not include overlapping src/main root", module.SourceRoots)
	}
	if !hasJavaSourceRoot(module, filepath.Join("src", "main", "java")) {
		t.Fatalf("module.SourceRoots = %+v, missing src/main/java root", module.SourceRoots)
	}
	if module.FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want only src/main/java file counted without src/main duplication", module.FileCount)
	}
}

func TestJavaDetectorRecordsTopFiles(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Small.java"), "class Small {}")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Large.java"), "class Large {\n\tpublic static void main(String[] args) {}\n}")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Medium.java"), "class Medium {\n\tvoid method() {}\n}")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	if len(modules[0].TopFiles) != 3 {
		t.Fatalf("len(module.TopFiles) = %d, want 3", len(modules[0].TopFiles))
	}
	if modules[0].TopFiles[0].Size < modules[0].TopFiles[1].Size || modules[0].TopFiles[1].Size < modules[0].TopFiles[2].Size {
		t.Fatalf("TopFiles not sorted by descending size: %+v", modules[0].TopFiles)
	}
	if modules[0].Heaviest.Path != modules[0].TopFiles[0].Path {
		t.Fatalf("Heaviest.Path = %q, want TopFiles[0].Path = %q", modules[0].Heaviest.Path, modules[0].TopFiles[0].Path)
	}
}

func TestJavaDetectorMultiModule(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "module-a", "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "module-a", "src", "main", "java", "A.java"), "class A {}")
	writeTestFile(t, filepath.Join(tmpDir, "module-b", "build.gradle"), "")
	writeTestFile(t, filepath.Join(tmpDir, "module-b", "src", "main", "java", "B.java"), "class B {}")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want 2 (multi-module)", len(modules))
	}
}

func TestJavaDetectorDoesNotDuplicatePackagesAcrossOverlappingSourceRoots(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "api", "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "api", "src", "main", "java", "com", "example", "api", "RestEndpoint.java"), "package com.example.api;\nclass RestEndpoint {}")
	writeTestFile(t, filepath.Join(tmpDir, "cli", "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "cli", "src", "main", "java", "com", "example", "cli", "Main.java"), "package com.example.cli;\nclass Main {}")
	writeTestFile(t, filepath.Join(tmpDir, "core", "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "core", "src", "main", "java", "com", "example", "core", "Service.java"), "package com.example.core;\nclass Service {}")

	modules, err := NewJavaDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 3 {
		t.Fatalf("len(modules) = %d, want 3", len(modules))
	}

	for _, module := range modules {
		if len(module.SourceRoots) != 1 {
			t.Fatalf("module %s source roots = %+v, want exactly one non-overlapping source root", module.Name, module.SourceRoots)
		}
		if len(module.SourceRoots[0].Packages) != 1 {
			t.Fatalf("module %s packages = %+v, want exactly one package", module.Name, module.SourceRoots[0].Packages)
		}
		if module.SourceRoots[0].Packages[0].FileCount != 1 {
			t.Fatalf("module %s package file count = %d, want 1", module.Name, module.SourceRoots[0].Packages[0].FileCount)
		}
	}
}

func TestJavaDetectorRootFallback(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Main.java"), "class Main {}")

	detector := NewJavaDetector()
	modules, err := detector.Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if modules[0].Name == "" {
		t.Fatalf("module.Name should not be empty for root module")
	}
}

func hasJavaSourceRoot(module model.Module, path string) bool {
	for _, sr := range module.SourceRoots {
		if sr.Path == path {
			return true
		}
	}
	return false
}

func findJavaSourceRoot(module model.Module, path string) *model.SourceRoot {
	for _, sr := range module.SourceRoots {
		if sr.Path == path {
			sr := sr
			return &sr
		}
	}
	return nil
}
