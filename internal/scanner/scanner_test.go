package scanner

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestFinalizeTopology(t *testing.T) {
	s := NewScanner("")

	tests := []struct {
		name              string
		modules           []model.Module
		expectedPrimary   string
		expectedLanguages []string
	}{
		{
			name: "Single language project",
			modules: []model.Module{
				{Language: "Java", FileCount: 10},
				{Language: "Java", FileCount: 5},
			},
			expectedPrimary:   "Java",
			expectedLanguages: []string{"Java"},
		},
		{
			name: "Mixed language project",
			modules: []model.Module{
				{Language: "Java", FileCount: 10},
				{Language: "Go", FileCount: 20},
			},
			expectedPrimary:   "Go",
			expectedLanguages: []string{"Go", "Java"},
		},
		{
			name: "Mixed language tie",
			modules: []model.Module{
				{Language: "Java", FileCount: 10},
				{Language: "Go", FileCount: 10},
			},
			expectedPrimary:   "Go",
			expectedLanguages: []string{"Go", "Java"},
		},
		{
			name:              "Unknown project",
			modules:           []model.Module{},
			expectedPrimary:   "Unknown",
			expectedLanguages: []string{"Unknown"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topology := &model.ProjectTopology{
				Modules: tt.modules,
			}
			s.finalizeTopology(topology)

			if topology.PrimaryLanguage != tt.expectedPrimary {
				t.Errorf("Got PrimaryLanguage %s, expected %s", topology.PrimaryLanguage, tt.expectedPrimary)
			}
			if !reflect.DeepEqual(topology.Languages, tt.expectedLanguages) {
				t.Errorf("Got Languages %v, expected %v", topology.Languages, tt.expectedLanguages)
			}
		})
	}
}

func TestFinalizeTopologyWithLanguageWeightsAppliesWeightsOncePerLanguage(t *testing.T) {
	s := NewScanner("")
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{Name: "go-a", Path: "go-a", Language: "Go", FileCount: 100},
			{Name: "go-b", Path: "go-b", Language: "Go", FileCount: 100},
			{Name: "py", Path: "py", Language: "Python", FileCount: 1},
		},
	}

	s.finalizeTopologyWithLanguageWeights(topology, map[string]int64{
		"Go":     1,
		"Python": 2,
	})

	if topology.PrimaryLanguage != "Python" {
		t.Fatalf("PrimaryLanguage = %q, want Python from one weighted count per language", topology.PrimaryLanguage)
	}
}

func TestSourceLanguageWeightsCountActualSourceFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "a.go"), "package store\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "b.go"), "package store\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "schema.sql"), "create table orders(id int);\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "logo.png"), "png")

	modules := []model.Module{
		{
			Name:     "example.com/app",
			Path:     "",
			Language: "Go",
			SourceRoots: []model.SourceRoot{
				{
					Path: "internal",
					Role: "Internal Source",
					Packages: []model.Package{
						{
							Path:      filepath.Join("internal", "store"),
							FileCount: 4,
							TopFiles: []model.FileSummary{
								{Name: "a.go", Path: filepath.Join("internal", "store", "a.go")},
								{Name: "schema.sql", Path: filepath.Join("internal", "store", "schema.sql")},
							},
							AuxFiles: []model.FileSummary{
								{Name: "logo.png", Path: filepath.Join("internal", "store", "logo.png"), Kind: model.FileKindAsset},
							},
						},
					},
				},
				{
					Path: "deploy",
					Role: "Ops/Deploy",
					Packages: []model.Package{
						{Path: "deploy", FileCount: 1},
					},
				},
			},
		},
	}

	weights := sourceLanguageWeightsFromModules(modules, tmpDir)
	if weights["Go"] != 2 {
		t.Fatalf("Go source weight = %d, want actual .go file count 2", weights["Go"])
	}
}

func TestFindModuleByCandidateRequiresIdentityMatch(t *testing.T) {
	modules := []model.Module{
		{Name: "go-root", Path: "", Language: "Go"},
		{Name: "js-root", Path: "", Language: "JavaScript"},
	}

	matched := findModuleByCandidate(modules, topologyFileCandidate{
		modulePath: "",
		moduleName: "js-root",
		moduleLang: "JavaScript",
	})
	if matched == nil || matched.Name != "js-root" {
		t.Fatalf("matched module = %+v, want JavaScript root", matched)
	}

	mismatched := findModuleByCandidate(modules, topologyFileCandidate{
		modulePath: "",
		moduleName: "renamed",
		moduleLang: "JavaScript",
	})
	if mismatched != nil {
		t.Fatalf("mismatched candidate resolved to %+v, want nil", mismatched)
	}

	legacy := findModuleByCandidate(modules, topologyFileCandidate{modulePath: ""})
	if legacy == nil || legacy.Name != "go-root" {
		t.Fatalf("legacy candidate resolved to %+v, want path fallback", legacy)
	}
}

func TestFinalizeTopologySortsNestedOutput(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:      "web",
				Path:      "web",
				Language:  "Java",
				FileCount: 1,
				SourceRoots: []model.SourceRoot{
					{
						Path:      "web/src/test/java",
						FileCount: 2,
						Packages: []model.Package{
							{Path: "web/src/test/java/com/example/z", FileCount: 1},
							{Path: "web/src/test/java/com/example/a", FileCount: 1},
						},
					},
					{
						Path:      "web/src/main/java",
						FileCount: 3,
						Packages: []model.Package{
							{
								Path:      "web/src/main/java/com/example/service",
								FileCount: 2,
								TopFiles: []model.FileSummary{
									{Path: "web/src/main/java/com/example/service/B.java", Size: 10},
									{Path: "web/src/main/java/com/example/service/A.java", Size: 10},
								},
							},
							{Path: "web/src/main/java/com/example", FileCount: 1},
						},
					},
				},
			},
			{
				Name:      "api",
				Path:      "api",
				Language:  "Java",
				FileCount: 1,
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	modulePaths := []string{topology.Modules[0].Path, topology.Modules[1].Path}
	if !reflect.DeepEqual(modulePaths, []string{"api", "web"}) {
		t.Fatalf("module paths = %v, want [api web]", modulePaths)
	}

	webModule := topology.Modules[1]
	sourceRootPaths := []string{webModule.SourceRoots[0].Path, webModule.SourceRoots[1].Path}
	if !reflect.DeepEqual(sourceRootPaths, []string{"web/src/main/java", "web/src/test/java"}) {
		t.Fatalf("source root paths = %v, want [web/src/main/java web/src/test/java]", sourceRootPaths)
	}

	mainPackages := webModule.SourceRoots[0].Packages
	packagePaths := []string{mainPackages[0].Path, mainPackages[1].Path}
	if !reflect.DeepEqual(packagePaths, []string{"web/src/main/java/com/example", "web/src/main/java/com/example/service"}) {
		t.Fatalf("package paths = %v, want [web/src/main/java/com/example web/src/main/java/com/example/service]", packagePaths)
	}

	topFilePaths := []string{mainPackages[1].TopFiles[0].Path, mainPackages[1].TopFiles[1].Path}
	if !reflect.DeepEqual(topFilePaths, []string{
		"web/src/main/java/com/example/service/A.java",
		"web/src/main/java/com/example/service/B.java",
	}) {
		t.Fatalf("top file paths = %v, want lexical tie order", topFilePaths)
	}
}

func TestFinalizeTopologyDetectsStackAndStructure(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), `<project>
<packaging>pom</packaging>
<modules><module>api</module></modules>
</project>`)

	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{Language: "Java", FileCount: 1},
			{Language: "Java", FileCount: 1},
		},
	}

	NewScanner(tmpDir).finalizeTopology(topology)

	if topology.PrimaryLanguage != "Java" {
		t.Fatalf("PrimaryLanguage = %q, want Java", topology.PrimaryLanguage)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Java"}) {
		t.Fatalf("Languages = %v, want [Java]", topology.Languages)
	}
	if topology.Structure != "Multi-Module" {
		t.Fatalf("Structure = %q, want Multi-Module", topology.Structure)
	}
	if !reflect.DeepEqual(topology.Stack, []string{"Maven"}) {
		t.Fatalf("Stack = %v, want [Maven]", topology.Stack)
	}
}

func TestFinalizeTopologyDeduplicatesPackageAndModuleFilesByPriority(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:     "app",
				Path:     "",
				Language: "Generic",
				SourceRoots: []model.SourceRoot{
					{
						Path:      "",
						FileCount: 2,
						Packages: []model.Package{
							{
								Name:      "pkg-a",
								Path:      "pkg-a",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "index.html", Path: filepath.Join("shared", "index.html"), Size: 10, Kind: model.FileKindSource},
								},
							},
							{
								Name:      "pkg-b",
								Path:      "pkg-b",
								FileCount: 1,
								AuxFiles: []model.FileSummary{
									{Name: "index.html", Path: filepath.Join("shared", "index.html"), Size: 99, Kind: model.FileKindAsset},
								},
							},
						},
					},
				},
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	module := topology.Modules[0]
	pkgA := findPackage(module, "pkg-a")
	if pkgA == nil {
		t.Fatalf("missing package pkg-a after finalization: %+v", module.SourceRoots)
	}
	if len(pkgA.TopFiles) != 1 || pkgA.TopFiles[0].Path != filepath.Join("shared", "index.html") {
		t.Fatalf("pkg-a TopFiles = %+v, want winning shared/index.html once", pkgA.TopFiles)
	}
	if pkgA.Heaviest.Path != filepath.Join("shared", "index.html") {
		t.Fatalf("pkg-a Heaviest.Path = %q, want shared/index.html", pkgA.Heaviest.Path)
	}

	if findPackage(module, "pkg-b") != nil {
		t.Fatalf("pkg-b should be pruned after losing all duplicate ownership: %+v", module.SourceRoots)
	}

	if len(module.TopFiles) != 1 {
		t.Fatalf("module.TopFiles = %+v, want single deduplicated file", module.TopFiles)
	}
	if module.TopFiles[0].Path != filepath.Join("shared", "index.html") {
		t.Fatalf("module.TopFiles[0].Path = %q, want shared/index.html", module.TopFiles[0].Path)
	}
	if len(module.AuxFiles) != 0 {
		t.Fatalf("module.AuxFiles = %+v, want duplicate loser removed", module.AuxFiles)
	}
	if module.Heaviest.Path != filepath.Join("shared", "index.html") {
		t.Fatalf("module.Heaviest.Path = %q, want shared/index.html", module.Heaviest.Path)
	}
}

func TestFinalizeTopologyDeduplicatesTiedCandidatesDeterministically(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:       "app",
				Path:       "",
				Language:   "Generic",
				FileCount:  2,
				TotalBytes: 128,
				SourceRoots: []model.SourceRoot{
					{
						Path:      "",
						FileCount: 2,
						Packages: []model.Package{
							{
								Name:      "pkg-z",
								Path:      "pkg-z",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "shared.go", Path: filepath.Join("shared", "shared.go"), Size: 64, Kind: model.FileKindSource},
								},
							},
							{
								Name:      "pkg-a",
								Path:      "pkg-a",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "shared.go", Path: filepath.Join("shared", "shared.go"), Size: 64, Kind: model.FileKindSource},
								},
							},
						},
					},
				},
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	module := topology.Modules[0]
	pkgA := findPackage(module, "pkg-a")
	if pkgA == nil || len(pkgA.TopFiles) != 1 {
		t.Fatalf("pkg-a TopFiles = %+v, want deterministic winner retained", pkgA)
	}
	if pkgA.FileCount != 1 {
		t.Fatalf("pkg-a FileCount = %d, want 1", pkgA.FileCount)
	}
	if findPackage(module, "pkg-z") != nil {
		t.Fatalf("pkg-z should be pruned after losing all duplicate ownership: %+v", module.SourceRoots)
	}
	if module.SourceRoots[0].FileCount != 1 {
		t.Fatalf("source root FileCount = %d, want 1", module.SourceRoots[0].FileCount)
	}
	if module.FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want 1", module.FileCount)
	}
	if module.TotalBytes != 64 {
		t.Fatalf("module.TotalBytes = %d, want 64", module.TotalBytes)
	}
	if len(module.TopFiles) != 1 || module.TopFiles[0].Path != filepath.Join("shared", "shared.go") {
		t.Fatalf("module.TopFiles = %+v, want one shared/shared.go entry", module.TopFiles)
	}
}

func TestFinalizeTopologyPrefersMoreSpecificPackageOwnerForDuplicateSourceFile(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:       "app",
				Path:       "",
				Language:   "TypeScript",
				FileCount:  4,
				TotalBytes: 4,
				SourceRoots: []model.SourceRoot{
					{
						Path:      "",
						FileCount: 3,
						Packages: []model.Package{
							{
								Name:      "root",
								Path:      "",
								FileCount: 3,
								TopFiles: []model.FileSummary{
									{Name: "package.json", Path: "package.json", Size: 1},
									{Name: "tsconfig.json", Path: "tsconfig.json", Size: 1},
									{Name: "main.ts", Path: filepath.Join("src", "main.ts"), Size: 1},
								},
							},
						},
					},
					{
						Path:      "src",
						FileCount: 1,
						Packages: []model.Package{
							{
								Name:      "src",
								Path:      "src",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "main.ts", Path: filepath.Join("src", "main.ts"), Size: 1},
								},
							},
						},
					},
				},
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	module := topology.Modules[0]
	rootPkg := findPackage(module, "")
	if rootPkg == nil {
		t.Fatalf("missing root package after finalization: %+v", module.SourceRoots)
	}
	if hasTopFile(rootPkg.TopFiles, filepath.Join("src", "main.ts")) {
		t.Fatalf("rootPkg.TopFiles = %+v, should not keep src/main.ts over the src package owner", rootPkg.TopFiles)
	}

	srcPkg := findPackage(module, "src")
	if srcPkg == nil {
		t.Fatalf("missing src package after finalization: %+v", module.SourceRoots)
	}
	if !hasTopFile(srcPkg.TopFiles, filepath.Join("src", "main.ts")) {
		t.Fatalf("srcPkg.TopFiles = %+v, want src/main.ts retained in the more specific package", srcPkg.TopFiles)
	}
}

func TestFinalizeTopologyPrunesEmptyPackagesAndSourceRootsAfterDedup(t *testing.T) {
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:       "app",
				Path:       "",
				Language:   "TypeScript",
				FileCount:  1,
				TotalBytes: 1,
				SourceRoots: []model.SourceRoot{
					{
						Path:      "",
						FileCount: 1,
						Packages: []model.Package{
							{
								Name:      "root",
								Path:      "",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "main.ts", Path: filepath.Join("src", "main.ts"), Size: 1},
								},
							},
						},
					},
					{
						Path:      "src",
						FileCount: 1,
						Packages: []model.Package{
							{
								Name:      "src",
								Path:      "src",
								FileCount: 1,
								TopFiles: []model.FileSummary{
									{Name: "main.ts", Path: filepath.Join("src", "main.ts"), Size: 1},
								},
							},
						},
					},
				},
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	module := topology.Modules[0]
	if len(module.SourceRoots) != 1 {
		t.Fatalf("len(module.SourceRoots) = %d, want 1 after pruning empty owners", len(module.SourceRoots))
	}
	if module.SourceRoots[0].Path != "src" {
		t.Fatalf("remaining source root path = %q, want src", module.SourceRoots[0].Path)
	}
	if findPackage(module, "") != nil {
		t.Fatalf("root package should be pruned after losing all duplicate ownership: %+v", module.SourceRoots)
	}
}

func TestFinalizeTopologyNestedPackageDoesNotShrinkRootModuleTopFiles(t *testing.T) {
	rootFiles := []model.FileSummary{
		{Name: "README.md", Path: "README.md", Size: 10},
		{Name: "AGENTS.md", Path: "AGENTS.md", Size: 9},
		{Name: "package.json", Path: "package.json", Size: 8},
		{Name: "tsconfig.json", Path: "tsconfig.json", Size: 7},
		{Name: "vite.config.ts", Path: "vite.config.ts", Size: 6},
		{Name: "Dockerfile", Path: "Dockerfile", Size: 5},
		{Name: "Makefile", Path: "Makefile", Size: 4},
		{Name: "LICENSE", Path: "LICENSE", Size: 3},
		{Name: "CHANGELOG.md", Path: "CHANGELOG.md", Size: 2},
		{Name: "index.html", Path: "index.html", Size: 1},
		{Name: "main.ts", Path: "main.ts", Size: 1},
	}
	topology := &model.ProjectTopology{
		Modules: []model.Module{
			{
				Name:      "app",
				Path:      "",
				Language:  "TypeScript",
				FileCount: len(rootFiles) + 4,
				SourceRoots: []model.SourceRoot{
					{
						Path:      "",
						FileCount: len(rootFiles) + 4,
						Packages: []model.Package{
							{
								Name:      "root",
								Path:      "",
								FileCount: len(rootFiles),
								TopFiles:  rootFiles,
							},
							{
								Name:      "src",
								Path:      "src",
								FileCount: 4,
								TopFiles: []model.FileSummary{
									{Name: "alpha.ts", Path: filepath.Join("src", "alpha.ts"), Size: 4},
									{Name: "beta.ts", Path: filepath.Join("src", "beta.ts"), Size: 3},
									{Name: "gamma.ts", Path: filepath.Join("src", "gamma.ts"), Size: 2},
									{Name: "delta.ts", Path: filepath.Join("src", "delta.ts"), Size: 1},
								},
							},
						},
					},
				},
			},
		},
	}

	NewScanner("").finalizeTopology(topology)

	module := topology.Modules[0]
	rootPkg := findPackage(module, "")
	if rootPkg == nil {
		t.Fatalf("missing root package after finalization: %+v", module.SourceRoots)
	}
	if len(rootPkg.TopFiles) != maxRootPackageTopFiles {
		t.Fatalf("len(rootPkg.TopFiles) = %d, want %d", len(rootPkg.TopFiles), maxRootPackageTopFiles)
	}
	srcPkg := findPackage(module, "src")
	if srcPkg == nil {
		t.Fatalf("missing src package after finalization: %+v", module.SourceRoots)
	}
	if len(srcPkg.TopFiles) != maxPackageTopFiles {
		t.Fatalf("len(srcPkg.TopFiles) = %d, want %d", len(srcPkg.TopFiles), maxPackageTopFiles)
	}
	if len(module.TopFiles) != maxRootPackageTopFiles {
		t.Fatalf("len(module.TopFiles) = %d, want %d; module.TopFiles = %+v", len(module.TopFiles), maxRootPackageTopFiles, module.TopFiles)
	}
	for _, path := range []string{"README.md", "AGENTS.md", "package.json", "tsconfig.json"} {
		if !hasTopFile(module.TopFiles, path) {
			t.Fatalf("module.TopFiles = %+v, missing high-signal root file %q", module.TopFiles, path)
		}
	}
}

func TestScanAppliesTopologyPipelineMVPPoliciesTogether(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/mvp\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "README.md"), "# Project\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "spec.md"), "# Spec\n")
	writeTestFile(t, filepath.Join(tmpDir, "docs", "notes.md"), "# Notes\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "index.html"), "<!doctype html>\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "app.js"), "console.log('ok')\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "logo.png"), "png")
	writeTestFile(t, filepath.Join(tmpDir, "public", "ignored.jar"), "jar")
	writeTestFile(t, filepath.Join(tmpDir, "go.sum"), "module sum\n")
	writeTestFile(t, filepath.Join(tmpDir, ".DS_Store"), "junk\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}

	module := topology.Modules[0]
	rootPkg := findPackage(module, "")
	if rootPkg == nil {
		t.Fatalf("root package missing from source roots: %+v", module.SourceRoots)
	}
	if !hasTopFile(rootPkg.TopFiles, "README.md") {
		t.Fatalf("rootPkg.TopFiles = %+v, missing README.md", rootPkg.TopFiles)
	}
	docsPkg := findPackage(module, "docs")
	if docsPkg == nil || !hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "spec.md")) {
		t.Fatalf("docs package = %+v, want docs/spec.md", docsPkg)
	}
	if !hasTopFile(docsPkg.TopFiles, filepath.Join("docs", "notes.md")) {
		t.Fatalf("docsPkg.TopFiles = %+v, missing docs/notes.md", docsPkg.TopFiles)
	}

	publicPkg := findPackage(module, "public")
	if publicPkg == nil {
		t.Fatalf("public package missing from source roots: %+v", module.SourceRoots)
	}
	for _, path := range []string{filepath.Join("public", "index.html"), filepath.Join("public", "app.js")} {
		if !hasTopFile(publicPkg.TopFiles, path) {
			t.Fatalf("publicPkg.TopFiles = %+v, missing %s", publicPkg.TopFiles, path)
		}
	}
	if !hasAuxFile(publicPkg.AuxFiles, filepath.Join("public", "logo.png")) {
		t.Fatalf("publicPkg.AuxFiles = %+v, missing public/logo.png", publicPkg.AuxFiles)
	}

	for _, omitted := range []string{
		"go.sum",
		".DS_Store",
		filepath.Join("public", "ignored.jar"),
	} {
		if hasTopFile(module.TopFiles, omitted) || moduleHasPackageTopFile(module, omitted) || moduleHasPackageAuxFile(module, omitted) {
			t.Fatalf("topology surfaced omitted file %s: module=%+v", omitted, module)
		}
	}
	if countPackageTopFile(module, filepath.Join("public", "index.html")) != 1 {
		t.Fatalf("public/index.html should be surfaced exactly once after standalone/delegated dedup: %+v", module.SourceRoots)
	}
}

func TestScanGenericResourcesFindsRootAndNestedSchemaFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "schema.sql"), "create table orders (id integer primary key);\n")
	writeTestFile(t, filepath.Join(tmpDir, "schemas", "orders.proto"), "syntax = \"proto3\";\n")
	writeTestFile(t, filepath.Join(tmpDir, "api", "schema.graphql"), "type Query { orders: [Order!]! }\n")

	sourceRoots, err := scanGenericResources(tmpDir)
	if err != nil {
		t.Fatalf("scanGenericResources() error = %v", err)
	}
	if len(sourceRoots) != 3 {
		t.Fatalf("len(sourceRoots) = %d, want root, schemas, and api resources: %+v", len(sourceRoots), sourceRoots)
	}
	for _, sourceRoot := range sourceRoots {
		if sourceRoot.Role != "Resources" {
			t.Fatalf("sourceRoot %s Role = %q, want Resources", sourceRoot.Path, sourceRoot.Role)
		}
	}
	if !resourceRootsHaveTopFile(sourceRoots, "schema.sql") {
		t.Fatalf("sourceRoots = %+v, missing root schema.sql", sourceRoots)
	}
	if !resourceRootsHaveTopFile(sourceRoots, filepath.Join("schemas", "orders.proto")) {
		t.Fatalf("sourceRoots = %+v, missing schemas/orders.proto", sourceRoots)
	}
	if !resourceRootsHaveTopFile(sourceRoots, filepath.Join("api", "schema.graphql")) {
		t.Fatalf("sourceRoots = %+v, missing api/schema.graphql", sourceRoots)
	}
}

func TestScanGenericResourcesFindsTopLevelExampleConfigs(t *testing.T) {
	tmpDir := t.TempDir()
	for relPath := range map[string]bool{
		filepath.Join("examples", "actuator.yaml"):          true,
		filepath.Join("examples", "nested", "statlite.yml"): true,
		filepath.Join("example", "single.json"):             true,
		filepath.Join("sample", "finrecord.json"):           true,
		filepath.Join("samples", "settings.toml"):           true,
		filepath.Join("demo", "demo.xml"):                   true,
		filepath.Join("demos", "demo.properties"):           true,
		filepath.Join("examples", ".env.example"):           true,
		filepath.Join("examples", ".env.template"):          true,
		filepath.Join("examples", ".env.sample"):            true,
	} {
		writeTestFile(t, filepath.Join(tmpDir, relPath), "example\n")
	}
	writeTestFile(t, filepath.Join(tmpDir, "examples", ".env"), "secret\n")
	writeTestFile(t, filepath.Join(tmpDir, "examples", "usage.md"), "documentation\n")
	writeTestFile(t, filepath.Join(tmpDir, "examples", "notes.txt"), "documentation\n")
	writeTestFile(t, filepath.Join(tmpDir, "examples", "node_modules", "ignored.yaml"), "ignored\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "examples", "not-public.yaml"), "ignored\n")

	sourceRoots, err := scanGenericResources(tmpDir)
	if err != nil {
		t.Fatalf("scanGenericResources() error = %v", err)
	}
	for _, relPath := range []string{
		filepath.Join("examples", "actuator.yaml"),
		filepath.Join("examples", "nested", "statlite.yml"),
		filepath.Join("example", "single.json"),
		filepath.Join("sample", "finrecord.json"),
		filepath.Join("samples", "settings.toml"),
		filepath.Join("demo", "demo.xml"),
		filepath.Join("demos", "demo.properties"),
		filepath.Join("examples", ".env.example"),
		filepath.Join("examples", ".env.template"),
		filepath.Join("examples", ".env.sample"),
	} {
		if !resourceRootsHaveTopFile(sourceRoots, relPath) {
			t.Fatalf("sourceRoots = %+v, missing %s", sourceRoots, relPath)
		}
	}
	for _, relPath := range []string{
		filepath.Join("examples", ".env"),
		filepath.Join("examples", "node_modules", "ignored.yaml"),
		filepath.Join("src", "examples", "not-public.yaml"),
		filepath.Join("examples", "usage.md"),
		filepath.Join("examples", "notes.txt"),
	} {
		if resourceRootsHaveTopFile(sourceRoots, relPath) {
			t.Fatalf("sourceRoots = %+v, should omit %s", sourceRoots, relPath)
		}
	}
}

func TestScanGenericResourcesPrioritizesExampleConfigsWithinPackageCap(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{
		"guide-a.md", "guide-b.md", "guide-c.md", "guide-d.md", "guide-e.md", "guide-f.md", "notes.txt",
	} {
		writeTestFile(t, filepath.Join(tmpDir, "examples", name), strings.Repeat("documentation\n", 100))
	}
	for _, name := range []string{"actuator.yaml", "statlite.yml"} {
		writeTestFile(t, filepath.Join(tmpDir, "examples", name), "targets: []\n")
	}

	sourceRoots, err := scanGenericResources(tmpDir)
	if err != nil {
		t.Fatalf("scanGenericResources() error = %v", err)
	}
	for _, relPath := range []string{
		filepath.Join("examples", "actuator.yaml"),
		filepath.Join("examples", "statlite.yml"),
	} {
		if !resourceRootsHaveTopFile(sourceRoots, relPath) {
			t.Fatalf("sourceRoots = %+v, missing example config %s", sourceRoots, relPath)
		}
	}
	for _, relPath := range []string{
		filepath.Join("examples", "guide-a.md"),
		filepath.Join("examples", "notes.txt"),
	} {
		if resourceRootsHaveTopFile(sourceRoots, relPath) {
			t.Fatalf("sourceRoots = %+v, should omit example prose %s", sourceRoots, relPath)
		}
	}
}

func TestScanGenericResourcesSkipsDocsWebAndOpsDirs(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "docs", "schema.sql"), "select 1;\n")
	writeTestFile(t, filepath.Join(tmpDir, "public", "schema.sql"), "select 1;\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "schema.sql"), "select 1;\n")
	writeTestFile(t, filepath.Join(tmpDir, ".github", "workflows", "schema.sql"), "select 1;\n")
	writeTestFile(t, filepath.Join(tmpDir, "data", "schema.sql"), "select 1;\n")

	sourceRoots, err := scanGenericResources(tmpDir)
	if err != nil {
		t.Fatalf("scanGenericResources() error = %v", err)
	}
	if len(sourceRoots) != 1 {
		t.Fatalf("sourceRoots = %+v, want only data resources", sourceRoots)
	}
	if !resourceRootsHaveTopFile(sourceRoots, filepath.Join("data", "schema.sql")) {
		t.Fatalf("sourceRoots = %+v, missing data/schema.sql", sourceRoots)
	}
	for _, path := range []string{
		filepath.Join("docs", "schema.sql"),
		filepath.Join("public", "schema.sql"),
		filepath.Join("deploy", "schema.sql"),
		filepath.Join(".github", "workflows", "schema.sql"),
	} {
		if resourceRootsHaveTopFile(sourceRoots, path) {
			t.Fatalf("sourceRoots = %+v, should skip %s", sourceRoots, path)
		}
	}
}

func TestScanGenericResourcesHandlesEmptyProject(t *testing.T) {
	sourceRoots, err := scanGenericResources(t.TempDir())
	if err != nil {
		t.Fatalf("scanGenericResources() error = %v", err)
	}
	if len(sourceRoots) != 0 {
		t.Fatalf("sourceRoots = %+v, want none for empty project", sourceRoots)
	}
}

func TestScanAttachesGenericResourcesToLanguageTopology(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/resources\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "app", "app.go"), "package app\n")
	writeTestFile(t, filepath.Join(tmpDir, "schema.sql"), "create table orders (id integer primary key);\n")
	writeTestFile(t, filepath.Join(tmpDir, "schemas", "orders.proto"), "syntax = \"proto3\";\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	module := topology.Modules[0]
	if !hasTopFile(module.TopFiles, "schema.sql") && !moduleHasPackageTopFile(module, "schema.sql") {
		t.Fatalf("module missing root schema.sql: %+v", module)
	}
	if !hasTopFile(module.TopFiles, filepath.Join("schemas", "orders.proto")) && !moduleHasPackageTopFile(module, filepath.Join("schemas", "orders.proto")) {
		t.Fatalf("module missing schemas/orders.proto: %+v", module)
	}
	if findPackage(module, "schemas") == nil {
		t.Fatalf("module.SourceRoots = %+v, missing schemas package", module.SourceRoots)
	}
}

func TestScanAttachesTopLevelExampleConfigsToLanguageTopology(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/examples\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "examples", "actuator.yaml"), "targets: []\n")
	writeTestFile(t, filepath.Join(tmpDir, "examples", "statlite.yml"), "targets: []\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	for _, relPath := range []string{
		filepath.Join("examples", "actuator.yaml"),
		filepath.Join("examples", "statlite.yml"),
	} {
		if !moduleHasPackageTopFile(topology.Modules[0], relPath) {
			t.Fatalf("module = %+v, missing example config %s", topology.Modules[0], relPath)
		}
	}
}

func TestScanGenericFallbackResourceOverlapDeduplicates(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "schema.sql"), "create table orders (id integer primary key);\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want generic fallback module", len(topology.Modules))
	}
	if countPackageTopFile(topology.Modules[0], "schema.sql") != 1 {
		t.Fatalf("schema.sql should be surfaced exactly once after generic fallback/resource dedup: %+v", topology.Modules[0].SourceRoots)
	}
	if topology.Modules[0].FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want generic fallback count preserved", topology.Modules[0].FileCount)
	}
}

func TestScanGenericResourcesDoNotDuplicateCoLocatedLanguageResources(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/resources\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "store.go"), "package store\n")
	writeTestFile(t, filepath.Join(tmpDir, "internal", "store", "schema.sql"), "create table orders (id integer primary key);\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	module := topology.Modules[0]
	if countPackageTopFile(module, filepath.Join("internal", "store", "schema.sql")) != 1 {
		t.Fatalf("schema.sql should be surfaced exactly once after Go/resource overlap handling: %+v", module.SourceRoots)
	}
	if module.FileCount != 2 {
		t.Fatalf("module.FileCount = %d, want Go source plus co-located SQL resource", module.FileCount)
	}
}

func TestScanReportsMarkerOnlyNodeAsStackNotLanguage(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), `<project></project>`)
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"scripts":{}}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "main", "java", "App.java"), "class App {}")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Java"}) {
		t.Fatalf("Languages = %v, want [Java]", topology.Languages)
	}
	if topology.PrimaryLanguage != "Java" {
		t.Fatalf("PrimaryLanguage = %q, want Java", topology.PrimaryLanguage)
	}
	if !reflect.DeepEqual(topology.Stack, []string{"Maven", "Node.js"}) {
		t.Fatalf("Stack = %v, want [Maven Node.js]", topology.Stack)
	}
}

func TestScanPrimaryLanguageIgnoresManifestResourceVisibility(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/hybrid\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "pom.xml"), `<project><modelVersion>4.0.0</modelVersion></project>`)
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"scripts":{"start":"node index.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "index.js"), "console.log('ok')\n")
	writeTestFile(t, filepath.Join(tmpDir, "pyproject.toml"), "[project]\nname = \"hybrid\"\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "app", "__init__.py"), "")
	writeTestFile(t, filepath.Join(tmpDir, "src", "app", "main.py"), "print('ok')\n")
	writeTestFile(t, filepath.Join(tmpDir, "Makefile"), "test:\n\ttrue\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if topology.PrimaryLanguage != "Python" {
		t.Fatalf("PrimaryLanguage = %q, want Python", topology.PrimaryLanguage)
	}
	if !topologyHasPackageTopFile(topology, "pom.xml") {
		t.Fatalf("topology should still surface pom.xml as a visible root resource: %+v", topology.Modules)
	}
}

func TestScannerUsesNodeDetectorForPureJavaScriptProject(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"web","main":"src/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export function main() { return true }\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	module := topology.Modules[0]
	if module.Name != "web" {
		t.Fatalf("module.Name = %q, want web", module.Name)
	}
	if module.Language != "JavaScript" {
		t.Fatalf("module.Language = %q, want JavaScript", module.Language)
	}
	if topology.PrimaryLanguage != "JavaScript" {
		t.Fatalf("PrimaryLanguage = %q, want JavaScript", topology.PrimaryLanguage)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"JavaScript"}) {
		t.Fatalf("Languages = %v, want [JavaScript]", topology.Languages)
	}
	if !reflect.DeepEqual(topology.Stack, []string{"Node.js"}) {
		t.Fatalf("Stack = %v, want [Node.js]", topology.Stack)
	}
	if !hasSourceRoot(module, "src") {
		t.Fatalf("SourceRoots = %+v, want Node detector src root", module.SourceRoots)
	}
	if !hasPackage(module, "src") {
		t.Fatalf("SourceRoots = %+v, want Node detector package rooted at src", module.SourceRoots)
	}
}

func TestScannerUsesNodeDetectorForPureTypeScriptProject(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"service","types":"dist/index.d.ts"}`)
	writeTestFile(t, filepath.Join(tmpDir, "tsconfig.json"), `{"compilerOptions":{"strict":true}}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.ts"), "export const service = true\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if len(topology.Modules) != 1 {
		t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
	}
	module := topology.Modules[0]
	if module.Name != "service" {
		t.Fatalf("module.Name = %q, want service", module.Name)
	}
	if module.Language != "TypeScript" {
		t.Fatalf("module.Language = %q, want TypeScript", module.Language)
	}
	if topology.PrimaryLanguage != "TypeScript" {
		t.Fatalf("PrimaryLanguage = %q, want TypeScript", topology.PrimaryLanguage)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"TypeScript"}) {
		t.Fatalf("Languages = %v, want [TypeScript]", topology.Languages)
	}
	if !hasSourceRoot(module, "src") {
		t.Fatalf("SourceRoots = %+v, want Node detector src root", module.SourceRoots)
	}
	if !hasTopFile(module.TopFiles, filepath.Join("src", "index.ts")) {
		t.Fatalf("TopFiles = %+v, want Node detector source top file", module.TopFiles)
	}
}

func TestScannerDetectsBasicNodeWorkspaceProject(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"workspace-root",
  "workspaces":["packages/ui","packages/api"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "package.json"), `{"name":"api","main":"server/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "server", "index.js"), "export const api = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{"name":"ui","types":"src/index.ts"}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "src", "index.ts"), "export const ui = true\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if topology.Structure != "Multi-Module" {
		t.Fatalf("Structure = %q, want Multi-Module", topology.Structure)
	}
	if !reflect.DeepEqual(topology.Stack, []string{"Node.js"}) {
		t.Fatalf("Stack = %v, want [Node.js]", topology.Stack)
	}
	if len(topology.Modules) != 3 {
		t.Fatalf("len(topology.Modules) = %d, want workspace root plus two child modules", len(topology.Modules))
	}

	gotPaths := []string{topology.Modules[0].Path, topology.Modules[1].Path, topology.Modules[2].Path}
	wantPaths := []string{"", filepath.Join("packages", "api"), filepath.Join("packages", "ui")}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("module paths = %v, want %v", gotPaths, wantPaths)
	}

	apiModule := findModule(topology.Modules, "api")
	if apiModule == nil || !hasSourceRoot(*apiModule, filepath.Join("packages", "api", "server")) {
		t.Fatalf("modules = %+v, missing api server source root", topology.Modules)
	}
	uiModule := findModule(topology.Modules, "ui")
	if uiModule == nil || !hasSourceRoot(*uiModule, filepath.Join("packages", "ui", "src")) {
		t.Fatalf("modules = %+v, missing ui src source root", topology.Modules)
	}
}

func TestScannerKeepsDeterministicDetectorMergeOrderWithNode(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/api\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "web", "package.json"), `{"name":"web","main":"src/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "web", "src", "index.js"), "export const app = true\n")

	for i := 0; i < 10; i++ {
		topology, err := NewScanner(tmpDir).Scan()
		if err != nil {
			t.Fatalf("Scan() error = %v", err)
		}
		if len(topology.Modules) != 2 {
			t.Fatalf("len(topology.Modules) = %d, want 2", len(topology.Modules))
		}
		modulePaths := []string{topology.Modules[0].Path, topology.Modules[1].Path}
		if !reflect.DeepEqual(modulePaths, []string{"", "web"}) {
			t.Fatalf("module paths = %v, want deterministic root then web order", modulePaths)
		}
		if !reflect.DeepEqual(topology.Languages, []string{"Go", "JavaScript"}) {
			t.Fatalf("Languages = %v, want [Go JavaScript]", topology.Languages)
		}
	}
}

func TestScannerKeepsMixedDetectorOrderingWithNodeStackSignals(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string)
		wantStack []string
		wantPaths []string
	}{
		{
			name: "Go and React",
			setup: func(root string) {
				writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/api\n")
				writeTestFile(t, filepath.Join(root, "main.go"), "package main\n\nfunc main() {}\n")
				writeTestFile(t, filepath.Join(root, "web", "package.json"), `{"name":"web","dependencies":{"react":"^18.0.0"}}`)
				writeTestFile(t, filepath.Join(root, "web", "src", "App.tsx"), "export const App = () => null\n")
			},
			wantStack: []string{"Go Modules", "Node.js", "React"},
			wantPaths: []string{"", "web"},
		},
		{
			name: "Java and Next.js",
			setup: func(root string) {
				writeTestFile(t, filepath.Join(root, "pom.xml"), `<project><groupId>com.example</groupId><artifactId>api</artifactId></project>`)
				writeTestFile(t, filepath.Join(root, "src", "main", "java", "com", "example", "Api.java"), "package com.example;\n\nclass Api {}\n")
				writeTestFile(t, filepath.Join(root, "web", "package.json"), `{"name":"web","dependencies":{"next":"^15.0.0"}}`)
				writeTestFile(t, filepath.Join(root, "web", "next.config.js"), "module.exports = {}\n")
				writeTestFile(t, filepath.Join(root, "web", "app", "page.tsx"), "export default function Page() { return null }\n")
			},
			wantStack: []string{"Maven", "Node.js", "Next.js"},
			wantPaths: []string{"", "web"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			for i := 0; i < 10; i++ {
				topology, err := NewScanner(tmpDir).Scan()
				if err != nil {
					t.Fatalf("Scan() error = %v", err)
				}
				if !reflect.DeepEqual(topology.Stack, tt.wantStack) {
					t.Fatalf("Stack = %v, want %v", topology.Stack, tt.wantStack)
				}
				if len(topology.Modules) != len(tt.wantPaths) {
					t.Fatalf("len(topology.Modules) = %d, want %d", len(topology.Modules), len(tt.wantPaths))
				}
				gotPaths := make([]string, 0, len(topology.Modules))
				for _, module := range topology.Modules {
					gotPaths = append(gotPaths, module.Path)
				}
				if !reflect.DeepEqual(gotPaths, tt.wantPaths) {
					t.Fatalf("module paths = %v, want %v", gotPaths, tt.wantPaths)
				}
			}
		})
	}
}

func TestDetectedStackRecognizesTargetNodeFrameworksConservatively(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "react-app", "package.json"), `{"dependencies":{"react":"^18.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "react-app", "src", "App.tsx"), "export const App = () => null\n")

	writeTestFile(t, filepath.Join(tmpDir, "next-app", "package.json"), `{"dependencies":{"next":"^15.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "next-app", "next.config.js"), "module.exports = {}\n")

	writeTestFile(t, filepath.Join(tmpDir, "vue-app", "package.json"), `{"dependencies":{"vue":"^3.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "vue-app", "src", "App.vue"), "<template><div/></template>\n")

	writeTestFile(t, filepath.Join(tmpDir, "vite-app", "package.json"), `{"devDependencies":{"vite":"^5.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "vite-app", "vite.config.ts"), "export default {}\n")

	writeTestFile(t, filepath.Join(tmpDir, "nest-app", "package.json"), `{"dependencies":{"@nestjs/core":"^11.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "nest-app", "nest-cli.json"), "{}\n")

	stack := detectedStack(tmpDir)
	want := []string{"Node.js", "React", "Next.js", "Vue", "Vite", "NestJS"}
	if !reflect.DeepEqual(stack, want) {
		t.Fatalf("Stack = %v, want %v", stack, want)
	}
}

func TestScannerRecognizesTargetNodeStacks(t *testing.T) {
	tests := []struct {
		name            string
		packageJSON     string
		files           map[string]string
		wantStack       []string
		wantTopFilePath string
	}{
		{
			name:        "React",
			packageJSON: `{"name":"react-app","dependencies":{"react":"^18.0.0"}}`,
			files: map[string]string{
				filepath.Join("src", "App.tsx"):                  "export const App = () => null\n",
				filepath.Join("src", "components", "Button.tsx"): "export const Button = () => null\n",
			},
			wantStack:       []string{"Node.js", "React"},
			wantTopFilePath: filepath.Join("src", "App.tsx"),
		},
		{
			name:        "Next.js",
			packageJSON: `{"name":"next-app","dependencies":{"next":"^15.0.0"}}`,
			files: map[string]string{
				"next.config.js":                 "module.exports = {}\n",
				filepath.Join("app", "page.tsx"): "export default function Page() { return null }\n",
			},
			wantStack:       []string{"Node.js", "Next.js"},
			wantTopFilePath: "next.config.js",
		},
		{
			name:        "Vue",
			packageJSON: `{"name":"vue-app","dependencies":{"vue":"^3.0.0"}}`,
			files: map[string]string{
				filepath.Join("src", "main.ts"): "export const app = true\n",
				filepath.Join("src", "App.vue"): "<template><div/></template>\n",
			},
			wantStack:       []string{"Node.js", "Vue"},
			wantTopFilePath: filepath.Join("src", "App.vue"),
		},
		{
			name:        "Vite",
			packageJSON: `{"name":"vite-app","devDependencies":{"vite":"^5.0.0"}}`,
			files: map[string]string{
				"vite.config.ts":                "export default {}\n",
				filepath.Join("src", "main.ts"): "export const main = true\n",
			},
			wantStack:       []string{"Node.js", "Vite"},
			wantTopFilePath: "vite.config.ts",
		},
		{
			name:        "NestJS",
			packageJSON: `{"name":"nest-app","dependencies":{"@nestjs/core":"^11.0.0"}}`,
			files: map[string]string{
				"nest-cli.json":                 "{}\n",
				filepath.Join("src", "main.ts"): "export const main = true\n",
			},
			wantStack:       []string{"Node.js", "NestJS"},
			wantTopFilePath: "nest-cli.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			writeTestFile(t, filepath.Join(tmpDir, "package.json"), tt.packageJSON)
			for path, content := range tt.files {
				writeTestFile(t, filepath.Join(tmpDir, path), content)
			}

			topology, err := NewScanner(tmpDir).Scan()
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if !reflect.DeepEqual(topology.Stack, tt.wantStack) {
				t.Fatalf("Stack = %v, want %v", topology.Stack, tt.wantStack)
			}
			if len(topology.Modules) != 1 {
				t.Fatalf("len(topology.Modules) = %d, want 1", len(topology.Modules))
			}
			if !hasTopFile(topology.Modules[0].TopFiles, tt.wantTopFilePath) && !moduleHasPackageTopFile(topology.Modules[0], tt.wantTopFilePath) {
				t.Fatalf("module topology missing %s; module=%+v", tt.wantTopFilePath, topology.Modules[0])
			}
		})
	}
}

func TestDetectedStackDoesNotLabelFrameworkFromDependencyAlone(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
		"dependencies":{
			"react":"^18.0.0",
			"next":"^15.0.0",
			"vue":"^3.0.0",
			"@nestjs/core":"^11.0.0"
		},
		"devDependencies":{"vite":"^5.0.0"}
	}`)

	stack := detectedStack(tmpDir)
	want := []string{"Node.js"}
	if !reflect.DeepEqual(stack, want) {
		t.Fatalf("Stack = %v, want %v", stack, want)
	}
}

func TestDetectedStackDoesNotInferFrameworkFromDeepProjectStructureAlone(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"scripts":{"dev":"node server.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "components", "Button.tsx"), "export const Button = () => null\n")
	writeTestFile(t, filepath.Join(tmpDir, "hooks", "useThing.ts"), "export const useThing = () => true\n")
	writeTestFile(t, filepath.Join(tmpDir, "app", "blog", "[slug]", "page.tsx"), "export default function Page() { return null }\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "modules", "app.module.ts"), "export class AppModule {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "controllers", "user.controller.ts"), "export class UserController {}\n")

	stack := detectedStack(tmpDir)
	want := []string{"Node.js"}
	if !reflect.DeepEqual(stack, want) {
		t.Fatalf("Stack = %v, want %v", stack, want)
	}
}

func topologyHasPackageTopFile(topology *model.ProjectTopology, path string) bool {
	for _, module := range topology.Modules {
		if moduleHasPackageTopFile(module, path) {
			return true
		}
	}
	return false
}

func moduleHasPackageAuxFile(module model.Module, path string) bool {
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			if hasAuxFile(pkg.AuxFiles, path) {
				return true
			}
		}
	}
	return false
}

func countPackageTopFile(module model.Module, path string) int {
	count := 0
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			for _, file := range pkg.TopFiles {
				if file.Path == path {
					count++
				}
			}
		}
	}
	return count
}

func resourceRootsHaveTopFile(sourceRoots []model.SourceRoot, path string) bool {
	for _, sourceRoot := range sourceRoots {
		for _, pkg := range sourceRoot.Packages {
			if hasTopFile(pkg.TopFiles, path) {
				return true
			}
		}
	}
	return false
}
