package scanner

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestScannerCoverageDoesNotRecoverSpecializedFilesBeyondSummaryCaps(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/capped\n")
	for idx := 0; idx < maxRootPackageTopFiles+3; idx++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("source%02d.go", idx)), "package capped\n")
	}
	writeTestFile(t, filepath.Join(root, "lib", "worker.rb"), "puts 'ok'\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range topology.Modules {
		if module.Coverage && module.Language == "Go" {
			t.Fatalf("specialized Go files beyond reporting caps were recovered as coverage: %+v", module)
		}
	}
	if coverageModuleContaining(topology.Modules, filepath.Join("lib", "worker.rb")) == nil {
		t.Fatal("genuinely unclaimed Ruby source was not recovered")
	}
	assertUniqueSurfacedPaths(t, topology)
}

func TestScannerCoverageDoesNotRecoverCSharpObjOutput(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "<Project />\n")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}\n")
	generated := filepath.Join("obj", "GeneratedAssemblyInfo.cs")
	writeTestFile(t, filepath.Join(root, generated), "class GeneratedAssemblyInfo {}\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topologyHasPackageTopFile(topology, generated) {
		t.Fatalf("generated C# obj output was recovered as coverage: %+v", topology.Modules)
	}
	if coverageModuleContaining(topology.Modules, generated) != nil {
		t.Fatal("generated C# obj output received coverage ownership")
	}
}

func TestCoverageLanguageWeightsUseAcceptedCounts(t *testing.T) {
	t.Run("rejected control in accepted package", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "scripts", "helper.py"), "print('ok')\n")
		writeTestFile(t, filepath.Join(root, "scripts", "deploy.py"), "print('control')\n")
		modules, err := collectUnclaimedSource(root, semanticSourceOwnership{projectRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if len(modules) != 1 || modules[0].FileCount != 1 {
			t.Fatalf("coverage=%+v, want only accepted helper.py", modules)
		}
		if got := sourceLanguageWeightsFromModules(modules, root)["Python"]; got != 1 {
			t.Fatalf("Python weight=%d, want accepted coverage FileCount 1", got)
		}
	})

	t.Run("per-directory scan cap", func(t *testing.T) {
		root := t.TempDir()
		for idx := 0; idx < 8; idx++ {
			writeTestFile(t, filepath.Join(root, "lib", fmt.Sprintf("worker%02d.rb", idx)), "puts 'ok'\n")
		}
		detector := NewGenericDetector()
		detector.maxFilesPerDir = 5
		modules, err := detector.collectUnclaimedSource(root, semanticSourceOwnership{projectRoot: root})
		if err != nil {
			t.Fatal(err)
		}
		if len(modules) != 1 || modules[0].FileCount != 5 {
			t.Fatalf("coverage=%+v, want five accepted files at scan cap", modules)
		}
		if got := sourceLanguageWeightsFromModules(modules, root)["Ruby"]; got != 5 {
			t.Fatalf("Ruby weight=%d, want bounded coverage FileCount 5", got)
		}
	})
}

func TestScannerWebResourceExclusionRequiresOwningModule(t *testing.T) {
	for _, nestedModule := range []bool{false, true} {
		t.Run(fmt.Sprintf("nestedModule=%v", nestedModule), func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/root\n")
			writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
			if nestedModule {
				writeTestFile(t, filepath.Join(root, "src", "go.mod"), "module example.com/child\n")
				writeTestFile(t, filepath.Join(root, "src", "main.go"), "package main\n")
			}
			paths := []string{
				filepath.Join("src", "assets", "tool.ts"),
				filepath.Join("internal", "static", "helper.py"),
				filepath.Join("lib", "public", "worker.rb"),
			}
			for _, path := range paths {
				writeTestFile(t, filepath.Join(root, path), "source\n")
			}
			topology, err := NewScanner(root).Scan()
			if err != nil {
				t.Fatal(err)
			}
			for idx, path := range paths {
				wantCoverage := idx != 0 || !nestedModule
				if got := coverageModuleContaining(topology.Modules, path) != nil; got != wantCoverage {
					t.Errorf("%s coverage=%v, want %v", path, got, wantCoverage)
				}
				count := 0
				for _, module := range topology.Modules {
					count += countPackageTopFile(module, path)
				}
				if count != 1 {
					t.Errorf("%s surfaced %d times, want exactly once", path, count)
				}
			}
		})
	}
}

func TestScannerJavaStaticResourcesDoNotBecomeLanguageCoverage(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "pom.xml"), "<project/>")
	writeTestFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "class App {}\n")
	staticPath := filepath.Join("src", "main", "resources", "static", "app.js")
	writeTestFile(t, filepath.Join(root, staticPath), "console.log('ok')\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Java"}) {
		t.Fatalf("Languages=%v, nested shared web resources must contribute zero language weight", topology.Languages)
	}
	for _, module := range topology.Modules {
		if module.Coverage {
			t.Fatalf("nested shared web resource created coverage: %+v", module)
		}
	}
	var javaModule *model.Module
	for idx := range topology.Modules {
		if topology.Modules[idx].Language == "Java" && !topology.Modules[idx].Coverage {
			javaModule = &topology.Modules[idx]
			break
		}
	}
	if javaModule == nil || !moduleHasPackageTopFile(*javaModule, staticPath) {
		t.Fatalf("nested static resource was not retained as shared Java context: %+v", topology.Modules)
	}
}

func TestScannerComposesSpecializedAndUnclaimedSource(t *testing.T) {
	tests := []struct {
		name      string
		files     map[string]string
		languages []string
		primary   string
		covered   string
	}{
		{
			name: "Go and C++",
			files: map[string]string{
				"go.mod": "module example.com/mixed\n", "main.go": "package main\n",
				filepath.Join("native", "main.cpp"): "int main() {}\n", filepath.Join("native", "support.cc"): "void support() {}\n",
			},
			languages: []string{"C++", "Go"}, primary: "C++", covered: filepath.Join("native", "main.cpp"),
		},
		{
			name: "Go and Ruby",
			files: map[string]string{
				"go.mod": "module example.com/mixed\n", "main.go": "package main\n", "worker.go": "package main\n",
				filepath.Join("lib", "worker.rb"): "puts 'ok'\n",
			},
			languages: []string{"Go", "Ruby"}, primary: "Go", covered: filepath.Join("lib", "worker.rb"),
		},
		{
			name: "Java and PHP",
			files: map[string]string{
				"pom.xml": "<project/>",
				filepath.Join("src", "main", "java", "App.java"):    "class App {}\n",
				filepath.Join("src", "main", "java", "Worker.java"): "class Worker {}\n",
				filepath.Join("php", "index.php"):                   "<?php echo 'ok';\n",
			},
			languages: []string{"Java", "PHP"}, primary: "Java", covered: filepath.Join("php", "index.php"),
		},
		{
			name: "Java and Kotlin",
			files: map[string]string{
				"pom.xml": "<project/>",
				filepath.Join("src", "main", "java", "App.java"):    "class App {}\n",
				filepath.Join("src", "main", "java", "Worker.java"): "class Worker {}\n",
				filepath.Join("src", "main", "kotlin", "App.kt"):    "class App\n",
			},
			languages: []string{"Java", "Kotlin"}, primary: "Java", covered: filepath.Join("src", "main", "kotlin", "App.kt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, contents := range tt.files {
				writeTestFile(t, filepath.Join(root, path), contents)
			}
			topology, err := NewScanner(root).Scan()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(topology.Languages, tt.languages) || topology.PrimaryLanguage != tt.primary {
				t.Fatalf("languages=%v primary=%q, want %v primary %q", topology.Languages, topology.PrimaryLanguage, tt.languages, tt.primary)
			}
			if topology.Structure != "Single Module" {
				t.Fatalf("Structure=%q, coverage must not change structural classification", topology.Structure)
			}
			coverage := coverageModuleContaining(topology.Modules, tt.covered)
			if coverage == nil || coverage.FileCount < 1 || coverage.FileCount != coverage.SourceRoots[0].FileCount {
				t.Fatalf("coverage for %s is incoherent: %+v", tt.covered, coverage)
			}
			assertUniqueSurfacedPaths(t, topology)
		})
	}
}

func TestScannerRejectsKotlinDSLControlsButIncludesKotlinSource(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "pom.xml"), "<project/>")
	writeTestFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "class App {}\n")
	writeTestFile(t, filepath.Join(root, "build.gradle.kts"), "plugins {}\n")
	writeTestFile(t, filepath.Join(root, "settings.gradle.kts"), "rootProject.name = \"app\"\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Java"}) {
		t.Fatalf("Languages=%v, Kotlin DSL controls must not create coverage", topology.Languages)
	}

	writeTestFile(t, filepath.Join(root, "src", "main", "kotlin", "App.kt"), "class App\n")
	topology, err = NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Java", "Kotlin"}) {
		t.Fatalf("Languages=%v, want Java and Kotlin source", topology.Languages)
	}
}

func TestScannerCoverageKeepsSharedContextOnStructuralModule(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/context\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "native", "tool.cpp"), "int tool;\n")
	writeTestFile(t, filepath.Join(root, "README.md"), "# Context\n")
	writeTestFile(t, filepath.Join(root, "public", "app.js"), "console.log('ok')\n")
	writeTestFile(t, filepath.Join(root, "deploy", "compose.yaml"), "services: {}\n")
	writeTestFile(t, filepath.Join(root, "schema.sql"), "select 1;\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range topology.Modules {
		if module.Coverage {
			for _, sourceRoot := range module.SourceRoots {
				if sourceRoot.Role != genericSourceRole {
					t.Fatalf("shared context attached to coverage module: %+v", module)
				}
			}
		}
	}
	assertUniqueSurfacedPaths(t, topology)
}

func TestScannerCppHobbyFixtureAddsOnlyUnclaimedCppCoverage(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "badger-cert", "fixtures", "cpp-gcc-hobby-app", "project"))
	if err != nil {
		t.Fatal(err)
	}
	toolPath := filepath.Join("tools", "notes_to_json.cpp")
	if language, ok := classifyGenericSourceCandidate(root, toolPath); !ok || language != "C++" {
		t.Fatalf("fixture tool candidate=(%q, %v), want C++ source", language, ok)
	}
	cppModules := mustDetectModules(t, NewCppDetector().Detect, root)
	ownership := newSemanticSourceOwnership(root, cppModules)
	if ownership.Owns(toolPath) {
		t.Fatal("specialized C++ ownership claimed fixture tool")
	}
	directCoverage, err := collectUnclaimedSource(root, ownership)
	if err != nil || coverageModuleContaining(directCoverage, toolPath) == nil {
		t.Fatalf("direct coverage=%+v err=%v, want fixture tool", directCoverage, err)
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	coverage := coverageModuleContaining(topology.Modules, toolPath)
	if coverage == nil {
		t.Fatalf("missing tools/notes_to_json.cpp coverage: %+v", topology.Modules)
	}
	if coverageModuleContaining(topology.Modules, "main.cpp") != nil || coverageModuleContaining(topology.Modules, filepath.Join("src", "app.cpp")) != nil {
		t.Fatal("coverage bypassed specialized C++ ownership")
	}
	if coverageModuleContaining(topology.Modules, filepath.Join("scripts", "compile.sh")) != nil {
		t.Fatal("shell control was recovered as language-source coverage")
	}
	assertUniqueSurfacedPaths(t, topology)
}

func TestCoveragePublicJSONAndSchemaAKeepStructuralContract(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/public-contract\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "native", "tool.cpp"), "int tool;\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topology.Structure != "Single Module" || len(topology.Modules) != 2 {
		t.Fatalf("structure=%q modules=%d, want Single Module with two topology groups", topology.Structure, len(topology.Modules))
	}
	coverage := coverageModuleContaining(topology.Modules, filepath.Join("native", "tool.cpp"))
	if coverage == nil || coverage.FileCount != 1 || coverage.TotalBytes != int64(len("int tool;\n")) || coverage.SourceRoots[0].FileCount != 1 {
		t.Fatalf("coverage ownership/counts are incoherent: %+v", coverage)
	}

	data, err := json.Marshal(topology)
	if err != nil {
		t.Fatal(err)
	}
	var public map[string]any
	if err := json.Unmarshal(data, &public); err != nil {
		t.Fatal(err)
	}
	modules, ok := public["modules"].([]any)
	if !ok || len(modules) != 2 || public["structure"] != "Single Module" {
		t.Fatalf("public topology shape=%s", data)
	}
	for _, item := range modules {
		module, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("invalid public module shape: %T", item)
		}
		for _, field := range []string{"name", "path", "file_count", "total_bytes", "heaviest", "top_files", "source_roots", "language"} {
			if _, exists := module[field]; !exists {
				t.Fatalf("public module missing %q: %s", field, data)
			}
		}
		if _, exists := module["coverage"]; exists {
			t.Fatalf("internal coverage marker serialized: %s", data)
		}
	}

	schemaA := protocol.NewFormatter().GenerateSchemaA(topology, "summarize")
	for _, want := range []string{"Structure: Single Module", "Pkg: native [1 files]", "tool.cpp"} {
		if !strings.Contains(schemaA, want) {
			t.Fatalf("Schema A missing %q:\n%s", want, schemaA)
		}
	}
	for _, forbidden := range []string{"[COVERAGE]", "Coverage:", "coverage_module"} {
		if strings.Contains(schemaA, forbidden) {
			t.Fatalf("Schema A added coverage structure %q:\n%s", forbidden, schemaA)
		}
	}
}

func coverageModuleContaining(modules []model.Module, path string) *model.Module {
	for idx := range modules {
		if modules[idx].Coverage && moduleHasPackageTopFile(modules[idx], path) {
			return &modules[idx]
		}
	}
	return nil
}

func assertUniqueSurfacedPaths(t *testing.T, topology *model.ProjectTopology) {
	t.Helper()
	var paths []string
	for _, module := range topology.Modules {
		for _, sourceRoot := range module.SourceRoots {
			for _, pkg := range sourceRoot.Packages {
				for _, file := range append(append([]model.FileSummary{}, pkg.TopFiles...), pkg.AuxFiles...) {
					paths = append(paths, normalizeTopologyFilePath(file.Path))
				}
			}
		}
	}
	sort.Strings(paths)
	for idx := 1; idx < len(paths); idx++ {
		if paths[idx] == paths[idx-1] {
			t.Fatalf("surfaced path %q is owned more than once", paths[idx])
		}
	}
}
