package scanner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestPythonDetectorReportsMarkerOnlyProject(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pyproject.toml"), "[project]\nname = \"tools\"\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Python"}) {
		t.Fatalf("Languages = %v, want [Python]", topology.Languages)
	}
	if topology.PrimaryLanguage != "Python" {
		t.Fatalf("PrimaryLanguage = %q, want Python", topology.PrimaryLanguage)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}
	if topology.Modules[0].Language != "Python" {
		t.Fatalf("module language = %q, want Python", topology.Modules[0].Language)
	}
}

func TestPythonDetectorRecognizesCommonLayouts(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "requirements.txt"), "pytest\n")
	writeTestFile(t, filepath.Join(tmpDir, "manage.py"), "def main(): pass\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "app", "__init__.py"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "app", "service.py"), "def service(): return True\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "conftest.py"), "import pytest\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "test_service.py"), "def test_service(): pass\n")
	writeTestFile(t, filepath.Join(tmpDir, "workers", "__init__.py"), "")
	writeTestFile(t, filepath.Join(tmpDir, "workers", "jobs.py"), "def run(): pass\n")

	modules, err := NewPythonDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	module := modules[0]
	for _, path := range []string{"", "src", "tests", "workers"} {
		if !hasSourceRoot(module, path) {
			t.Fatalf("SourceRoots = %+v, missing %q", module.SourceRoots, path)
		}
	}
	for _, path := range []string{
		"manage.py",
		filepath.Join("src", "app", "service.py"),
		filepath.Join("tests", "conftest.py"),
		filepath.Join("tests", "test_service.py"),
		filepath.Join("workers", "jobs.py"),
	} {
		if !moduleHasPackageTopFile(module, path) {
			t.Fatalf("module packages = %+v, missing %s", module.SourceRoots, path)
		}
	}
}

func TestScannerAddsPythonToMixedLanguageTopology(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/api\n")
	for _, relPath := range []string{
		"main.go",
		filepath.Join("internal", "api", "handler.go"),
		filepath.Join("internal", "api", "server.go"),
		filepath.Join("internal", "config", "config.go"),
		filepath.Join("pkg", "client", "client.go"),
	} {
		writeTestFile(t, filepath.Join(tmpDir, relPath), "package main\n")
	}
	writeTestFile(t, filepath.Join(tmpDir, "tests", "conftest.py"), "import pytest\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "test_fixtures.py"), "def test_fixture(): pass\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "test_stress.py"), "def test_stress(): pass\n")

	for i := 0; i < 10; i++ {
		topology, err := NewScanner(tmpDir).Scan()
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if !reflect.DeepEqual(topology.Languages, []string{"Go", "Python"}) {
			t.Fatalf("Languages = %v, want [Go Python]", topology.Languages)
		}
		if topology.PrimaryLanguage != "Go" {
			t.Fatalf("PrimaryLanguage = %q, want Go", topology.PrimaryLanguage)
		}
		if len(topology.Modules) != 2 {
			t.Fatalf("len(Modules) = %d, want 2", len(topology.Modules))
		}

		pythonModule := findModule(topology.Modules, filepath.Base(tmpDir))
		if pythonModule == nil || pythonModule.Language != "Python" {
			t.Fatalf("Modules = %+v, missing Python module", topology.Modules)
		}
		for _, path := range []string{
			filepath.Join("tests", "conftest.py"),
			filepath.Join("tests", "test_fixtures.py"),
			filepath.Join("tests", "test_stress.py"),
		} {
			if !moduleHasPackageTopFile(*pythonModule, path) {
				t.Fatalf("Python module = %+v, missing %s", pythonModule, path)
			}
		}
	}
}

func TestPythonTopologyPromptSurfacesBadgerCertPytestFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"cert-harness","main":"src/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "console.log('cert')\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "helpers.py"), strings.Repeat("x", 5000))
	writeTestFile(t, filepath.Join(tmpDir, "tests", "conftest.py"), "import pytest\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "test_fixtures.py"), "def test_fixture(): pass\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "test_stress.py"), "def test_stress(): pass\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	output := protocol.NewFormatter().GenerateSchemaA(topology, "inspect cert topology")

	if !strings.Contains(output, "Languages: JavaScript, Python") {
		t.Fatalf("topology prompt missing mixed languages:\n%s", output)
	}
	if !strings.Contains(output, "Pkg: tests ") {
		t.Fatalf("topology prompt missing tests package:\n%s", output)
	}
	for _, want := range []string{"conftest.py", "test_fixtures.py", "test_stress.py"} {
		if !strings.Contains(output, want) {
			t.Fatalf("topology prompt missing %s:\n%s", want, output)
		}
	}
	if strings.Contains(output, "helpers.py") {
		t.Fatalf("topology prompt should rank pytest files above helpers.py:\n%s", output)
	}
}
