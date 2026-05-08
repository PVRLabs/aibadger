package scanner

import (
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestScanIncludesStandardStaticWebResources(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/web\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "app", "main.go"), "package app\n\nfunc Main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "index.html"), "<!doctype html>\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "app.js"), "console.log('ok')\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "logo.png"), "png")
	writeTestFile(t, filepath.Join(tmpDir, "static", "site.css"), "body { color: black; }\n")
	writeTestFile(t, filepath.Join(tmpDir, "assets", "hero.svg"), "<svg></svg>\n")
	writeTestFile(t, filepath.Join(tmpDir, "assets", "ignored.jar"), "jar")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}

	module := topology.Modules[0]
	if module.FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want only Go source files counted for language weighting", module.FileCount)
	}
	for _, rootPath := range []string{"public", "static", "assets"} {
		root := findSourceRoot(module, rootPath)
		if root == nil {
			t.Fatalf("module.SourceRoots = %+v, missing web root %s", module.SourceRoots, rootPath)
		}
		if root.Role != "Web Resources" {
			t.Fatalf("web root %s Role = %q, want Web Resources", rootPath, root.Role)
		}
	}
	if root := findSourceRoot(module, "public"); root.FileCount != 3 {
		t.Fatalf("public FileCount = %d, want 3 non-omitted files without duplicate standalone/delegated counting", root.FileCount)
	}

	publicPkg := findPackage(module, "public")
	if publicPkg == nil {
		t.Fatalf("module packages missing public package")
	}
	for _, path := range []string{
		filepath.Join("public", "index.html"),
		filepath.Join("public", "app.js"),
	} {
		if !hasTopFile(publicPkg.TopFiles, path) {
			t.Fatalf("publicPkg.TopFiles = %+v, missing %s", publicPkg.TopFiles, path)
		}
	}
	if !hasAuxFile(publicPkg.AuxFiles, filepath.Join("public", "logo.png")) {
		t.Fatalf("publicPkg.AuxFiles = %+v, missing public/logo.png", publicPkg.AuxFiles)
	}

	assetsPkg := findPackage(module, "assets")
	if assetsPkg == nil {
		t.Fatalf("module packages missing assets package")
	}
	if !hasAuxFile(assetsPkg.AuxFiles, filepath.Join("assets", "hero.svg")) {
		t.Fatalf("assetsPkg.AuxFiles = %+v, missing assets/hero.svg", assetsPkg.AuxFiles)
	}
	if hasAuxFile(assetsPkg.AuxFiles, filepath.Join("assets", "ignored.jar")) || hasTopFile(assetsPkg.TopFiles, filepath.Join("assets", "ignored.jar")) {
		t.Fatalf("assets package surfaced omitted jar: top=%+v aux=%+v", assetsPkg.TopFiles, assetsPkg.AuxFiles)
	}
}

func TestGoDetectorDelegatesStaticWebResources(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/go-web\n")
	writeTestFile(t, filepath.Join(tmpDir, "cmd", "server", "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "index.html"), "<!doctype html>\n")
	writeTestFile(t, filepath.Join(tmpDir, "static", "app.css"), "body {}\n")

	modules, err := NewGoDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if modules[0].FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want only Go source files counted", modules[0].FileCount)
	}

	for _, path := range []string{"public", "static"} {
		root := findSourceRoot(modules[0], path)
		if root == nil {
			t.Fatalf("module.SourceRoots = %+v, missing delegated web root %s", modules[0].SourceRoots, path)
		}
		if root.Role != "Web Resources" {
			t.Fatalf("source root %s Role = %q, want Web Resources", path, root.Role)
		}
	}
	if !hasTopFile(findPackageOrFail(t, modules[0], "public").TopFiles, filepath.Join("public", "index.html")) {
		t.Fatalf("public package missing index.html")
	}
	if !hasTopFile(findPackageOrFail(t, modules[0], "static").TopFiles, filepath.Join("static", "app.css")) {
		t.Fatalf("static package missing app.css")
	}
}

func TestJavaDetectorDelegatesStaticWebResources(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "Main.java"), "class Main {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "resources", "static", "index.html"), "<!doctype html>\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "resources", "static", "css", "site.css"), "body {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "health.html"), "ok\n")

	modules, err := NewJavaDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if modules[0].FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want only Java source files counted", modules[0].FileCount)
	}

	staticRootPath := filepath.Join("src", "main", "resources", "static")
	staticRoot := findSourceRoot(modules[0], staticRootPath)
	if staticRoot == nil {
		t.Fatalf("module.SourceRoots = %+v, missing delegated Java static root", modules[0].SourceRoots)
	}
	if staticRoot.Role != "Web Resources" {
		t.Fatalf("staticRoot.Role = %q, want Web Resources", staticRoot.Role)
	}
	if !hasTopFile(findPackageOrFail(t, modules[0], staticRootPath).TopFiles, filepath.Join(staticRootPath, "index.html")) {
		t.Fatalf("static root package missing index.html")
	}
	if !hasTopFile(findPackageOrFail(t, modules[0], filepath.Join(staticRootPath, "css")).TopFiles, filepath.Join(staticRootPath, "css", "site.css")) {
		t.Fatalf("static css package missing site.css")
	}
	if !hasTopFile(findPackageOrFail(t, modules[0], "public").TopFiles, filepath.Join("public", "health.html")) {
		t.Fatalf("public package missing health.html")
	}
}

func hasAuxFile(files []model.FileSummary, path string) bool {
	for _, file := range files {
		if file.Path == path {
			return true
		}
	}
	return false
}

func findPackageOrFail(t *testing.T, module model.Module, path string) *model.Package {
	t.Helper()
	pkg := findPackage(module, path)
	if pkg == nil {
		t.Fatalf("module packages missing %s in source roots: %+v", path, module.SourceRoots)
	}
	return pkg
}
