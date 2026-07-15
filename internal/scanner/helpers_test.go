package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestCloneExclusions(t *testing.T) {
	cloned := cloneExclusions(commonIgnoredDirs, "vendor")
	if !cloned[".git"] {
		t.Fatalf("expected cloned exclusions to retain common ignored dirs")
	}
	if !cloned["vendor"] {
		t.Fatalf("expected cloned exclusions to include extras")
	}

	cloned["vendor"] = false
	if commonIgnoredDirs["vendor"] {
		t.Fatalf("mutating cloned exclusions should not affect commonIgnoredDirs")
	}
}

func TestDiscoverProjectMarkersHonorsDepthLimit(t *testing.T) {
	root := t.TempDir()
	depthFour := filepath.Join(root, "one", "two", "three", "four", "go.mod")
	depthFive := filepath.Join(root, "one", "two", "three", "four", "five", "go.mod")
	writeTestFile(t, depthFour, "module example.com/depth4\n")
	writeTestFile(t, depthFive, "module example.com/depth5\n")

	paths, err := discoverProjectMarkers(root, "go.mod")
	if err != nil {
		t.Fatalf("discoverProjectMarkers() error = %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want 1: %v", len(paths), paths)
	}
	if paths[0] != depthFour {
		t.Fatalf("paths[0] = %q, want depth-4 marker %q", paths[0], depthFour)
	}
}

func TestDiscoverProjectMarkersSkipsCommonExcludedDirs(t *testing.T) {
	root := t.TempDir()
	found := filepath.Join(root, "apps", "web", "package.json")
	writeTestFile(t, found, `{"name":"web"}`)

	for _, dir := range []string{
		".git",
		"node_modules",
		"vendor",
		"dist",
		"build",
		"target",
		".gocache",
		".idea",
		".vscode",
	} {
		writeTestFile(t, filepath.Join(root, dir, "nested", "package.json"), `{"name":"ignored"}`)
	}

	paths, err := discoverProjectMarkers(root, "package.json")
	if err != nil {
		t.Fatalf("discoverProjectMarkers() error = %v", err)
	}

	if len(paths) != 1 {
		t.Fatalf("len(paths) = %d, want only unexcluded marker: %v", len(paths), paths)
	}
	if paths[0] != found {
		t.Fatalf("paths[0] = %q, want %q", paths[0], found)
	}
}

func TestRelativePathHelpers(t *testing.T) {
	root := filepath.Join("tmp", "project")
	file := filepath.Join(root, "src", "main.go")

	if got := relativePath(root, root); got != "" {
		t.Fatalf("relativePath(root, root) = %q, want empty string", got)
	}
	if got := relativePath(root, file); got != filepath.Join("src", "main.go") {
		t.Fatalf("relativePath() = %q", got)
	}
	if got := relativeDirFromFile(root, file); got != "src" {
		t.Fatalf("relativeDirFromFile() = %q, want %q", got, "src")
	}
	if got := normalizeRelativeDir("."); got != "" {
		t.Fatalf("normalizeRelativeDir(\".\") = %q, want empty string", got)
	}
}

func TestShouldOmitFile(t *testing.T) {
	root := t.TempDir()
	write := func(relPath string, data []byte) string {
		path := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	tests := []struct {
		relPath string
		data    []byte
		want    bool
	}{
		{relPath: ".DS_Store", data: []byte("junk"), want: true},
		{relPath: "Thumbs.db", data: []byte("junk"), want: true},
		{relPath: "go.sum", data: []byte("module sum"), want: true},
		{relPath: "package-lock.json", data: []byte("{}"), want: true},
		{relPath: "lib/app.jar", data: []byte("jar"), want: true},
		{relPath: "Main.class", data: []byte("class"), want: true},
		{relPath: filepath.Join("build", "tmp.txt"), data: []byte("generated"), want: false},
		{relPath: "badger", data: []byte{0, 1, 2, 3}, want: true},
		{relPath: ".gitignore", data: []byte("tmp\n"), want: true},
		{relPath: ".dockerignore", data: []byte(".git\n"), want: true},
		{relPath: filepath.Join("bin", "badger"), data: []byte{0, 1, 2, 3}, want: false},
		{relPath: "Dockerfile", data: []byte("FROM scratch"), want: false},
		{relPath: "Makefile", data: []byte("build:\n\tgo test ./..."), want: false},
		{relPath: "LICENSE", data: []byte("MIT"), want: false},
		{relPath: "README.md", data: []byte("# Project"), want: false},
	}

	for _, tt := range tests {
		path := write(tt.relPath, tt.data)
		if got := shouldOmitFile(root, path, filepath.Base(path)); got != tt.want {
			t.Fatalf("shouldOmitFile(%q) = %v, want %v", tt.relPath, got, tt.want)
		}
	}
}

func TestReviewPathPriorityPathCategories(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		relPath  string
		priority int
		ok       bool
	}{
		{name: "source path", relPath: filepath.Join("internal", "api", "new_client.go"), priority: 1, ok: true},
		{name: "Go test source path", relPath: filepath.Join("internal", "model", "topology_test.go"), priority: 1, ok: true},
		{name: "asset path", relPath: filepath.Join("media", "logo.png"), priority: 0, ok: true},
		{name: "binary path", relPath: filepath.Join("bin", "tool.exe"), priority: 0, ok: true},
		{name: "unknown extension", relPath: filepath.Join("notes", "draft.xyz"), priority: 0, ok: true},
		{name: "extensionless path", relPath: filepath.Join("tools", "badger"), priority: 0, ok: true},
		{name: "generated path", relPath: filepath.Join("build", "tmp.txt"), priority: 0, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ReviewPathPriority(root, filepath.Join(root, tt.relPath))
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.priority {
				t.Fatalf("priority = %d, want %d", got, tt.priority)
			}
		})
	}
}

func TestReviewPathPrioritySeparatorHandling(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name     string
		relPath  string
		priority int
	}{
		{name: "root index html", relPath: "index.html", priority: 1},
		{name: "nested index html", relPath: filepath.Join("docs", "index.html"), priority: 1},
		{name: "public path", relPath: filepath.Join("public", "app.js"), priority: 1},
		{name: "static path", relPath: filepath.Join("static", "style.css"), priority: 1},
		{name: "assets path", relPath: filepath.Join("assets", "logo.png"), priority: 1},
		{name: "nested source path", relPath: filepath.Join("internal", "api", "service.go"), priority: 1},
		{name: "top-level tests path", relPath: filepath.Join("tests", "data.xyz"), priority: 1},
		{name: "top-level testdata path", relPath: filepath.Join("testdata", "data.xyz"), priority: 1},
		{name: "top-level fixtures path", relPath: filepath.Join("fixtures", "data.xyz"), priority: 1},
		{name: "top-level migrations path", relPath: filepath.Join("migrations", "data.xyz"), priority: 1},
		{name: "top-level schema path", relPath: filepath.Join("schema", "data.xyz"), priority: 1},
		{name: "top-level sample path", relPath: filepath.Join("sample", "data.xyz"), priority: 1},
		{name: "nested category path", relPath: filepath.Join("pkg", "testdata", "data.xyz"), priority: 1},
		{name: "binary in tests", relPath: filepath.Join("tests", "tool.exe"), priority: 0},
		{name: "asset in fixtures", relPath: filepath.Join("fixtures", "logo.png"), priority: 0},
		{name: "archive in sample", relPath: filepath.Join("sample", "archive.jar"), priority: 0},
		{name: "migration filename only", relPath: "migration-notes.log", priority: 0},
		{name: "test filename only", relPath: "test_output.xyz", priority: 0},
		{name: "similar directory name", relPath: filepath.Join("samples", "data.xyz"), priority: 0},
		{name: "nested root web resource", relPath: filepath.Join("docs", "favicon.ico"), priority: 0},
		{name: "nested robots file", relPath: filepath.Join("examples", "robots.txt"), priority: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ReviewPathPriority(root, filepath.Join(root, tt.relPath))
			if !ok {
				t.Fatalf("ok = false, want true")
			}
			if got != tt.priority {
				t.Fatalf("priority = %d, want %d", got, tt.priority)
			}
		})
	}
}

func TestJavaPackageName(t *testing.T) {
	if got := javaPackageName(""); got != "default" {
		t.Fatalf("javaPackageName(\"\") = %q, want %q", got, "default")
	}
	if got := javaPackageName(filepath.Join("com", "example")); got != "com.example" {
		t.Fatalf("javaPackageName() = %q, want %q", got, "com.example")
	}
}

func TestAddTopFileCapsAndOrdersBySize(t *testing.T) {
	var files []model.FileSummary
	for _, file := range []model.FileSummary{
		{Name: "small.go", Path: "pkg/small.go", Size: 100},
		{Name: "large.go", Path: "pkg/large.go", Size: 300},
		{Name: "medium.go", Path: "pkg/medium.go", Size: 200},
		{Name: "huge.go", Path: "pkg/huge.go", Size: 400},
	} {
		files = addTopFile(files, file, 3)
	}

	if len(files) != 3 {
		t.Fatalf("len(files) = %d, want 3", len(files))
	}
	for i, want := range []string{"pkg/huge.go", "pkg/large.go", "pkg/medium.go"} {
		if files[i].Path != want {
			t.Fatalf("files[%d].Path = %q, want %q", i, files[i].Path, want)
		}
	}
}

func TestPackageTopFileLimit(t *testing.T) {
	if got := packageTopFileLimit("", maxPackageTopFiles); got != maxRootPackageTopFiles {
		t.Fatalf("packageTopFileLimit(\"\", %d) = %d, want %d", maxPackageTopFiles, got, maxRootPackageTopFiles)
	}
	if got := packageTopFileLimit("src", maxPackageTopFiles); got != maxPackageTopFiles {
		t.Fatalf("packageTopFileLimit(\"src\", %d) = %d, want %d", maxPackageTopFiles, got, maxPackageTopFiles)
	}
	if got := moduleTopFileLimit("", maxPackageTopFiles); got != maxRootPackageTopFiles {
		t.Fatalf("moduleTopFileLimit(\"\", %d) = %d, want %d", maxPackageTopFiles, got, maxRootPackageTopFiles)
	}
	if got := moduleTopFileLimit("pkg", maxPackageTopFiles); got != maxPackageTopFiles {
		t.Fatalf("moduleTopFileLimit(\"pkg\", %d) = %d, want %d", maxPackageTopFiles, got, maxPackageTopFiles)
	}
	if got := moduleTopFileLimit("pkg", 5); got != 5 {
		t.Fatalf("moduleTopFileLimit(\"pkg\", 5) = %d, want 5", got)
	}
}

func TestAddTopFileTieBreaksByPath(t *testing.T) {
	var files []model.FileSummary
	files = addTopFile(files, model.FileSummary{Name: "b.go", Path: "pkg/b.go", Size: 100}, 3)
	files = addTopFile(files, model.FileSummary{Name: "a.go", Path: "pkg/a.go", Size: 100}, 3)

	if files[0].Path != "pkg/a.go" || files[1].Path != "pkg/b.go" {
		t.Fatalf("files = %+v, want equal-size files sorted by path", files)
	}
}

func TestAddTopFilePrioritizesGuidanceDocsOverOperationalConfig(t *testing.T) {
	var files []model.FileSummary
	for _, file := range []model.FileSummary{
		{Name: "Dockerfile", Path: "Dockerfile", Size: 4096},
		{Name: "Makefile", Path: "Makefile", Size: 2048},
		{Name: "package.json", Path: "package.json", Size: 1024},
		{Name: "README.md", Path: "README.md", Size: 512},
		{Name: "AGENTS.md", Path: "AGENTS.md", Size: 256},
	} {
		files = addTopFile(files, file, 5)
	}

	want := []string{"README.md", "AGENTS.md", "package.json", "Dockerfile", "Makefile"}
	for i, path := range want {
		if files[i].Path != path {
			t.Fatalf("files[%d].Path = %q, want %q (files=%+v)", i, files[i].Path, path, files)
		}
	}
}

func TestAddTopFileRanksSourceBeforeAssetAndBinary(t *testing.T) {
	var files []model.FileSummary
	for _, file := range []model.FileSummary{
		{Name: "badger", Path: "badger", Size: 5000, Kind: model.FileKindBinary},
		{Name: "hero.png", Path: "hero.png", Size: 3000, Kind: model.FileKindAsset},
		{Name: "index.html", Path: "index.html", Size: 1000, Kind: model.FileKindSource},
	} {
		files = addTopFile(files, file, 3)
	}

	for i, want := range []string{"index.html", "hero.png", "badger"} {
		if files[i].Path != want {
			t.Fatalf("files[%d].Path = %q, want %q", i, files[i].Path, want)
		}
	}
}

func TestNormalizeWorkspacePattern(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
		ok      bool
	}{
		{pattern: "packages/ui", want: filepath.Join("packages", "ui"), ok: true},
		{pattern: "apps/*", want: filepath.Join("apps", "*"), ok: true},
		{pattern: "*", ok: false},
		{pattern: "apps/*/client", ok: false},
		{pattern: "packages/**", ok: false},
		{pattern: "../outside", ok: false},
	}

	for _, tt := range tests {
		got, ok := normalizeWorkspacePattern(tt.pattern)
		if ok != tt.ok {
			t.Fatalf("normalizeWorkspacePattern(%q) ok = %v, want %v", tt.pattern, ok, tt.ok)
		}
		if got != tt.want {
			t.Fatalf("normalizeWorkspacePattern(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}
