package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestCSharpDetectorCreatesModulesFromMarkersWithoutParsingXML(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Root.csproj"), "not xml")
	writeTestFile(t, filepath.Join(root, "Alternate.csproj"), "")
	writeTestFile(t, filepath.Join(root, "src", "Feature", "Service.cs"), "namespace Ignored.Semantics; class Service {}")
	writeTestFile(t, filepath.Join(root, "tests", "ServiceTests.cs"), "class ServiceTests {}")
	writeTestFile(t, filepath.Join(root, "apps", "Child", "Child.csproj"), "<broken")
	writeTestFile(t, filepath.Join(root, "apps", "Child", "Program.cs"), "class Program {}")

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want 2: %+v", len(modules), modules)
	}
	if modules[0].Path != "" || modules[0].Name != filepath.Base(root) || modules[0].Language != "C#" {
		t.Fatalf("root module = %+v", modules[0])
	}
	if modules[1].Path != filepath.Join("apps", "Child") || modules[1].Name != "Child" {
		t.Fatalf("child module = %+v", modules[1])
	}
	rootPaths := csharpModuleFilePaths(modules[0])
	for _, want := range []string{"Alternate.csproj", "Root.csproj", filepath.Join("src", "Feature", "Service.cs"), filepath.Join("tests", "ServiceTests.cs")} {
		if !containsString(rootPaths, want) {
			t.Errorf("root paths %v missing %q", rootPaths, want)
		}
	}
	if containsString(rootPaths, filepath.Join("apps", "Child", "Program.cs")) {
		t.Fatalf("parent crossed nested project boundary: %v", rootPaths)
	}
	if pkg := csharpPackage(modules[0], filepath.Join("src", "Feature")); pkg == nil || pkg.Name != "Feature" {
		t.Fatalf("literal directory package missing or namespace-derived: %+v", pkg)
	}
}

func TestCSharpDetectorKeepsMarkerOnlyModuleAndColocatedSolutionContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "src", "Empty", "Empty.csproj"), "{")
	writeTestFile(t, filepath.Join(root, "src", "Empty", "Workspace.sln"), "Project(phantom)")

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 1 || modules[0].FileCount != 2 || modules[0].TotalBytes == 0 || len(modules[0].SourceRoots) != 1 {
		t.Fatalf("marker-only module = %+v", modules)
	}
	paths := csharpModuleFilePaths(modules[0])
	if !containsString(paths, filepath.Join("src", "Empty", "Empty.csproj")) || !containsString(paths, filepath.Join("src", "Empty", "Workspace.sln")) {
		t.Fatalf("overview paths = %v", paths)
	}
}

func TestCSharpDetectorSurfacesRootAppSettingsVariantsOnly(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	for _, name := range []string{"appsettings.json", "appsettings.Development.json", "appsettings.Production.json"} {
		writeTestFile(t, filepath.Join(root, name), "{\"placeholder\":true}")
	}
	writeTestFile(t, filepath.Join(root, "settings.json"), "{\"unrelated\":true}")
	writeTestFile(t, filepath.Join(root, "appsettings.Local.Debug.json"), "{\"unrelated\":true}")
	writeTestFile(t, filepath.Join(root, "src", "appsettings.json"), "{\"deep\":true}")

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := csharpModuleFilePaths(modules[0])
	for _, want := range []string{"appsettings.json", "appsettings.Development.json", "appsettings.Production.json"} {
		if !containsString(paths, want) {
			t.Errorf("root appsettings path %q missing from %v", want, paths)
		}
	}
	for _, unwanted := range []string{"settings.json", "appsettings.Local.Debug.json", filepath.Join("src", "appsettings.json")} {
		if containsString(paths, unwanted) {
			t.Errorf("unrelated/deep JSON unexpectedly surfaced: %v", paths)
		}
	}
}

func TestCSharpDetectorKeepsBoundedApplicationCompanionContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "Pages", "Index.razor"), "<h1>Home</h1>")
	writeTestFile(t, filepath.Join(root, "Views", "Home", "Index.cshtml"), "<h1>Home</h1>")
	writeTestFile(t, filepath.Join(root, "UI", "MainWindow.xaml"), "<Window />")
	writeTestFile(t, filepath.Join(root, "wwwroot", "css", "site.css"), "body {}")
	writeTestFile(t, filepath.Join(root, "wwwroot", "images", "logo.svg"), "<svg />")
	writeTestFile(t, filepath.Join(root, "Properties", "launchSettings.json"), "{}")
	writeTestFile(t, filepath.Join(root, "Properties", "unrelated.json"), "{}")
	writeTestFile(t, filepath.Join(root, "assets", "outside-wwwroot.css"), "body {}")

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := csharpModuleFilePaths(modules[0])
	for _, want := range []string{
		filepath.Join("Pages", "Index.razor"),
		filepath.Join("Views", "Home", "Index.cshtml"),
		filepath.Join("UI", "MainWindow.xaml"),
		filepath.Join("wwwroot", "css", "site.css"),
		filepath.Join("Properties", "launchSettings.json"),
	} {
		if !containsString(paths, want) {
			t.Errorf("companion path %q missing from %v", want, paths)
		}
	}
	if !csharpModuleHasPackageAuxFile(modules[0], filepath.Join("wwwroot", "images", "logo.svg")) {
		t.Fatalf("wwwroot asset was not retained as auxiliary context: %+v", modules[0].SourceRoots)
	}
	for _, unwanted := range []string{
		filepath.Join("Properties", "unrelated.json"),
		filepath.Join("assets", "outside-wwwroot.css"),
	} {
		if containsString(paths, unwanted) {
			t.Errorf("unrelated context unexpectedly surfaced: %v", paths)
		}
	}
}

func TestCSharpDetectorRanksSourceBeforeCompanionContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "UI", "Service.cs"), "class Service {}")
	writeTestFile(t, filepath.Join(root, "UI", "Page.razor"), strings.Repeat("razor", 100))
	writeTestFile(t, filepath.Join(root, "UI", "View.cshtml"), strings.Repeat("view", 100))
	writeTestFile(t, filepath.Join(root, "UI", "Window.xaml"), strings.Repeat("xaml", 100))

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	pkg := csharpPackage(modules[0], "UI")
	if pkg == nil || len(pkg.TopFiles) != maxPackageTopFiles {
		t.Fatalf("UI package summary = %+v", pkg)
	}
	if !containsFilePath(pkg.TopFiles, filepath.Join("UI", "Service.cs")) {
		t.Fatalf("large companions displaced real C# source: %+v", pkg.TopFiles)
	}
	if pkg.TopFiles[0].Path != filepath.Join("UI", "Service.cs") {
		t.Fatalf("package top-file ranking = %+v, want C# source first", pkg.TopFiles)
	}
}

func TestCSharpDetectorKeepsCompanionOnlyPackages(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "Pages", "Index.razor"), "<h1>Home</h1>")
	writeTestFile(t, filepath.Join(root, "UI", "Window.xaml"), "<Window />")

	detector := NewCSharpDetector()
	modules, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Join("Pages", "Index.razor"), filepath.Join("UI", "Window.xaml")} {
		if !containsString(csharpModuleFilePaths(modules[0]), path) {
			t.Errorf("companion-only package lost %q", path)
		}
	}
	if detector.languageSourceCount != 0 {
		t.Fatal("companion-only module contributed C# language weight")
	}
}

func TestCSharpDetectorRoutesWWWRootAssetsToAuxiliaryFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}")
	writeTestFile(t, filepath.Join(root, "wwwroot", "site.css"), "body {}")
	writeTestFile(t, filepath.Join(root, "wwwroot", "app.js"), "console.log('ok')")
	writeTestFile(t, filepath.Join(root, "wwwroot", "logo.png"), "png data")
	writeTestFile(t, filepath.Join(root, "wwwroot", "font.woff2"), "font data")

	detector := NewCSharpDetector()
	modules, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	pkg := csharpPackage(modules[0], filepath.Join("wwwroot"))
	if pkg == nil || pkg.FileCount != 4 || len(pkg.TopFiles) != 2 {
		t.Fatalf("wwwroot package = %+v, want four counted files and two primary text files", pkg)
	}
	for _, path := range []string{filepath.Join("wwwroot", "site.css"), filepath.Join("wwwroot", "app.js")} {
		if !containsFilePath(pkg.TopFiles, path) {
			t.Errorf("text companion %q missing from TopFiles: %+v", path, pkg.TopFiles)
		}
	}
	for _, path := range []string{filepath.Join("wwwroot", "logo.png"), filepath.Join("wwwroot", "font.woff2")} {
		if !containsFilePath(pkg.AuxFiles, path) {
			t.Errorf("asset %q missing from AuxFiles: %+v", path, pkg.AuxFiles)
		}
		if containsFilePath(pkg.TopFiles, path) {
			t.Errorf("asset %q competes in TopFiles: %+v", path, pkg.TopFiles)
		}
	}
	if modules[0].FileCount != 6 || modules[0].TotalBytes == 0 || detector.languageSourceCount != 1 {
		t.Fatalf("file/byte/language accounting = module %+v, C# weight %d", modules[0], detector.languageSourceCount)
	}
	if pkg.Heaviest.Path == filepath.Join("wwwroot", "logo.png") || pkg.Heaviest.Path == filepath.Join("wwwroot", "font.woff2") {
		t.Fatalf("asset became package heaviest primary context: %+v", pkg.Heaviest)
	}

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	var scannedCSharp *model.Module
	for idx := range topology.Modules {
		if isFirstClassCSharpModule(&topology.Modules[idx]) {
			scannedCSharp = &topology.Modules[idx]
			break
		}
	}
	if scannedCSharp == nil {
		t.Fatalf("first-class C# module missing from scanned topology: %+v", topology.Modules)
	}
	scannedPkg := csharpPackage(*scannedCSharp, filepath.Join("wwwroot"))
	if scannedPkg == nil || !containsFilePath(scannedPkg.AuxFiles, filepath.Join("wwwroot", "logo.png")) || containsFilePath(scannedPkg.TopFiles, filepath.Join("wwwroot", "logo.png")) {
		t.Fatalf("finalized topology lost auxiliary asset classification: %+v", scannedCSharp.SourceRoots)
	}
	if scannedCSharp.Heaviest.Path == filepath.Join("wwwroot", "logo.png") || scannedCSharp.Heaviest.Path == filepath.Join("wwwroot", "font.woff2") {
		t.Fatalf("asset became module heaviest primary context: %+v", scannedCSharp.Heaviest)
	}
}

func TestCSharpDetectorCompanionContextHonorsExclusionsAndOmissions(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	for _, dir := range []string{"bin", "obj", ".vs", "packages", "coverage", "TestResults"} {
		writeTestFile(t, filepath.Join(root, dir, "Generated.razor"), "generated")
		writeTestFile(t, filepath.Join(root, dir, "wwwroot", "generated.js"), "generated")
	}
	writeTestFile(t, filepath.Join(root, "wwwroot", "native.dll"), "binary")

	modules, err := NewCSharpDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := csharpModuleFilePaths(modules[0])
	if len(paths) != 1 || paths[0] != "App.csproj" {
		t.Fatalf("excluded or omitted companion context surfaced: %v", paths)
	}
}

func TestScannerCSharpCompanionContextHasZeroLanguageWeight(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "app", "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "app", "Page.razor"), "@page")
	writeTestFile(t, filepath.Join(root, "app", "View.cshtml"), "view")
	writeTestFile(t, filepath.Join(root, "app", "Window.xaml"), "<Window />")
	writeTestFile(t, filepath.Join(root, "app", "wwwroot", "site.css"), "body {}")
	writeTestFile(t, filepath.Join(root, "app", "Properties", "launchSettings.json"), "{}")
	writeTestFile(t, filepath.Join(root, "tool", "go.mod"), "module example.com/tool")
	writeTestFile(t, filepath.Join(root, "tool", "main.go"), "package main")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topology.PrimaryLanguage != "Go" || !reflect.DeepEqual(topology.Languages, []string{"C#", "Go"}) {
		t.Fatalf("primary=%q languages=%v, want Go primary with C# structural presence", topology.PrimaryLanguage, topology.Languages)
	}
}

func TestScannerCSharpWeightUsesOnlyBoundedAcceptedSources(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "app", "App.csproj"), "")
	for idx := 0; idx < 20; idx++ {
		writeTestFile(t, filepath.Join(root, "app", fmt.Sprintf("source%02d.cs", idx)), "class Source {}")
	}
	writeTestFile(t, filepath.Join(root, "tool", "go.mod"), "module example.com/tool")
	writeTestFile(t, filepath.Join(root, "tool", "a.go"), "package tool")
	writeTestFile(t, filepath.Join(root, "tool", "b.go"), "package tool")

	scanner := NewScanner(root)
	scanner.MaxFilesPerDirectory = 2
	topology, err := scanner.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topology.PrimaryLanguage != "Go" {
		t.Fatalf("PrimaryLanguage = %q, want Go from 2 accepted Go files versus 1 bounded C# file", topology.PrimaryLanguage)
	}
	for _, module := range topology.Modules {
		if isFirstClassCSharpModule(&module) {
			if module.FileCount != 2 {
				t.Fatalf("bounded C# FileCount = %d, want marker plus one accepted source", module.FileCount)
			}
		}
	}
}

func TestCSharpDetectorPrunesGeneratedAndOutputDirectories(t *testing.T) {
	for _, dir := range []string{"bin", "obj", ".vs", "packages", "TestResults", "coverage", "dist", "build", "node_modules", "vendor"} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, filepath.Join(root, "App.csproj"), "")
			writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}")
			writeTestFile(t, filepath.Join(root, dir, "Generated.csproj"), "")
			writeTestFile(t, filepath.Join(root, dir, "Generated.cs"), "class Generated {}")
			modules, err := NewCSharpDetector().Detect(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(modules) != 1 {
				t.Fatalf("generated marker created module: %+v", modules)
			}
			if paths := csharpModuleFilePaths(modules[0]); containsString(paths, filepath.Join(dir, "Generated.cs")) {
				t.Fatalf("generated file surfaced: %v", paths)
			}
		})
	}
}

func TestCSharpDetectorRankingAndLimitsAreDeterministic(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "x")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "p")
	writeTestFile(t, filepath.Join(root, "Startup.cs"), "s")
	writeTestFile(t, filepath.Join(root, "Huge.cs"), strings.Repeat("h", 1000))
	writeTestFile(t, filepath.Join(root, "src", "a.cs"), "a")
	writeTestFile(t, filepath.Join(root, "src", "b.cs"), "b")
	writeTestFile(t, filepath.Join(root, "src", "c.cs"), "c")
	writeTestFile(t, filepath.Join(root, "one", "two", "three", "four", "five", "six", "AtLimit.cs"), "x")
	writeTestFile(t, filepath.Join(root, "one", "two", "three", "four", "five", "six", "seven", "TooDeep.cs"), "x")

	detector := NewCSharpDetector()
	first, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated detection differs\nfirst: %+v\nsecond: %+v", first, second)
	}
	gotPrefix := []string{first[0].TopFiles[0].Name, first[0].TopFiles[1].Name, first[0].TopFiles[2].Name}
	if want := []string{"App.csproj", "Program.cs", "Startup.cs"}; !reflect.DeepEqual(gotPrefix, want) {
		t.Fatalf("top ranking = %v, want %v", gotPrefix, want)
	}
	paths := csharpModuleFilePaths(first[0])
	if !containsString(paths, filepath.Join("one", "two", "three", "four", "five", "six", "AtLimit.cs")) || containsString(paths, filepath.Join("one", "two", "three", "four", "five", "six", "seven", "TooDeep.cs")) {
		t.Fatalf("depth-bound paths = %v", paths)
	}
	if pkg := csharpPackage(first[0], "src"); pkg == nil || pkg.FileCount != 3 || len(pkg.TopFiles) != maxPackageTopFiles {
		t.Fatalf("representative-file cap package = %+v", pkg)
	}
}

func TestCSharpDetectorUsesConfiguredPerDirectoryEntryLimit(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	for _, name := range []string{"a.cs", "b.cs", "c.cs"} {
		writeTestFile(t, filepath.Join(root, name), name)
	}
	detector := NewCSharpDetector()
	detector.maxFilesPerDir = 2
	modules, err := detector.Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := csharpModuleFilePaths(modules[0])
	if !containsString(paths, "a.cs") || containsString(paths, "b.cs") || containsString(paths, "c.cs") {
		t.Fatalf("per-directory bounded paths = %v", paths)
	}
}

func TestCSharpDetectorHonorsDetectorWideRemainingEntryBudget(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "App.csproj")
	writeTestFile(t, marker, "")
	writeTestFile(t, filepath.Join(root, "a.cs"), "a")
	writeTestFile(t, filepath.Join(root, "b.cs"), "b")
	remaining := 2
	module := NewCSharpDetector().analyzeModule(root, root, []string{marker}, &remaining)
	paths := csharpModuleFilePaths(module)
	if remaining != 0 || !containsString(paths, "a.cs") || containsString(paths, "b.cs") {
		t.Fatalf("remaining=%d bounded paths=%v", remaining, paths)
	}
}

func TestCSharpDetectorBoundsWideSparseDirectoryTraversal(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "App.csproj")
	writeTestFile(t, marker, "")
	for outer := 0; outer < 30; outer++ {
		for inner := 0; inner < 30; inner++ {
			if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("d%02d", outer), fmt.Sprintf("empty%02d", inner)), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	writeTestFile(t, filepath.Join(root, "z", "Program.cs"), "class Program {}")

	remaining := 100
	module := NewCSharpDetector().analyzeModule(root, root, []string{marker}, &remaining)
	if remaining != 0 {
		t.Fatalf("remaining = %d, want traversal budget exhausted by directory entries", remaining)
	}
	if paths := csharpModuleFilePaths(module); containsString(paths, filepath.Join("z", "Program.cs")) {
		t.Fatalf("wide sparse traversal escaped total entry budget: %v", paths)
	}
}

func TestCSharpDetectorTotalBudgetLeavesLaterProjectMarkerVisible(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "a", "A.csproj"), "")
	writeTestFile(t, filepath.Join(root, "z", "Z.csproj"), "")
	for dir := 0; dir < 2; dir++ {
		for file := 0; file < 8; file++ {
			writeTestFile(t, filepath.Join(root, "a", fmt.Sprintf("d%02d", dir), fmt.Sprintf("f%03d.cs", file)), "x")
		}
	}
	writeTestFile(t, filepath.Join(root, "z", "Program.cs"), "class Program {}")

	// Small budgets exercise project fairness without a large filesystem fixture.
	modules, err := NewCSharpDetector().detectWithBudgets(root, 12, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) != 2 || !containsString(csharpModuleFilePaths(modules[1]), filepath.Join("z", "Z.csproj")) {
		t.Fatalf("later marker missing after total budget: %+v", modules)
	}
	if !containsString(csharpModuleFilePaths(modules[1]), filepath.Join("z", "Program.cs")) {
		t.Fatalf("fixed per-project cap starved later representative source: %+v", modules[1])
	}
}

func TestScannerIntegratesCSharpWeightingAndSolutionBoundaries(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "App.sln"), "fake/Phantom.csproj")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}")
	writeTestFile(t, filepath.Join(root, "Feature.cs"), "class Feature {}")
	writeTestFile(t, filepath.Join(root, "tool", "go.mod"), "module example.com/tool")
	writeTestFile(t, filepath.Join(root, "tool", "main.go"), "package main")

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
	if first.PrimaryLanguage != "C#" || !reflect.DeepEqual(first.Languages, []string{"C#", "Go"}) || len(first.Modules) != 2 {
		t.Fatalf("integrated topology primary=%q languages=%v modules=%+v", first.PrimaryLanguage, first.Languages, first.Modules)
	}
	for _, module := range first.Modules {
		if strings.Contains(strings.ToLower(module.Name), "phantom") {
			t.Fatalf("solution introduced phantom module: %+v", module)
		}
	}
}

func TestScannerDoesNotActivateCSharpWithoutBoundedProjectMarker(t *testing.T) {
	t.Run("solution and source only", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "Only.sln"), "Phantom.csproj")
		writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}")
		topology, err := NewScanner(root).Scan()
		if err != nil {
			t.Fatal(err)
		}
		if !containsString(topology.Languages, "C#") || len(topology.Modules) != 1 {
			t.Fatalf("generic fallback topology languages=%v modules=%+v", topology.Languages, topology.Modules)
		}
		if isFirstClassCSharpModule(&topology.Modules[0]) {
			t.Fatalf("markerless source activated first-class C# module: %+v", topology.Modules[0])
		}
	})
	t.Run("marker deeper than discovery bound", func(t *testing.T) {
		root := t.TempDir()
		for idx := 0; idx < 100; idx++ {
			deep := filepath.Join(fmt.Sprintf("area-%03d", idx), "two", "three", "four", "five")
			writeTestFile(t, filepath.Join(root, deep, "Deep.csproj"), "")
			writeTestFile(t, filepath.Join(root, deep, "Program.cs"), "class Program {}")
		}
		modules, err := NewCSharpDetector().Detect(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(modules) != 0 {
			t.Fatalf("deep modules = %+v", modules)
		}
	})
}

func TestCSharpProjectIsHighPriorityIdentityManifest(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "App.csproj")
	writeTestFile(t, path, "")
	priority, ok := ReviewPathPriority(root, path)
	if !ok || priority != 1 {
		t.Fatalf("ReviewPathPriority() = (%d, %v), want (1, true)", priority, ok)
	}
}

func csharpModuleFilePaths(module model.Module) []string {
	seen := map[string]bool{}
	for _, root := range module.SourceRoots {
		for _, pkg := range root.Packages {
			for _, file := range pkg.TopFiles {
				seen[file.Path] = true
			}
			for _, file := range pkg.AuxFiles {
				seen[file.Path] = true
			}
		}
	}
	paths := make([]string, 0, len(seen))
	for path := range seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func csharpPackage(module model.Module, path string) *model.Package {
	for rootIdx := range module.SourceRoots {
		for pkgIdx := range module.SourceRoots[rootIdx].Packages {
			pkg := &module.SourceRoots[rootIdx].Packages[pkgIdx]
			if pkg.Path == path {
				return pkg
			}
		}
	}
	return nil
}

func containsFilePath(files []model.FileSummary, want string) bool {
	for _, file := range files {
		if file.Path == want {
			return true
		}
	}
	return false
}

func csharpModuleHasPackageAuxFile(module model.Module, path string) bool {
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			if containsFilePath(pkg.AuxFiles, path) {
				return true
			}
		}
	}
	return false
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
