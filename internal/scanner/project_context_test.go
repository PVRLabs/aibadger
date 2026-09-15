package scanner

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestProjectContextEligibilityAndRanking(t *testing.T) {
	tests := []struct {
		name     string
		wantRank int
		want     bool
	}{
		{name: "compile.sh", wantRank: projectContextHelperRank, want: true},
		{name: "bootstrap.ps1", wantRank: projectContextHelperRank, want: true},
		{name: "settings.yaml", wantRank: projectContextConfigNameRank, want: true},
		{name: "config.json", wantRank: projectContextConfigNameRank, want: true},
		{name: "appsettings.Development.json", wantRank: projectContextConfigNameRank, want: true},
		{name: "maintenance.sh", wantRank: projectContextScriptRank, want: true},
		{name: "project.toml", wantRank: projectContextConfigFormatRank, want: true},
		{name: "fixture.json", want: false},
		{name: "document.xml", want: false},
		{name: "notes.txt", want: false},
		{name: "compile.rb", want: false},
		{name: "project.gpr", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rank, got := projectContextRank(tt.name)
			if got != tt.want || got && rank != tt.wantRank {
				t.Fatalf("projectContextRank(%q) = (%d, %v), want (%d, %v)", tt.name, rank, got, tt.wantRank, tt.want)
			}
		})
	}
}

func TestSelectProjectContextCandidatesPreservesHighSignalNamesWithinCaps(t *testing.T) {
	var candidates []projectContextCandidate
	for idx := 0; idx < maxProjectContextPackageFiles; idx++ {
		candidates = append(candidates, projectContextCandidate{
			summary: model.FileSummary{Name: fmt.Sprintf("arbitrary-%d.sh", idx), Path: fmt.Sprintf("scripts/arbitrary-%d.sh", idx), Size: 1000, Kind: model.FileKindSource},
			dir:     "scripts", rank: projectContextScriptRank,
		})
	}
	candidates = append(candidates, projectContextCandidate{
		summary: model.FileSummary{Name: "compile.sh", Path: "scripts/compile.sh", Size: 1, Kind: model.FileKindSource},
		dir:     "scripts", rank: projectContextHelperRank,
	})

	selected := selectProjectContextCandidates(candidates)
	if len(selected) != maxProjectContextPackageFiles || !projectContextCandidatesContain(selected, "scripts/compile.sh") {
		t.Fatalf("selected candidates = %+v, want compile.sh plus a bounded directory selection", selected)
	}
}

func TestSelectProjectContextCandidatesAppliesGlobalCapDeterministically(t *testing.T) {
	var candidates []projectContextCandidate
	for idx := maxProjectContextFiles + 9; idx >= 0; idx-- {
		path := fmt.Sprintf("area-%02d/project.yaml", idx)
		candidates = append(candidates, projectContextCandidate{
			summary: model.FileSummary{Name: "project.yaml", Path: path, Size: 1, Kind: model.FileKindSource},
			dir:     filepath.Dir(path), rank: projectContextConfigFormatRank,
		})
	}
	selected := selectProjectContextCandidates(candidates)
	if len(selected) != maxProjectContextFiles {
		t.Fatalf("selected %d context files, want global cap %d", len(selected), maxProjectContextFiles)
	}
	if selected[0].summary.Path != "area-00/project.yaml" || selected[len(selected)-1].summary.Path != "area-49/project.yaml" {
		t.Fatalf("global selection is not deterministic by path: first=%q last=%q", selected[0].summary.Path, selected[len(selected)-1].summary.Path)
	}
}

func TestCollectGenericAugmentationSharesSourceAndContextTraversal(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "tools", "worker.rb"), "puts 'work'\n")
	writeTestFile(t, filepath.Join(root, "scripts", "compile.sh"), "g++ main.cpp\n")
	writeTestFile(t, filepath.Join(root, "config", "settings.yaml"), "mode: test\n")
	writeTestFile(t, filepath.Join(root, "fixtures", "data.json"), "{}\n")
	writeTestFile(t, filepath.Join(root, "build", "ignored.sh"), "ignored\n")

	modules, context, err := NewGenericDetector().collectGenericAugmentation(root, semanticSourceOwnership{projectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := coveragePaths(modules); !reflect.DeepEqual(got, []string{"main.go", "tools/worker.rb"}) {
		t.Fatalf("coverage paths = %v", got)
	}
	if got := projectContextCandidatePaths(context); !reflect.DeepEqual(got, []string{"config/settings.yaml", "scripts/compile.sh"}) {
		t.Fatalf("context paths = %v", got)
	}
}

func TestCollectGenericAugmentationRejectsBinarySensitiveAndIgnoredContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "safe.yaml"), "safe: true\n")
	writeTestFile(t, filepath.Join(root, "binary.yaml"), "text\x00binary")
	writeTestFile(t, filepath.Join(root, ".azure", "settings.yaml"), "token: secret\n")
	writeTestFile(t, filepath.Join(root, "vendor", "vendored.sh"), "ignored\n")
	writeTestFile(t, filepath.Join(root, "dist", "generated.toml"), "ignored = true\n")

	_, context, err := NewGenericDetector().collectGenericAugmentation(root, semanticSourceOwnership{projectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if got := projectContextCandidatePaths(context); !reflect.DeepEqual(got, []string{"safe.yaml"}) {
		t.Fatalf("context paths = %v, want only safe text", got)
	}
}

func TestProjectContextAdversarialMixedFilesStayBoundedAndSignalOriented(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() { return 0; }\n")
	for idx := 0; idx < 35; idx++ {
		dir := filepath.Join("random-area", fmt.Sprintf("run-%02d", idx))
		writeTestFile(t, filepath.Join(root, dir, "project.json"), "{}\n")
		writeTestFile(t, filepath.Join(root, dir, "project.yaml"), "enabled: true\n")
		writeTestFile(t, filepath.Join(root, dir, "maintenance.sh"), "echo ready\n")
		writeTestFile(t, filepath.Join(root, dir, "notes.txt"), "arbitrary text\n")
	}
	ownership := semanticSourceOwnership{projectRoot: root, claims: []semanticSourceClaim{{
		language: "C++", areas: []sourceClaimArea{{root: ""}},
	}}}
	detector := NewGenericDetector()
	modules, candidates, err := detector.collectGenericAugmentation(root, ownership)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 70 {
		t.Fatalf("raw eligible candidates = %d, want the 70 approved YAML/script files", len(candidates))
	}
	if len(modules) != 0 {
		t.Fatalf("claimed C++ sources unexpectedly produced coverage modules: %+v", modules)
	}

	topology := &model.ProjectTopology{Modules: []model.Module{{
		Name: "cpp", Path: "", Language: "C++",
		SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source"}},
	}}}
	attachProjectContextToTopology(topology, candidates)
	var surfaced, yamlFiles, scripts int
	for _, sourceRoot := range topology.Modules[0].SourceRoots {
		if sourceRoot.Role != projectContextRole {
			continue
		}
		if sourceRoot.FileCount > maxProjectContextPackageFiles {
			t.Fatalf("package %q exceeds context cap: %d", sourceRoot.Path, sourceRoot.FileCount)
		}
		for _, pkg := range sourceRoot.Packages {
			for _, file := range pkg.TopFiles {
				surfaced++
				switch filepath.Ext(file.Name) {
				case ".yaml":
					yamlFiles++
				case ".sh":
					scripts++
				case ".json", ".txt":
					t.Fatalf("unapproved arbitrary file surfaced: %s", file.Path)
				}
			}
		}
	}
	if surfaced != maxProjectContextFiles || scripts != 35 || yamlFiles != maxProjectContextFiles-35 {
		t.Fatalf("bounded selection = %d total (%d scripts, %d YAML), want 50 (35 scripts, 15 YAML)", surfaced, scripts, yamlFiles)
	}
}

func TestAttachProjectContextUsesDeepestStructuralOwnerAndFallback(t *testing.T) {
	topology := &model.ProjectTopology{Modules: []model.Module{
		{Name: "coverage", Path: "", Language: "Ruby", Coverage: true},
		{Name: "service", Path: "services", Language: "Go", SourceRoots: []model.SourceRoot{{Path: "services", Role: "Main Source"}}},
		{Name: "api", Path: "services/api", Language: "Java", SourceRoots: []model.SourceRoot{{Path: "services/api/src", Role: "Main Source"}}},
		{Name: "tool", Path: "tools", Language: "Python", SourceRoots: []model.SourceRoot{{Path: "tools", Role: "Main Source"}}},
	}}
	candidates := []projectContextCandidate{
		projectContextTestCandidate("services/api/settings.yaml"),
		projectContextTestCandidate("build.sh"),
	}

	attachProjectContextToTopology(topology, candidates)
	if !moduleHasProjectContext(topology.Modules[2], "services/api/settings.yaml") {
		t.Fatalf("deep candidate not attached to api module: %+v", topology.Modules)
	}
	if !moduleHasProjectContext(topology.Modules[1], "build.sh") {
		t.Fatalf("root candidate not attached to lexicographically smallest fallback path: %+v", topology.Modules)
	}
	if len(topology.Modules[0].SourceRoots) != 0 {
		t.Fatalf("Coverage module received context: %+v", topology.Modules[0])
	}
}

func TestAttachProjectContextAppliesCapsAfterExistingOwnershipFilter(t *testing.T) {
	var ownedFiles []model.FileSummary
	var candidates []projectContextCandidate
	for _, stem := range []string{"build", "compile", "bootstrap", "setup", "release"} {
		path := filepath.Join("scripts", stem+".sh")
		file := model.FileSummary{Name: filepath.Base(path), Path: path, Size: 100, Kind: model.FileKindSource}
		ownedFiles = append(ownedFiles, file)
		candidates = append(candidates, projectContextCandidate{summary: file, dir: "scripts", rank: projectContextHelperRank})
	}
	missing := projectContextCandidate{
		summary: model.FileSummary{Name: "maintenance.sh", Path: "scripts/maintenance.sh", Size: 1, Kind: model.FileKindSource},
		dir:     "scripts", rank: projectContextScriptRank,
	}
	candidates = append(candidates, missing)
	topology := &model.ProjectTopology{Modules: []model.Module{{
		Name: "app", Path: "", Language: "Go",
		SourceRoots: []model.SourceRoot{{
			Path: "scripts", Role: "Ops/Deploy", FileCount: len(ownedFiles),
			Packages: []model.Package{{Name: "scripts", Path: "scripts", FileCount: len(ownedFiles), TopFiles: ownedFiles}},
		}},
	}}}

	attachProjectContextToTopology(topology, candidates)
	if !moduleHasProjectContext(topology.Modules[0], missing.summary.Path) {
		t.Fatalf("existing enrichment candidates consumed the context cap: %+v", topology.Modules[0])
	}
	if moduleProjectContextCount(topology.Modules[0]) != 1 {
		t.Fatalf("project context count = %d, want only the missing candidate", moduleProjectContextCount(topology.Modules[0]))
	}
}

func TestAttachProjectContextEqualRootsUseStablePreContextIdentity(t *testing.T) {
	for _, modules := range [][]model.Module{
		{
			{Name: "zeta", Path: "", Language: "Go", SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source"}}},
			{Name: "alpha", Path: "", Language: "Java", SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source"}}},
		},
		{
			{Name: "alpha", Path: "", Language: "Java", SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source"}}},
			{Name: "zeta", Path: "", Language: "Go", SourceRoots: []model.SourceRoot{{Path: "src", Role: "Main Source"}}},
		},
	} {
		topology := &model.ProjectTopology{Modules: modules}
		attachProjectContextToTopology(topology, []projectContextCandidate{
			projectContextTestCandidate("build.sh"),
			projectContextTestCandidate("settings.yaml"),
		})
		for _, module := range topology.Modules {
			if module.Name == "alpha" && (!moduleHasProjectContext(module, "build.sh") || !moduleHasProjectContext(module, "settings.yaml")) {
				t.Fatalf("stable winner lacks context: %+v", topology.Modules)
			}
			if module.Name == "zeta" && moduleProjectContextCount(module) != 0 {
				t.Fatalf("later attachment changed equal-root ownership: %+v", topology.Modules)
			}
		}
	}
}

func TestIdenticalBasicModuleIdentitiesFinalizeDeterministically(t *testing.T) {
	module := func(pkg string) model.Module {
		path := filepath.Join("src", pkg, "main.go")
		file := model.FileSummary{Name: "main.go", Path: path, Size: 10, Kind: model.FileKindSource}
		return model.Module{
			Name: "app", Path: "", Language: "Go", FileCount: 1,
			SourceRoots: []model.SourceRoot{{
				Path: "src", Role: "Main Source", FileCount: 1,
				Packages: []model.Package{{Name: pkg, Path: filepath.Join("src", pkg), FileCount: 1, TopFiles: []model.FileSummary{file}}},
			}},
		}
	}
	orders := [][]model.Module{{module("zeta"), module("alpha")}, {module("alpha"), module("zeta")}}
	var finalized []*model.ProjectTopology
	root := t.TempDir()
	for _, modules := range orders {
		topology := &model.ProjectTopology{Modules: modules}
		attachProjectContextToTopology(topology, []projectContextCandidate{projectContextTestCandidate("build.sh")})
		(&Scanner{ProjectRoot: root}).finalizeTopology(topology)
		finalized = append(finalized, topology)
	}
	if !reflect.DeepEqual(finalized[0], finalized[1]) {
		t.Fatalf("reversed equal-basic-identity modules finalized differently:\nfirst=%+v\nsecond=%+v", finalized[0].Modules, finalized[1].Modules)
	}
}

func TestScannerPreservesFirstClassContextWithoutLanguageOrModuleChanges(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.cpp"), "int main() { return 0; }\n")
	for idx := 0; idx < maxProjectContextPackageFiles; idx++ {
		writeTestFile(t, filepath.Join(root, "scripts", fmt.Sprintf("arbitrary-%d.sh", idx)), strings.Repeat("x", 100))
	}
	writeTestFile(t, filepath.Join(root, "scripts", "compile.sh"), "g++ main.cpp\n")
	writeTestFile(t, filepath.Join(root, "config", "settings.yaml"), "compiler: g++\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if got := topology.Languages; !reflect.DeepEqual(got, []string{"C++"}) || topology.PrimaryLanguage != "C++" {
		t.Fatalf("language accounting changed: languages=%v primary=%q", got, topology.PrimaryLanguage)
	}
	structural := 0
	for _, module := range topology.Modules {
		if !module.Coverage {
			structural++
		}
	}
	if structural != 1 {
		t.Fatalf("structural module count = %d, want 1: %+v", structural, topology.Modules)
	}
	if !topologyHasProjectContext(topology, "scripts/compile.sh") || !topologyHasProjectContext(topology, "config/settings.yaml") {
		t.Fatalf("accepted project context missing: %+v", topology.Modules)
	}
	assertUniqueSurfacedPaths(t, topology)
}

func TestScannerPreservesPythonProjectConfigurationAsContext(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "main.py"), "print('hello')\n")
	writeTestFile(t, filepath.Join(root, "config", "settings.yaml"), "mode: local\n")

	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	if topology.PrimaryLanguage != "Python" || !reflect.DeepEqual(topology.Languages, []string{"Python"}) {
		t.Fatalf("language accounting changed: languages=%v primary=%q", topology.Languages, topology.PrimaryLanguage)
	}
	if !topologyHasProjectContext(topology, "config/settings.yaml") {
		t.Fatalf("Python project context missing: %+v", topology.Modules)
	}
}

func projectContextTestCandidate(path string) projectContextCandidate {
	return projectContextCandidate{
		summary: model.FileSummary{Name: filepath.Base(path), Path: path, Size: 1, Kind: model.FileKindSource},
		dir:     normalizeRelativeDir(filepath.Dir(path)), rank: projectContextHelperRank,
	}
}

func projectContextCandidatesContain(candidates []projectContextCandidate, path string) bool {
	for _, candidate := range candidates {
		if candidate.summary.Path == path {
			return true
		}
	}
	return false
}

func projectContextCandidatePaths(candidates []projectContextCandidate) []string {
	paths := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		paths = append(paths, filepath.ToSlash(candidate.summary.Path))
	}
	return paths
}

func moduleHasProjectContext(module model.Module, path string) bool {
	for _, root := range module.SourceRoots {
		if root.Role != projectContextRole {
			continue
		}
		for _, pkg := range root.Packages {
			for _, file := range pkg.TopFiles {
				if filepath.Clean(file.Path) == filepath.Clean(path) {
					return true
				}
			}
		}
	}
	return false
}

func moduleProjectContextCount(module model.Module) int {
	count := 0
	for _, root := range module.SourceRoots {
		if root.Role == projectContextRole {
			count += root.FileCount
		}
	}
	return count
}

func topologyHasProjectContext(topology *model.ProjectTopology, path string) bool {
	for _, module := range topology.Modules {
		if moduleHasProjectContext(module, path) {
			return true
		}
	}
	return false
}
