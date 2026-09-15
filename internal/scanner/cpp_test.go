package scanner

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestCppDetectorActivatesOnlyFromRootOrBoundedSrcImplementation(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  bool
	}{
		{"root cpp", []string{"main.cpp"}, true},
		{"src cc", []string{filepath.Join("src", "main.cc")}, true},
		{"src cxx", []string{filepath.Join("src", "lib", "parser.cxx")}, true},
		{"headers only", []string{filepath.Join("include", "parser.hpp")}, false},
		{"c only", []string{filepath.Join("src", "parser.c")}, false},
		{"cmake headers only", []string{"CMakeLists.txt", filepath.Join("include", "parser.hpp")}, false},
		{"nested cmake only", []string{filepath.Join("src", "CMakeLists.txt")}, false},
		{"test implementation", []string{filepath.Join("tests", "parser.cpp")}, false},
		{"include implementation", []string{filepath.Join("include", "parser.cpp")}, false},
		{"non conventional", []string{filepath.Join("native", "main.cpp")}, false},
		{"too deep", []string{filepath.Join("src", "a", "b", "c", "d", "e", "f", "g", "main.cpp")}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range tt.files {
				writeTestFile(t, filepath.Join(root, path), "content\n")
			}
			modules, err := NewCppDetector().Detect(root)
			if err != nil {
				t.Fatal(err)
			}
			if got := len(modules) == 1; got != tt.want {
				t.Fatalf("activated = %v, want %v; modules=%+v", got, tt.want, modules)
			}
		})
	}
}

func TestCppDetectorBuildsPhysicalConventionalTopologyAndRanking(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"CMakeLists.txt":                              "cmake\n",
		"main.cpp":                                    "int main() { return 0; }\n",
		filepath.Join("src", "main.cpp"):              "src main\n",
		filepath.Join("src", "parser.cpp"):            "parser implementation\n",
		filepath.Join("include", "parser.hpp"):        "parser header\n",
		filepath.Join("tests", "parser_test.cpp"):     "test\n",
		filepath.Join("src", "lib", "CMakeLists.txt"): "nested cmake\n",
	}
	for path, contents := range files {
		writeTestFile(t, filepath.Join(root, path), contents)
	}
	modules, err := NewCppDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 {
		t.Fatalf("modules = %+v, want one", modules)
	}
	module := modules[0]
	if module.Name != filepath.Base(root) || module.Path != "" || module.Language != "C++" {
		t.Fatalf("module identity = %+v", module)
	}
	if module.FileCount != len(files) {
		t.Fatalf("FileCount = %d, want %d", module.FileCount, len(files))
	}
	wantPrefix := []string{"CMakeLists.txt", "main.cpp", filepath.Join("src", "main.cpp")}
	for i, path := range wantPrefix {
		if module.TopFiles[i].Path != path {
			t.Fatalf("TopFiles prefix = %v, want %v", cppPaths(module.TopFiles), wantPrefix)
		}
	}
	for _, rootPath := range []string{"", "src", "include", "tests"} {
		if cppSourceRoot(module, rootPath) == nil {
			t.Fatalf("missing source root %q: %+v", rootPath, module.SourceRoots)
		}
	}
	if cppPackage(module, filepath.Join("src", "lib")) == nil || !hasTopFile(cppPackage(module, filepath.Join("src", "lib")).TopFiles, filepath.Join("src", "lib", "CMakeLists.txt")) {
		t.Fatalf("nested CMakeLists.txt not surfaced as ordinary package context: %+v", module)
	}
}

func TestCppHeaviestExcludesCMakeContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "CMakeLists.txt"), string(make([]byte, 4096)))
	writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() {}\n")
	modules, err := NewCppDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if modules[0].Heaviest.Path != "main.cpp" {
		t.Fatalf("module Heaviest = %+v, want physical C++ source", modules[0].Heaviest)
	}
	pkg := cppPackage(modules[0], "")
	if pkg == nil || pkg.Heaviest.Path != "main.cpp" {
		t.Fatalf("package Heaviest = %+v, want physical C++ source", pkg)
	}
}

func TestCppDetectorPrunesExcludedTreesAndHonorsBoundsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "src", "00-main.cpp"), "main\n")
	for _, dir := range []string{"build", "cmake-build-debug", "out", "vendor", "third-party", "generated"} {
		writeTestFile(t, filepath.Join(root, "src", dir, "ignored.cpp"), "ignored\n")
	}
	writeTestFile(t, filepath.Join(root, "src", "01.hpp"), "header\n")
	writeTestFile(t, filepath.Join(root, "src", "02.cpp"), "source\n")
	detector := NewCppDetector()
	detector.maxFilesPerDir = 2
	first, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated detection differs:\nfirst=%+v\nsecond=%+v", first, second)
	}
	paths := cppModulePhysicalPaths(first[0])
	if !reflect.DeepEqual(paths, []string{filepath.Join("src", "00-main.cpp"), filepath.Join("src", "01.hpp")}) {
		t.Fatalf("bounded paths = %v", paths)
	}

	total := NewCppDetector()
	total.maxEntries = 2 // root's src entry, then src/00-main.cpp
	modules, err := total.Detect(root)
	if err != nil || len(modules) != 1 || modules[0].FileCount != 1 {
		t.Fatalf("total-entry bounded modules=%+v err=%v", modules, err)
	}
}

func TestCppLogicalSameDirectoryPairsSpendOnePackageItem(t *testing.T) {
	var files []model.FileSummary
	for i := 0; i < 10; i++ {
		for _, ext := range []string{".cpp", ".hpp"} {
			name := fmt.Sprintf("unit%02d%s", i, ext)
			files = append(files, model.FileSummary{Name: name, Path: name, Size: int64(100 - i)})
		}
	}
	selected := selectCppPackageFiles(files, 10)
	if len(selected) != 20 {
		t.Fatalf("selected %d physical files, want 20", len(selected))
	}
	for _, file := range files {
		if !hasTopFile(selected, file.Path) {
			t.Fatalf("selected files missing %s: %v", file.Path, cppPaths(selected))
		}
	}
}

func TestCppLogicalPairsSupportExtensionsAndSameDirectoryPrecedence(t *testing.T) {
	for _, pair := range [][2]string{{".c", ".h"}, {".cc", ".hh"}, {".cpp", ".hpp"}, {".cxx", ".hxx"}} {
		files := []model.FileSummary{
			{Name: "parser" + pair[0], Path: filepath.Join("src", "parser"+pair[0]), Size: 2},
			{Name: "parser" + pair[1], Path: filepath.Join("src", "parser"+pair[1]), Size: 1},
		}
		if selected := selectCppPackageFiles(files, 1); len(selected) != 2 {
			t.Fatalf("extensions %v selected %v, want both physical files", pair, cppPaths(selected))
		}
	}

	overlap := []model.FileSummary{
		{Name: "parser.cpp", Path: filepath.Join("src", "parser.cpp"), Size: 3},
		{Name: "parser.hpp", Path: filepath.Join("src", "parser.hpp"), Size: 2},
		{Name: "parser.hpp", Path: filepath.Join("include", "parser.hpp"), Size: 1},
	}
	selected := selectCppModuleFiles(overlap, 1)
	if len(selected) != 2 || !hasTopFile(selected, filepath.Join("src", "parser.hpp")) || hasTopFile(selected, filepath.Join("include", "parser.hpp")) {
		t.Fatalf("same-directory pair did not take precedence: %v", cppPaths(selected))
	}
}

func TestCppLogicalMirroredAndAmbiguousPairs(t *testing.T) {
	files := []model.FileSummary{
		{Name: "parser.cpp", Path: filepath.Join("src", "lib", "parser.cpp"), Size: 30},
		{Name: "parser.hpp", Path: filepath.Join("include", "lib", "parser.hpp"), Size: 20},
		{Name: "platform.cpp", Path: filepath.Join("src", "platform.cpp"), Size: 18},
		{Name: "platform.cc", Path: filepath.Join("src", "platform.cc"), Size: 17},
		{Name: "platform.hpp", Path: filepath.Join("include", "platform.hpp"), Size: 16},
		{Name: "platform.h", Path: filepath.Join("include", "platform.h"), Size: 15},
		{Name: "other.hpp", Path: filepath.Join("include", "other.hpp"), Size: 14},
	}
	selected := selectCppModuleFiles(files, 1)
	if len(selected) != 2 || !hasTopFile(selected, filepath.Join("src", "lib", "parser.cpp")) || !hasTopFile(selected, filepath.Join("include", "lib", "parser.hpp")) {
		t.Fatalf("mirrored pair selection = %v", cppPaths(selected))
	}
	selected = selectCppModuleFiles(files[2:], 2)
	if len(selected) != 2 {
		t.Fatalf("ambiguous candidates paired unexpectedly: %v", cppPaths(selected))
	}
}

func TestCppMirroredMemberMustSurviveItsOwnPackageCap(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "include", "parser.hpp"), "header\n")
	writeTestFile(t, filepath.Join(root, "src", "parser.cpp"), "x\n")
	for _, name := range []string{"alpha.cpp", "beta.cpp", "gamma.cpp"} {
		writeTestFile(t, filepath.Join(root, "src", name), "a much larger implementation\n")
	}
	modules, err := NewCppDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	src := cppPackage(modules[0], "src")
	include := cppPackage(modules[0], "include")
	if src == nil || include == nil || hasTopFile(src.TopFiles, filepath.Join("src", "parser.cpp")) {
		t.Fatalf("mirrored implementation unexpectedly bypassed src package cap: src=%+v", src)
	}
	if !hasTopFile(include.TopFiles, filepath.Join("include", "parser.hpp")) || !hasTopFile(modules[0].TopFiles, filepath.Join("include", "parser.hpp")) {
		t.Fatalf("surviving mirrored header was not treated as an ordinary module item: module=%+v", modules[0])
	}
}

func TestCppFinalizationPreservesPairAwarePhysicalFiles(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 10; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("unit%02d.cpp", i)), "implementation\n")
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("unit%02d.hpp", i)), "header\n")
	}
	modules, err := NewCppDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	topology := model.ProjectTopology{Modules: modules}
	NewScanner(root).finalizeTopology(&topology)
	pkg := cppPackage(topology.Modules[0], "")
	if pkg == nil || len(pkg.TopFiles) != 20 || len(topology.Modules[0].TopFiles) != 20 {
		t.Fatalf("finalization recapped physical pair members: module=%d package=%+v", len(topology.Modules[0].TopFiles), pkg)
	}
	seen := make(map[string]bool)
	for _, file := range pkg.TopFiles {
		if seen[file.Path] {
			t.Fatalf("duplicate physical path %q", file.Path)
		}
		seen[file.Path] = true
	}
}

func TestCppFinalizationLeavesAttachedContextOnRoleSpecificSemantics(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() {}\n")
	modules, err := NewCppDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	docs := model.Package{Name: "docs", Path: "docs", FileCount: 6, Heaviest: model.HeaviestFile{Name: "stale", Path: "stale"}}
	for i := 0; i < 6; i++ {
		name := fmt.Sprintf("doc%d.md", i)
		docs.TopFiles = append(docs.TopFiles, model.FileSummary{Name: name, Path: filepath.Join("docs", name), Size: int64(i + 1)})
	}
	resources := model.Package{Name: "resources", Path: "resources", FileCount: 4, Heaviest: model.HeaviestFile{Name: "stale", Path: "stale"}}
	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("data%d.json", i)
		resources.TopFiles = append(resources.TopFiles, model.FileSummary{Name: name, Path: filepath.Join("resources", name), Size: int64(i + 1)})
	}
	modules[0].SourceRoots = append(modules[0].SourceRoots,
		model.SourceRoot{Path: "docs", Role: "Documentation", FileCount: 6, Packages: []model.Package{docs}},
		model.SourceRoot{Path: "resources", Role: "Resources", FileCount: 4, Packages: []model.Package{resources}},
	)
	modules[0].FileCount += 10
	topology := model.ProjectTopology{Modules: modules}
	NewScanner(root).finalizeTopology(&topology)
	docs = *cppPackage(topology.Modules[0], "docs")
	resources = *cppPackage(topology.Modules[0], "resources")
	if len(docs.TopFiles) != 5 || docs.Heaviest.Path != filepath.Join("docs", "doc5.md") {
		t.Fatalf("Documentation package used C++ rebuild semantics: %+v", docs)
	}
	if len(resources.TopFiles) != maxPackageTopFiles || resources.Heaviest.Path != filepath.Join("resources", "data3.json") {
		t.Fatalf("Resources package used C++ rebuild semantics: %+v", resources)
	}
}

func TestScannerCppModuleOverviewBudgetsOwnedUnitsSeparatelyFromSharedContext(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < maxRootPackageTopFiles; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("unit%02d.cpp", i)), "implementation\n")
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("unit%02d.hpp", i)), "header\n")
	}
	writeTestFile(t, filepath.Join(root, "README.md"), "project overview\n")
	for i := 0; i < 3; i++ {
		writeTestFile(t, filepath.Join(root, "docs", fmt.Sprintf("guide%02d.md", i)), "guide\n")
	}
	writeTestFile(t, filepath.Join(root, "schema", "model.sql"), "create table model(id int);\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Modules) != 1 || !isFirstClassCppModule(&topology.Modules[0]) {
		t.Fatalf("modules=%+v, want one first-class C++ module", topology.Modules)
	}
	module := topology.Modules[0]
	for i := 0; i < maxRootPackageTopFiles; i++ {
		for _, ext := range []string{".cpp", ".hpp"} {
			path := fmt.Sprintf("unit%02d%s", i, ext)
			if !hasTopFile(module.TopFiles, path) {
				t.Fatalf("shared context evicted C++ pair member %q: %v", path, cppPaths(module.TopFiles))
			}
		}
	}
	for _, path := range []string{"README.md", filepath.Join("docs", "guide00.md"), filepath.Join("schema", "model.sql")} {
		if !hasTopFile(module.TopFiles, path) {
			t.Fatalf("shared context %q missing from independently budgeted overview: %v", path, cppPaths(module.TopFiles))
		}
	}
}

func TestCppFinalizationPreservesSamePathSourceRootRoles(t *testing.T) {
	for _, duplicateREADME := range []bool{false, true} {
		for _, reverseRoots := range []bool{false, true} {
			t.Run(fmt.Sprintf("duplicate=%t/reverse=%t", duplicateREADME, reverseRoots), func(t *testing.T) {
				main := model.FileSummary{Name: "main.cpp", Path: "main.cpp", Size: 20}
				readme := model.FileSummary{Name: "README.md", Path: "README.md", Size: 30}
				code := model.SourceRoot{Role: "Main Source", FileCount: 1, Packages: []model.Package{{FileCount: 1, TopFiles: []model.FileSummary{main}, Heaviest: heaviestFromSummary(main)}}}
				docs := model.SourceRoot{Role: "Documentation", FileCount: 1, Packages: []model.Package{{FileCount: 1, TopFiles: []model.FileSummary{readme}}}}
				module := model.Module{Name: "cpp", Language: "C++", FileCount: 2, TotalBytes: 50, Heaviest: heaviestFromSummary(main)}
				if duplicateREADME {
					code.Packages[0].TopFiles = append(code.Packages[0].TopFiles, readme)
					code.Packages[0].FileCount++
					code.FileCount++
					module.FileCount++
					module.TotalBytes += readme.Size
				}
				module.SourceRoots = []model.SourceRoot{code, docs}
				if reverseRoots {
					module.SourceRoots = []model.SourceRoot{docs, code}
				}
				topology := model.ProjectTopology{Modules: []model.Module{module}}
				for pass := 0; pass < 2; pass++ {
					deduplicateTopologyFiles(&topology)
					sortTopology(&topology)
					got := &topology.Modules[0]
					if len(got.SourceRoots) != 2 || got.FileCount != 2 || got.TotalBytes != 50 {
						t.Fatalf("pass %d: ownership counts = %+v", pass, got)
					}
					for _, want := range []struct{ role, path string }{{"Main Source", "main.cpp"}, {"Documentation", "README.md"}} {
						sr := findSourceRootInModule(got, "", want.role)
						if sr == nil || sr.FileCount != 1 || len(sr.Packages) != 1 {
							t.Fatalf("pass %d: root %s = %+v", pass, want.role, sr)
						}
						pkg := sr.Packages[0]
						if pkg.FileCount != 1 || len(pkg.TopFiles) != 1 || pkg.TopFiles[0].Path != want.path || pkg.Heaviest.Path != want.path {
							t.Fatalf("pass %d: package in %s = %+v", pass, want.role, pkg)
						}
					}
				}
			})
		}
	}
}

func TestCppDetectionIgnoresCreationOrder(t *testing.T) {
	paths := []string{
		filepath.Join("src", "main.cpp"),
		filepath.Join("src", "parser.cpp"),
		filepath.Join("include", "parser.hpp"),
		filepath.Join("tests", "parser.cpp"),
	}
	scan := func(reverse bool) model.Module {
		root := t.TempDir()
		for i := range paths {
			index := i
			if reverse {
				index = len(paths) - 1 - i
			}
			writeTestFile(t, filepath.Join(root, paths[index]), paths[index]+"\n")
		}
		modules, err := NewCppDetector().Detect(root)
		if err != nil {
			t.Fatal(err)
		}
		modules[0].Name = ""
		return modules[0]
	}
	if first, second := scan(false), scan(true); !reflect.DeepEqual(first, second) {
		t.Fatalf("creation order changed topology:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestScannerIntegratesCppWithMixedLanguageWeighting(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/mixed\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "src", "main.cpp"), "int main() {}\n")
	writeTestFile(t, filepath.Join(root, "src", "parser.cc"), "void parse() {}\n")
	writeTestFile(t, filepath.Join(root, "tests", "parser_test.cxx"), "void test() {}\n")
	writeTestFile(t, filepath.Join(root, "include", "parser.hxx"), "void parse();\n")
	writeTestFile(t, filepath.Join(root, "src", "compat.c"), "void compat() {}\n")
	writeTestFile(t, filepath.Join(root, "CMakeLists.txt"), "project(mixed)\n")

	first, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	first.ScanTime, second.ScanTime = 0, 0
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated scans differ\nfirst: %+v\nsecond: %+v", first, second)
	}
	if first.PrimaryLanguage != "C++" || !reflect.DeepEqual(first.Languages, []string{"C++", "Go"}) {
		t.Fatalf("primary=%q languages=%v, want additive source-based C++ and Go", first.PrimaryLanguage, first.Languages)
	}
	cppModules := 0
	for i := range first.Modules {
		if isFirstClassCppModule(&first.Modules[i]) {
			cppModules++
		}
		if first.Modules[i].Language == "Generic" {
			t.Fatalf("specialized mixed scan included Generic duplicate: %+v", first.Modules[i])
		}
	}
	if cppModules != 1 || len(first.Modules) != 2 {
		t.Fatalf("modules=%+v, want one C++ and one Go module", first.Modules)
	}
}

func TestScannerCppWeightUsesOnlyBoundedAcceptedImplementations(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/bounded\n")
	for i := 0; i < 3; i++ {
		writeTestFile(t, filepath.Join(root, fmt.Sprintf("go%02d.go", i)), "package bounded\n")
	}
	for i := 0; i < 12; i++ {
		writeTestFile(t, filepath.Join(root, "src", fmt.Sprintf("%02d.cpp", i)), "void f() {}\n")
	}
	scanner := NewScanner(root)
	scanner.MaxFilesPerDirectory = 2
	topology, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topology.PrimaryLanguage != "Go" || !reflect.DeepEqual(topology.Languages, []string{"C++", "Go"}) {
		t.Fatalf("primary=%q languages=%v, want Go from 3 sources over 2 bounded C++ sources", topology.PrimaryLanguage, topology.Languages)
	}
}

func TestScannerDoesNotClaimCppOutsideConventionalBoundary(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/owned\n")
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	for i := 0; i < 100; i++ {
		writeTestFile(t, filepath.Join(root, "native", fmt.Sprintf("area-%03d", i), "deep", "main.cpp"), "int main() {}\n")
	}
	first, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	first.ScanTime, second.ScanTime = 0, 0
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("large out-of-bound scan is not deterministic\nfirst=%+v\nsecond=%+v", first, second)
	}
	if !reflect.DeepEqual(first.Languages, []string{"C++", "Go"}) || len(first.Modules) != 2 || first.Structure != "Single Module" {
		t.Fatalf("out-of-bound C++ was not added as non-structural coverage: languages=%v structure=%q modules=%+v", first.Languages, first.Structure, first.Modules)
	}
}

func TestCppDetectorLanguageCountRequiresActivation(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "tests", "only.cc"), "void test() {}\n")
	detector := NewCppDetector()
	modules, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 0 || detector.languageSourceCount != 0 {
		t.Fatalf("modules=%+v count=%d, want no activation weight", modules, detector.languageSourceCount)
	}
}

func TestCppDetectorLanguageCountIncludesOnlyAcceptedCppImplementations(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join("src", "main.cpp"),
		filepath.Join("src", "parser.cc"),
		filepath.Join("tests", "parser_test.cxx"),
		filepath.Join("src", "compat.c"),
		filepath.Join("include", "parser.hh"),
		"CMakeLists.txt",
	} {
		writeTestFile(t, filepath.Join(root, path), "content\n")
	}
	detector := NewCppDetector()
	modules, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 || detector.languageSourceCount != 3 {
		t.Fatalf("modules=%+v count=%d, want exactly 3 accepted C++ implementations", modules, detector.languageSourceCount)
	}
}

func cppSourceRoot(module model.Module, path string) *model.SourceRoot {
	for i := range module.SourceRoots {
		if module.SourceRoots[i].Path == path {
			return &module.SourceRoots[i]
		}
	}
	return nil
}

func cppPackage(module model.Module, path string) *model.Package {
	for i := range module.SourceRoots {
		for j := range module.SourceRoots[i].Packages {
			if module.SourceRoots[i].Packages[j].Path == path {
				return &module.SourceRoots[i].Packages[j]
			}
		}
	}
	return nil
}

func cppPaths(files []model.FileSummary) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func cppModulePhysicalPaths(module model.Module) []string {
	var paths []string
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			for _, file := range pkg.TopFiles {
				paths = append(paths, file.Path)
			}
		}
	}
	sort.Strings(paths)
	return paths
}
