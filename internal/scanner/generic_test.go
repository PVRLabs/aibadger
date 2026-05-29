package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestGenericDetector(t *testing.T) {
	// Create a temp directory structure
	tmpDir, err := os.MkdirTemp("", "badger-generic-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Add some files
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "src", "main.py"), []byte("print('hello')"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("# Test"), 0644)

	detector := NewGenericDetector()
	modules, err := detector.Detect(tmpDir)

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(modules) != 1 {
		t.Errorf("expected 1 module, got %d", len(modules))
	}

	if modules[0].Language != "Python" {
		t.Errorf("expected language Python, got %s", modules[0].Language)
	}

	// Verify exclusions
	os.MkdirAll(filepath.Join(tmpDir, ".git"), 0755)
	os.WriteFile(filepath.Join(tmpDir, ".git", "config"), []byte(""), 0644)

	modules, _ = detector.Detect(tmpDir)
	for _, m := range modules {
		for _, sr := range m.SourceRoots {
			for _, pkg := range sr.Packages {
				if pkg.Path == ".git" {
					t.Errorf("found .git package but it should be excluded")
				}
			}
		}
	}
}

func TestGenericDetectorLabelsAssetFilesAndOmitsRootBinary(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(strings.Repeat("h", 2048)), 0644)
	os.WriteFile(filepath.Join(tmpDir, "android-chrome-512x512.png"), []byte(strings.Repeat("p", 3072)), 0644)
	os.WriteFile(filepath.Join(tmpDir, "badger"), []byte{0, 1, 2, 3, 4, 5}, 0755)

	modules, err := NewGenericDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	rootPkg := findGenericPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatal("root package not found")
	}
	if len(rootPkg.TopFiles) != 1 {
		t.Fatalf("len(rootPkg.TopFiles) = %d, want 1", len(rootPkg.TopFiles))
	}
	if rootPkg.TopFiles[0].Path != "index.html" || rootPkg.TopFiles[0].Kind != model.FileKindSource {
		t.Fatalf("first top file = %+v, want source index.html", rootPkg.TopFiles[0])
	}
	if len(rootPkg.AuxFiles) != 1 {
		t.Fatalf("len(rootPkg.AuxFiles) = %d, want 1", len(rootPkg.AuxFiles))
	}
	if rootPkg.AuxFiles[0].Path != "android-chrome-512x512.png" || rootPkg.AuxFiles[0].Kind != model.FileKindAsset {
		t.Fatalf("aux file = %+v, want asset png", rootPkg.AuxFiles[0])
	}
}

func TestGenericDetectorUsesFallbackTopFileBudgetForRootAndNestedPackages(t *testing.T) {
	tmpDir := t.TempDir()

	rootFiles := map[string]string{
		"README.md":      "# root\n",
		"AGENTS.md":      "# agents\n",
		"package.json":   "{\n  \"name\": \"root\"\n}\n",
		"pyproject.toml": "[project]\nname = \"root\"\n",
		"Dockerfile":     "FROM scratch\n",
		"Makefile":       "test:\n\ttrue\n",
		"LICENSE":        "MIT\n",
		"CHANGELOG.md":   "# changelog\n",
		"notes.txt":      "notes\n",
		"index.html":     "<html></html>\n",
		"app.ts":         "export const app = true\n",
	}
	for relPath, contents := range rootFiles {
		writeTestFile(t, filepath.Join(tmpDir, relPath), contents)
	}
	writeTestFile(t, filepath.Join(tmpDir, "src", "a.py"), "print('a')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "b.py"), "print('b')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "c.py"), "print('c')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "d.py"), "print('d')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "e.py"), "print('e')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "f.py"), "print('f')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "g.py"), "print('g')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "h.py"), "print('h')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "i.py"), "print('i')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "j.py"), "print('j')\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "k.py"), "print('k')\n")

	modules, err := NewGenericDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	rootPkg := findGenericPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatal("root package not found")
	}
	if len(rootPkg.TopFiles) != maxRootPackageTopFiles {
		t.Fatalf("len(rootPkg.TopFiles) = %d, want %d", len(rootPkg.TopFiles), maxRootPackageTopFiles)
	}

	srcPkg := findGenericPackage(modules[0], "src")
	if srcPkg == nil {
		t.Fatal("nested src package not found")
	}
	if len(srcPkg.TopFiles) != maxGenericPackageFiles {
		t.Fatalf("len(srcPkg.TopFiles) = %d, want %d", len(srcPkg.TopFiles), maxGenericPackageFiles)
	}
}

func TestGenericDetectorOmitsNoiseFiles(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string][]byte{
		".DS_Store":         []byte("junk"),
		"Thumbs.db":         []byte("junk"),
		"go.sum":            []byte("module sum"),
		"package-lock.json": []byte("{}"),
		"pnpm-lock.yaml":    []byte("lockfileVersion: 5"),
		"yarn.lock":         []byte("# yarn lockfile"),
		"target/app.jar":    []byte("jar"),
		"build/Main.class":  []byte("class"),
		"bin/native.dylib":  []byte("binary"),
		"Dockerfile":        []byte("FROM scratch"),
		"Makefile":          []byte("test:\n\tgo test ./..."),
		"LICENSE":           []byte("MIT"),
	}
	for relPath, data := range files {
		path := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	modules, err := NewGenericDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	var paths []string
	for _, sr := range modules[0].SourceRoots {
		for _, pkg := range sr.Packages {
			for _, file := range pkg.TopFiles {
				paths = append(paths, file.Path)
			}
			for _, file := range pkg.AuxFiles {
				paths = append(paths, file.Path)
			}
		}
	}

	for _, omitted := range []string{".DS_Store", "Thumbs.db", "go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", filepath.Join("target", "app.jar"), filepath.Join("build", "Main.class"), filepath.Join("bin", "native.dylib")} {
		for _, path := range paths {
			if path == omitted {
				t.Fatalf("omitted file %q surfaced in topology paths: %v", omitted, paths)
			}
		}
	}
	for _, preserved := range []string{"Dockerfile", "Makefile", "LICENSE"} {
		found := false
		for _, path := range paths {
			if path == preserved {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("preserved file %q missing from topology paths: %v", preserved, paths)
		}
	}
}

func TestGenericDetectorOmitsSensitivePrompt1Files(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string][]byte{
		".env":                  []byte("SECRET=1"),
		".env.local":            []byte("LOCAL=1"),
		".aws/credentials":      []byte("[default]\naws_access_key_id=1"),
		"keys/id_rsa":           []byte("-----BEGIN OPENSSH PRIVATE KEY-----"),
		"src/main.go":           []byte("package main\n"),
		"assets/logo.png":       []byte("png"),
		"notes/readme.txt":      []byte("safe"),
		".azure/token.json":     []byte("{}"),
		".gcp/credentials.json": []byte("{}"),
	}
	for relPath, data := range files {
		path := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	modules, err := NewGenericDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	var paths []string
	for _, sr := range modules[0].SourceRoots {
		for _, pkg := range sr.Packages {
			for _, file := range pkg.TopFiles {
				paths = append(paths, file.Path)
			}
			for _, file := range pkg.AuxFiles {
				paths = append(paths, file.Path)
			}
		}
	}

	for _, omitted := range []string{".env", ".env.local", filepath.Join(".aws", "credentials"), filepath.Join("keys", "id_rsa"), filepath.Join(".azure", "token.json"), filepath.Join(".gcp", "credentials.json")} {
		for _, path := range paths {
			if path == omitted {
				t.Fatalf("sensitive file %q surfaced in topology paths: %v", omitted, paths)
			}
		}
	}
	for _, preserved := range []string{"src/main.go", "assets/logo.png", "notes/readme.txt"} {
		found := false
		for _, path := range paths {
			if path == preserved {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected preserved path %q in topology paths: %v", preserved, paths)
		}
	}
}

func TestGenericFallbackSurfacesControlConfigPrompt1(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string]string{
		"Dockerfile":            "FROM scratch\n",
		"Makefile":              "test:\n\tgo test ./...\n",
		"Taskfile.yml":          "version: '3'\n",
		"justfile":              "test:\n\tgo test ./...\n",
		".gitignore":            "tmp\n",
		".dockerignore":         ".git\n",
		"README.md":             "# Generic fixture\n",
		"AGENTS.md":             "# Agent notes\n",
		"LICENSE":               "MIT\n",
		"config/app.toml":       "name = 'app'\n",
		"config/app.yaml":       "name: app\n",
		"config/app.yml":        "name: app\n",
		"config/app.json":       `{"name":"app"}`,
		"config/app.xml":        "<app />\n",
		"config/app.ini":        "[app]\n",
		"config/app.conf":       "enabled=true\n",
		"config/app.properties": "enabled=true\n",
		"notes/overview.txt":    "plain text\n",
	}
	for relPath, contents := range files {
		writeTestFile(t, filepath.Join(tmpDir, relPath), contents)
	}

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if got := topology.PrimaryLanguage; got != "Generic" {
		t.Fatalf("PrimaryLanguage = %q, want Generic", got)
	}

	output := protocol.NewFormatter().GenerateSchemaA(topology, "summarize this repository")
	for _, want := range []string{
		"Languages: Generic",
		"Pkg: . [9 files] -> Top:",
		"Dockerfile",
		"Makefile",
		"Taskfile.yml",
		"justfile",
		".gitignore",
		".dockerignore",
		"README.md",
		"AGENTS.md",
		"LICENSE",
		"Pkg: config [8 files] -> Top:",
		"app.toml",
		"app.yaml",
		"app.yml",
		"app.json",
		"app.xml",
		"app.ini",
		"app.conf",
		"app.properties",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("Prompt 1 missing %q:\n%s", want, output)
		}
	}
}

func TestGenericFallbackSurfacesRootIndexHTMLBeforeWebMetadata(t *testing.T) {
	tmpDir := t.TempDir()

	files := map[string]string{
		"index.html":       "<!doctype html>\n",
		"script.js":        "console.log('ok')\n",
		"style.css":        "body { color: black; }\n",
		"sitemap.xml":      "<?xml version=\"1.0\"?>\n",
		"site.webmanifest": `{"name":"test"}`,
		"robots.txt":       "User-agent: *\n",
		"BingSiteAuth.xml": "<users />\n",
		"google.html":      "verification\n",
	}
	for relPath, contents := range files {
		writeTestFile(t, filepath.Join(tmpDir, relPath), contents)
	}

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	output := protocol.NewFormatter().GenerateSchemaA(topology, "inspect static site")
	rootLine := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "Pkg: . ") {
			rootLine = line
			break
		}
	}
	if rootLine == "" {
		t.Fatalf("Prompt 1 missing root package line:\n%s", output)
	}
	if !strings.Contains(rootLine, "index.html") {
		t.Fatalf("root package line missing index.html:\n%s", output)
	}
	if strings.Index(rootLine, "sitemap.xml") != -1 && strings.Index(rootLine, "index.html") > strings.Index(rootLine, "sitemap.xml") {
		t.Fatalf("root package line ranks sitemap.xml before index.html:\n%s", rootLine)
	}
}

func TestGenericFallbackOmitsGeneratedDirsAndSensitiveFiles(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string]string{
		"README.md":                  "# Generic fixture\n",
		"config/app.conf":            "enabled=true\n",
		".git/config":                "secret-ish\n",
		"node_modules/pkg/index.txt": "vendor\n",
		"target/classes/app.txt":     "generated\n",
		"build/output/app.txt":       "generated\n",
		"dist/app.txt":               "generated\n",
		".venv/lib/site.py":          "print('ignored')\n",
		"venv/lib/site.py":           "print('ignored')\n",
		"__pycache__/module.pyc":     "ignored\n",
		".pytest_cache/state.txt":    "ignored\n",
		".mypy_cache/state.txt":      "ignored\n",
		".ruff_cache/state.txt":      "ignored\n",
		"coverage/report.txt":        "ignored\n",
		".env":                       "SECRET=1\n",
		".env.local":                 "SECRET=1\n",
		".env.production":            "SECRET=1\n",
		"keys/server.pem":            "secret\n",
		"keys/server.key":            "secret\n",
		"config/credentials":         "secret\n",
		"config/credentials.json":    "{}\n",
		"config/secrets.json":        "{}\n",
	}
	for relPath, contents := range files {
		writeTestFile(t, filepath.Join(tmpDir, relPath), contents)
	}

	modules, err := NewGenericDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	paths := genericModulePaths(modules[0])

	for _, omitted := range []string{
		filepath.Join(".git", "config"),
		filepath.Join("node_modules", "pkg", "index.txt"),
		filepath.Join("target", "classes", "app.txt"),
		filepath.Join("build", "output", "app.txt"),
		filepath.Join("dist", "app.txt"),
		filepath.Join(".venv", "lib", "site.py"),
		filepath.Join("venv", "lib", "site.py"),
		filepath.Join("__pycache__", "module.pyc"),
		filepath.Join(".pytest_cache", "state.txt"),
		filepath.Join(".mypy_cache", "state.txt"),
		filepath.Join(".ruff_cache", "state.txt"),
		filepath.Join("coverage", "report.txt"),
		".env",
		".env.local",
		".env.production",
		filepath.Join("keys", "server.pem"),
		filepath.Join("keys", "server.key"),
		filepath.Join("config", "credentials"),
		filepath.Join("config", "credentials.json"),
		filepath.Join("config", "secrets.json"),
	} {
		if containsPath(paths, omitted) {
			t.Fatalf("omitted path %q surfaced in topology paths: %v", omitted, paths)
		}
	}
	for _, preserved := range []string{"README.md", filepath.Join("config", "app.conf")} {
		if !containsPath(paths, preserved) {
			t.Fatalf("expected preserved path %q in topology paths: %v", preserved, paths)
		}
	}
}

func genericModulePaths(module model.Module) []string {
	var paths []string
	for _, sr := range module.SourceRoots {
		for _, pkg := range sr.Packages {
			for _, file := range pkg.TopFiles {
				paths = append(paths, file.Path)
			}
			for _, file := range pkg.AuxFiles {
				paths = append(paths, file.Path)
			}
		}
	}
	return paths
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func findGenericPackage(module model.Module, path string) *model.Package {
	for _, sr := range module.SourceRoots {
		for i := range sr.Packages {
			if sr.Packages[i].Path == path {
				return &sr.Packages[i]
			}
		}
	}
	return nil
}
