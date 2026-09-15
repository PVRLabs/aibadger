package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestCollectUnclaimedSourceGroupsLanguagesAliasesAndDirectories(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"native/main.cpp":          "int main() { return 0; }\n",
		"native/support.cc":        "void support() {}\n",
		"lib/worker.rb":            "puts 'work'\n",
		"php/index.php":            "<?php echo 'ok';\n",
		"src/main/kotlin/App.kt":   "class App\n",
		"src/main/kotlin/Util.kts": "class Util\n",
		"tools/tool.rs":            "fn main() {}\n",
		"ios/App.swift":            "struct App {}\n",
	}
	for path, contents := range files {
		writeTestFile(t, filepath.Join(root, path), contents)
	}
	for i := 0; i < maxGenericPackageFiles+2; i++ {
		writeTestFile(t, filepath.Join(root, "rust", fmt.Sprintf("tool-%02d.rs", i)), "fn tool() {}\n")
	}

	modules, _, err := NewGenericDetector().collectGenericAugmentation(root, semanticSourceOwnership{projectRoot: root})
	if err != nil {
		t.Fatalf("collectGenericAugmentation() error = %v", err)
	}
	wantLanguages := []string{"C++", "Kotlin", "PHP", "Ruby", "Rust", "Swift"}
	if got := coverageLanguages(modules); !reflect.DeepEqual(got, wantLanguages) {
		t.Fatalf("languages = %v, want %v", got, wantLanguages)
	}
	for _, module := range modules {
		if !module.Coverage || module.Path != "" || len(module.SourceRoots) != 1 || module.SourceRoots[0].Role != genericSourceRole {
			t.Fatalf("invalid coverage representation: %+v", module)
		}
		if module.FileCount != module.SourceRoots[0].FileCount || module.FileCount == 0 || module.TotalBytes == 0 {
			t.Fatalf("incoherent module rollup: %+v", module)
		}
		if module.Heaviest.Path != module.TopFiles[0].Path || module.Heaviest.Size != module.TopFiles[0].Size {
			t.Fatalf("incoherent module heaviest summary: %+v", module)
		}
	}
	cpp := findCoverageModule(modules, "C++")
	if cpp == nil || cpp.FileCount != 2 || findGenericPackage(*cpp, "native").FileCount != 2 {
		t.Fatalf("C++ coverage = %+v, want two alias files in literal native package", cpp)
	}
	if wantBytes := int64(len(files["native/main.cpp"]) + len(files["native/support.cc"])); cpp.TotalBytes != wantBytes {
		t.Fatalf("C++ TotalBytes = %d, want %d", cpp.TotalBytes, wantBytes)
	}
	kotlin := findCoverageModule(modules, "Kotlin")
	if kotlin == nil || kotlin.FileCount != 2 || findGenericPackage(*kotlin, filepath.Join("src", "main", "kotlin")).FileCount != 2 {
		t.Fatalf("Kotlin coverage = %+v, want .kt/.kts aliases in literal package", kotlin)
	}
	rust := findCoverageModule(modules, "Rust")
	if rust == nil || rust.FileCount != maxGenericPackageFiles+3 || len(rust.TopFiles) != maxGenericPackageFiles {
		t.Fatalf("Rust coverage = %+v, want coherent counts with bounded summaries", rust)
	}
}

func TestCollectUnclaimedSourceFiltersClaimedControlsAndResources(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/owned.go":                   "package main\n",
		"native/unclaimed.cpp":           "int native;\n",
		"migrations/application.py":      "print('application migration')\n",
		"README.md":                      "# docs\n",
		"notes.txt":                      "notes\n",
		"web/index.html":                 "<html></html>\n",
		"deploy/release.rb":              "puts 'release'\n",
		"scripts/build.rb":               "puts 'build'\n",
		"scripts/backup.py":              "print('backup')\n",
		"scripts/diagnose.py":            "print('diagnose')\n",
		"scripts/export.ts":              "export {}\n",
		"scripts/helpers/check.rb":       "puts 'check'\n",
		"scripts/health.rb":              "puts 'health'\n",
		"scripts/reporting/formatter.rb": "puts 'format'\n",
		"scripts/run-tests.ts":           "export {}\n",
		"scripts/provisioning/main.py":   "print('provision')\n",
		"scripts/deployments/task.rb":    "puts 'deploy'\n",
		"scripts/releases/publish.ts":    "export {}\n",
		"scripts/migrations/helper.py":   "print('migrate')\n",
		"config/schema.json":             "{}\n",
		"assets/logo.png":                "png\n",
		"blob.bin":                       "binary\n",
		"build.gradle.kts":               "plugins {}\n",
		"vite.config.ts":                 "export default {}\n",
	}
	for path, contents := range files {
		writeTestFile(t, filepath.Join(root, path), contents)
	}
	ownership := semanticSourceOwnership{projectRoot: root, claims: []semanticSourceClaim{{
		language: "Go", areas: []sourceClaimArea{{root: "src", recursive: true}},
	}}}

	modules, _, err := NewGenericDetector().collectGenericAugmentation(root, ownership)
	if err != nil {
		t.Fatalf("collectGenericAugmentation() error = %v", err)
	}
	if got, want := coveragePaths(modules), []string{"native/unclaimed.cpp", "migrations/application.py", "scripts/reporting/formatter.rb"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("coverage paths = %v, want %v", got, want)
	}
}

func TestCppCompanionHeaderIndexCachesOneDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "lib", "math")
	for _, name := range []string{"a.cpp", "a.hpp", "b.cpp", "b.HPP", "c.cpp", "c.hpp"} {
		writeTestFile(t, filepath.Join(dir, name), "source\n")
	}

	index := newCppCompanionHeaderIndex(root, semanticSourceOwnership{projectRoot: root}, 0)
	for _, test := range []struct {
		implementation string
		header         string
	}{
		{implementation: "a.cpp", header: "a.hpp"},
		{implementation: "b.cpp", header: "b.HPP"},
		{implementation: "c.cpp", header: "c.hpp"},
	} {
		header, ok := index.find(filepath.Join(dir, test.implementation))
		if !ok || header.Name != test.header {
			t.Errorf("find(%s) = (%+v, %v), want %s", test.implementation, header, ok, test.header)
		}
	}
	if len(index.byDirectory) != 1 {
		t.Fatalf("indexed %d directories for repeated lookups in one directory, want 1", len(index.byDirectory))
	}
}

func TestCppCompanionHeaderIndexCachesEmptyDirectoryResult(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "lib", "math")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := newCppCompanionHeaderIndex(root, semanticSourceOwnership{projectRoot: root}, 0)
	if _, ok := index.find(filepath.Join(dir, "math.cpp")); ok {
		t.Fatal("missing header unexpectedly matched")
	}
	writeTestFile(t, filepath.Join(dir, "math.hpp"), "int math();\n")
	if _, ok := index.find(filepath.Join(dir, "math.cpp")); ok {
		t.Fatal("cached empty directory result was re-enumerated")
	}
	if len(index.byDirectory) != 1 {
		t.Fatalf("indexed %d directories after repeated empty lookup, want cached result", len(index.byDirectory))
	}
}

func TestCollectUnclaimedSourceCppCompanionIndexHonorsDirectoryLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() {}\n")
	for _, name := range []string{"a.cpp", "a.hpp", "b.cpp", "b.hpp"} {
		writeTestFile(t, filepath.Join(root, "lib", "math", name), "source\n")
	}
	ownership := semanticSourceOwnership{projectRoot: root, claims: []semanticSourceClaim{{
		language: "C++",
		areas:    []sourceClaimArea{{root: ""}},
	}}}
	detector := NewGenericDetector()
	detector.maxFilesPerDir = 3

	modules, _, err := detector.collectGenericAugmentation(root, ownership)
	if err != nil {
		t.Fatal(err)
	}
	cpp := findCoverageModule(modules, "C++")
	if cpp == nil || cpp.FileCount != 3 {
		t.Fatalf("C++ coverage = %+v, want a.cpp/a.hpp/b.cpp only", cpp)
	}
	for _, want := range []string{"lib/math/a.cpp", "lib/math/a.hpp", "lib/math/b.cpp"} {
		if !moduleHasPackageTopFile(*cpp, want) {
			t.Errorf("bounded coverage missing %q", want)
		}
	}
	if moduleHasPackageTopFile(*cpp, "lib/math/b.hpp") {
		t.Fatalf("companion lookup bypassed per-directory limit: %+v", cpp)
	}
}

func TestCollectUnclaimedSourceBoundsAndOrderingAreDeterministic(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		writeTestFile(t, filepath.Join(root, "src", fmt.Sprintf("%02d.rb", i)), "puts 0\n")
	}
	writeTestFile(t, filepath.Join(root, "z", "last.php"), "<?php echo 1;\n")

	detector := NewGenericDetector()
	detector.maxFilesPerDir = 5
	detector.maxTotalFiles = 5
	ownership := semanticSourceOwnership{projectRoot: root}
	first, firstContext, err := detector.collectGenericAugmentation(root, ownership)
	if err != nil {
		t.Fatalf("first collection error = %v", err)
	}
	second, secondContext, err := detector.collectGenericAugmentation(root, ownership)
	if err != nil {
		t.Fatalf("second collection error = %v", err)
	}
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstContext, secondContext) {
		t.Fatalf("repeated collection differs:\nfirst=(%+v, %+v)\nsecond=(%+v, %+v)", first, firstContext, second, secondContext)
	}
	if len(first) != 1 || first[0].Language != "Ruby" || first[0].FileCount != 5 {
		t.Fatalf("bounded modules = %+v, want five deterministic Ruby files", first)
	}
	wantPaths := []string{"src/00.rb", "src/01.rb", "src/02.rb", "src/03.rb", "src/04.rb"}
	if got := coveragePaths(first); !reflect.DeepEqual(got, wantPaths) {
		t.Fatalf("bounded paths = %v, want %v", got, wantPaths)
	}
}

func TestCoverageMarkerIsInternalAndUsesGenericRanking(t *testing.T) {
	module := model.Module{Name: "Ruby Source", Language: "Ruby", Coverage: true}
	data, err := json.Marshal(module)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || !usesGenericRanking(&module) {
		t.Fatalf("coverage module was not recognized by Generic ranking: %s", data)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["Coverage"]; ok {
		t.Fatalf("internal Coverage marker serialized: %s", data)
	}
	if _, ok := decoded["coverage"]; ok {
		t.Fatalf("internal coverage marker serialized: %s", data)
	}
}

func coverageLanguages(modules []model.Module) []string {
	languages := make([]string, len(modules))
	for i := range modules {
		languages[i] = modules[i].Language
	}
	return languages
}

func findCoverageModule(modules []model.Module, language string) *model.Module {
	for i := range modules {
		if modules[i].Language == language {
			return &modules[i]
		}
	}
	return nil
}

func coveragePaths(modules []model.Module) []string {
	var paths []string
	for _, module := range modules {
		paths = append(paths, genericModulePaths(module)...)
	}
	return paths
}
