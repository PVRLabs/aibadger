package scanner

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestSemanticSourceOwnershipIgnoresReportingAndScanCaps(t *testing.T) {
	tests := []struct {
		name      string
		prepare   func(*testing.T, string) []model.Module
		claimed   []string
		unclaimed []string
	}{
		{
			name: "Go",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/capped\n")
				for idx := 0; idx < maxRootPackageTopFiles+2; idx++ {
					writeTestFile(t, filepath.Join(root, fmt.Sprintf("source%02d.go", idx)), "package capped\n")
				}
				return mustDetectModules(t, NewGoDetector().Detect, root)
			},
			claimed:   []string{"source00.go", fmt.Sprintf("source%02d.go", maxRootPackageTopFiles+1)},
			unclaimed: []string{filepath.Join("native", "main.cpp"), filepath.Join("tools", "helper.go")},
		},
		{
			name: "Java",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "pom.xml"), "<project/>")
				for idx := 0; idx < maxPackageTopFiles+2; idx++ {
					writeTestFile(t, filepath.Join(root, "src", "main", "java", "app", fmt.Sprintf("Source%02d.java", idx)), "class Source {}\n")
				}
				return mustDetectModules(t, NewJavaDetector().Detect, root)
			},
			claimed:   []string{filepath.Join("src", "main", "java", "app", "Source00.java"), filepath.Join("src", "main", "java", "app", fmt.Sprintf("Source%02d.java", maxPackageTopFiles+1))},
			unclaimed: []string{filepath.Join("src", "main", "kotlin", "App.kt"), filepath.Join("tools", "Helper.java")},
		},
		{
			name: "Python",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "pyproject.toml"), "[project]\nname='capped'\n")
				for idx := 0; idx < maxPackageTopFiles+2; idx++ {
					writeTestFile(t, filepath.Join(root, "src", "app", fmt.Sprintf("source%02d.py", idx)), "pass\n")
				}
				return mustDetectModules(t, NewPythonDetector().Detect, root)
			},
			claimed:   []string{filepath.Join("src", "app", "source00.py"), filepath.Join("src", "app", fmt.Sprintf("source%02d.py", maxPackageTopFiles+1))},
			unclaimed: []string{filepath.Join("lib", "worker.rb")},
		},
		{
			name: "Node",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "package.json"), `{"name":"capped","main":"src/a.js"}`)
				writeTestFile(t, filepath.Join(root, "src", "a.js"), "export const a = 1\n")
				writeTestFile(t, filepath.Join(root, "src", "b.ts"), "export const b = 1\n")
				detector := NewNodeDetector()
				detector.maxFilesPerDir = 1
				return mustDetectModules(t, detector.Detect, root)
			},
			claimed:   []string{filepath.Join("src", "a.js"), filepath.Join("src", "b.ts")},
			unclaimed: []string{filepath.Join("native", "main.cpp")},
		},
		{
			name: "CSharp",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "App.csproj"), "")
				writeTestFile(t, filepath.Join(root, "a.cs"), "class A {}\n")
				writeTestFile(t, filepath.Join(root, "b.cs"), "class B {}\n")
				writeTestFile(t, filepath.Join(root, "one", "two", "three", "four", "five", "six", "seven", "Deep.cs"), "class Deep {}\n")
				detector := NewCSharpDetector()
				detector.maxFilesPerDir = 2
				return mustDetectModules(t, detector.Detect, root)
			},
			claimed:   []string{"a.cs", "b.cs", filepath.Join("one", "two", "three", "four", "five", "six", "seven", "Deep.cs")},
			unclaimed: []string{filepath.Join("native", "main.cpp")},
		},
		{
			name: "Cpp",
			prepare: func(t *testing.T, root string) []model.Module {
				writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() {}\n")
				writeTestFile(t, filepath.Join(root, "src", "a.cpp"), "void a() {}\n")
				writeTestFile(t, filepath.Join(root, "src", "b.cpp"), "void b() {}\n")
				writeTestFile(t, filepath.Join(root, "include", "app.h"), "void app();\n")
				writeTestFile(t, filepath.Join(root, "include", "detail", "bounded.hpp"), "void bounded();\n")
				detector := NewCppDetector()
				detector.maxFilesPerDir = 1
				return mustDetectModules(t, detector.Detect, root)
			},
			claimed:   []string{"main.cpp", filepath.Join("src", "a.cpp"), filepath.Join("src", "b.cpp"), filepath.Join("include", "app.h"), filepath.Join("include", "detail", "bounded.hpp")},
			unclaimed: []string{filepath.Join("tools", "helper.cpp")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range append(append([]string{}, tt.claimed...), tt.unclaimed...) {
				if filepath.Ext(path) != ".csproj" {
					writeTestFile(t, filepath.Join(root, path), "source\n")
				}
			}
			ownership := newSemanticSourceOwnership(root, tt.prepare(t, root))
			for _, path := range tt.claimed {
				if !ownership.Owns(path) || !ownership.Owns(filepath.Join(root, path)) {
					t.Errorf("%s should be claimed using relative and absolute paths", path)
				}
			}
			for _, path := range tt.unclaimed {
				if ownership.Owns(path) {
					t.Errorf("%s should remain unclaimed", path)
				}
			}
			if ownership.Owns(filepath.Join("..", "outside.go")) {
				t.Error("path outside the project should not be claimed")
			}
		})
	}
}

func TestSemanticSourceOwnershipCppHobbyBoundary(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		"main.cpp",
		filepath.Join("src", "app.cpp"),
		filepath.Join("include", "lantern", "app.h"),
		filepath.Join("tests", "app_test.cpp"),
		filepath.Join("tools", "notes_to_json.cpp"),
	} {
		writeTestFile(t, filepath.Join(root, path), "source\n")
	}
	modules := mustDetectModules(t, NewCppDetector().Detect, root)
	ownership := newSemanticSourceOwnership(root, modules)
	for _, path := range []string{"main.cpp", filepath.Join("src", "app.cpp"), filepath.Join("include", "lantern", "app.h"), filepath.Join("tests", "app_test.cpp")} {
		if !ownership.Owns(path) {
			t.Errorf("hobby application path %s should be claimed", path)
		}
	}
	if ownership.Owns(filepath.Join("tools", "notes_to_json.cpp")) {
		t.Error("tools/notes_to_json.cpp should remain unclaimed")
	}
}

func TestSemanticSourceOwnershipHonorsExcludedAndNestedAreas(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}\n")
	writeTestFile(t, filepath.Join(root, "obj", "Generated.cs"), "class Generated {}\n")
	writeTestFile(t, filepath.Join(root, "apps", "Child", "Child.csproj"), "")
	writeTestFile(t, filepath.Join(root, "apps", "Child", "Program.cs"), "class Child {}\n")

	modules := mustDetectModules(t, NewCSharpDetector().Detect, root)
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want root and nested C# modules", len(modules))
	}
	outerOnly := newSemanticSourceOwnership(root, modules[:1])
	if outerOnly.Owns(filepath.Join("apps", "Child", "Program.cs")) {
		t.Error("outer C# claim crossed a nested project boundary")
	}
	if outerOnly.Owns(filepath.Join("obj", "Generated.cs")) {
		t.Error("C# claim included generated output")
	}
	if !newSemanticSourceOwnership(root, modules).Owns(filepath.Join("apps", "Child", "Program.cs")) {
		t.Error("successful nested C# module should claim its own source")
	}

	cppRoot := t.TempDir()
	writeTestFile(t, filepath.Join(cppRoot, "main.cpp"), "int main() {}\n")
	writeTestFile(t, filepath.Join(cppRoot, "src", "generated", "Generated.cpp"), "void generated() {}\n")
	writeTestFile(t, filepath.Join(cppRoot, "src", "cmake-build-debug", "Generated.cpp"), "void generated() {}\n")
	cppOwnership := newSemanticSourceOwnership(cppRoot, mustDetectModules(t, NewCppDetector().Detect, cppRoot))
	for _, path := range []string{filepath.Join("src", "generated", "Generated.cpp"), filepath.Join("src", "cmake-build-debug", "Generated.cpp")} {
		if cppOwnership.Owns(path) {
			t.Errorf("C++ claim included excluded path %s", path)
		}
	}
}

func TestClassifyGenericSourceCandidateFiltersControlsAndOps(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		"build.gradle.kts",
		"settings.gradle.kts",
		"vite.config.ts",
		"next.config.ts",
		"setup.py",
		filepath.Join(".github", "workflows", "release.ts"),
		filepath.Join(".github", "workflows", "jobs", "release.ts"),
		filepath.Join("deploy", "db", "migrate.rb"),
		filepath.Join("scripts", "release.ts"),
		filepath.Join("scripts", "migrate.py"),
		filepath.Join("scripts", "nested", "build.rb"),
		filepath.Join("scripts", "release", "helper.py"),
		filepath.Join("scripts", "build", "main.ts"),
		filepath.Join("scripts", "deploy", "task.rb"),
		filepath.Join("scripts", "backup.py"),
		filepath.Join("scripts", "health.rb"),
		filepath.Join("scripts", "export.ts"),
		filepath.Join("scripts", "diagnose.py"),
		filepath.Join("scripts", "run-tests.ts"),
		filepath.Join("scripts", "provisioning", "main.py"),
		filepath.Join("scripts", "deployments", "task.rb"),
		filepath.Join("scripts", "releases", "publish.ts"),
		filepath.Join("scripts", "migrations", "helper.py"),
		filepath.Join("Scripts", "Release", "helper.py"),
		filepath.Join("build", "generated.rb"),
		filepath.Join("obj", "GeneratedAssemblyInfo.cs"),
		filepath.Join("nested", "OBJ", "Generated.cs"),
		filepath.Join(".azure", "token.kt"),
	}
	for _, path := range paths {
		writeTestFile(t, filepath.Join(root, path), "control\n")
	}
	writeTestFile(t, filepath.Join(root, "src", "main", "kotlin", "Foo.kt"), "class Foo\n")
	writeTestFile(t, filepath.Join(root, "native", "main.cpp"), "int main() {}\n")
	writeTestFile(t, filepath.Join(root, "lib", "worker.rb"), "puts 'work'\n")
	writeTestFile(t, filepath.Join(root, "php", "index.php"), "<?php\n")
	writeTestFile(t, filepath.Join(root, "scripts", "data_analysis.py"), "print('analysis')\n")
	writeTestFile(t, filepath.Join(root, "scripts", "helper.py"), "print('helper')\n")
	writeTestFile(t, filepath.Join(root, "scripts", "reporting", "formatter.rb"), "puts 'format'\n")
	writeTestFile(t, filepath.Join(root, "scripts", "analysis", "helper.py"), "print('helper')\n")
	writeTestFile(t, filepath.Join(root, "Scripts", "Analysis", "helper.py"), "print('helper')\n")
	writeTestFile(t, filepath.Join(root, "tools", "helper.py"), "print('helper')\n")
	writeTestFile(t, filepath.Join(root, "migrations", "helper.py"), "print('application migration')\n")
	writeTestFile(t, filepath.Join(root, "obj", "parser.cpp"), "int parser;\n")
	writeTestFile(t, filepath.Join(root, "obj", "tool.rb"), "puts 'tool'\n")
	writeTestFile(t, filepath.Join(root, "src", "obj", "helper.py"), "print('helper')\n")

	for _, path := range paths {
		if language, ok := classifyGenericSourceCandidate(root, path); ok {
			t.Errorf("control/generated path %s classified as %s", path, language)
		}
	}
	for path, want := range map[string]string{
		filepath.Join("src", "main", "kotlin", "Foo.kt"):      "Kotlin",
		filepath.Join("native", "main.cpp"):                   "C++",
		filepath.Join("lib", "worker.rb"):                     "Ruby",
		filepath.Join("php", "index.php"):                     "PHP",
		filepath.Join("scripts", "data_analysis.py"):          "Python",
		filepath.Join("scripts", "helper.py"):                 "Python",
		filepath.Join("scripts", "reporting", "formatter.rb"): "Ruby",
		filepath.Join("scripts", "analysis", "helper.py"):     "Python",
		filepath.Join("Scripts", "Analysis", "helper.py"):     "Python",
		filepath.Join("tools", "helper.py"):                   "Python",
		filepath.Join("migrations", "helper.py"):              "Python",
		filepath.Join("obj", "parser.cpp"):                    "C++",
		filepath.Join("obj", "tool.rb"):                       "Ruby",
		filepath.Join("src", "obj", "helper.py"):              "Python",
	} {
		if got, ok := classifyGenericSourceCandidate(root, path); !ok || got != want {
			t.Errorf("candidate %s = %q, %v; want %q, true", path, got, ok, want)
		}
	}
}

func TestSpecializedOwnershipLeavesForeignGenericLanguagesUnclaimed(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/mixed\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	foreign := map[string]string{
		filepath.Join("native", "main.cpp"): "C++",
		filepath.Join("lib", "worker.rb"):   "Ruby",
		filepath.Join("php", "index.php"):   "PHP",
		filepath.Join("kotlin", "App.kt"):   "Kotlin",
	}
	for path := range foreign {
		writeTestFile(t, filepath.Join(root, path), "source\n")
	}
	ownership := newSemanticSourceOwnership(root, mustDetectModules(t, NewGoDetector().Detect, root))
	for path, wantLanguage := range foreign {
		if ownership.Owns(path) {
			t.Errorf("Go module claimed foreign source %s", path)
		}
		if language, ok := classifyGenericSourceCandidate(root, path); !ok || language != wantLanguage {
			t.Errorf("foreign source %s = %q, %v; want %q, true", path, language, ok, wantLanguage)
		}
	}
}

func TestGradleSettingsFilesDoNotActivateJavaDetector(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "settings.gradle"), "rootProject.name = 'app'\n")
	writeTestFile(t, filepath.Join(root, "settings.gradle.kts"), "rootProject.name = \"app\"\n")
	writeTestFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "class App {}\n")
	modules, err := NewJavaDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 0 {
		t.Fatalf("Gradle settings files activated Java modules: %+v", modules)
	}
}

func TestJavaKotlinDSLOwnershipAndCandidateDistinction(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "build.gradle.kts"), "plugins { java }\n")
	writeTestFile(t, filepath.Join(root, "settings.gradle.kts"), "rootProject.name = \"app\"\n")
	writeTestFile(t, filepath.Join(root, "src", "main", "java", "App.java"), "class App {}\n")
	writeTestFile(t, filepath.Join(root, "src", "main", "kotlin", "App.kt"), "class App\n")

	modules := mustDetectModules(t, NewJavaDetector().Detect, root)
	ownership := newSemanticSourceOwnership(root, modules)
	if !ownership.Owns(filepath.Join("src", "main", "java", "App.java")) {
		t.Error("Java source should be specialized-claimed")
	}
	if ownership.Owns(filepath.Join("src", "main", "kotlin", "App.kt")) {
		t.Error("Java module should not claim Kotlin application source")
	}
	for _, path := range []string{"build.gradle.kts", "settings.gradle.kts"} {
		if language, ok := classifyGenericSourceCandidate(root, path); ok {
			t.Errorf("%s classified as augmentation source %s", path, language)
		}
	}
	if language, ok := classifyGenericSourceCandidate(root, filepath.Join("src", "main", "kotlin", "App.kt")); !ok || language != "Kotlin" {
		t.Fatalf("application Kotlin candidate = %q, %v; want Kotlin, true", language, ok)
	}
}

func mustDetectModules(t *testing.T, detect func(string) ([]model.Module, error), root string) []model.Module {
	t.Helper()
	modules, err := detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) == 0 {
		t.Fatal("detector did not return a successful module")
	}
	return modules
}
