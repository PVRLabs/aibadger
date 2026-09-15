// This file finds relevant build, setup, configuration, and script files and
// adds them to the project topology as context alongside source files.
package scanner

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

const (
	projectContextRole             = "Project Context"
	maxProjectContextPackageFiles  = 5
	maxProjectContextFiles         = 50
	projectContextHelperRank       = 0
	projectContextConfigNameRank   = 1
	projectContextScriptRank       = 2
	projectContextConfigFormatRank = 3
)

type projectContextCandidate struct {
	summary model.FileSummary
	dir     string
	rank    int
}

var projectContextScriptExtensions = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".ps1": true,
	".psm1": true, ".bat": true, ".cmd": true,
}

var projectContextConfigExtensions = map[string]bool{
	".yaml": true, ".yml": true, ".toml": true, ".ini": true,
	".conf": true, ".properties": true,
}

var projectContextHelperStems = map[string]bool{
	"build": true, "compile": true, "bootstrap": true,
	"setup": true, "release": true, "clean": true,
}

func classifyProjectContextCandidate(projectRoot, path string, size int64) (projectContextCandidate, bool) {
	rel, full, ok := normalizeSourcePath(projectRoot, path)
	if !ok {
		return projectContextCandidate{}, false
	}
	name := filepath.Base(rel)
	if _, source := classifyGenericSourceCandidate(projectRoot, full); source ||
		shouldOmitFile(projectRoot, full, name) ||
		isUnderIgnoredDir(projectRoot, full, NewGenericDetector().Exclusions) {
		return projectContextCandidate{}, false
	}
	rank, eligible := projectContextRank(name)
	if !eligible {
		return projectContextCandidate{}, false
	}
	kind := filekind.Classify(full)
	if kind == model.FileKindAsset || kind == model.FileKindBinary || projectContextFileLooksBinary(full) {
		return projectContextCandidate{}, false
	}
	return projectContextCandidate{
		summary: model.FileSummary{Name: name, Path: rel, Size: size, Kind: kind},
		dir:     relativeDirFromFile(projectRoot, full),
		rank:    rank,
	}, true
}

func projectContextFileLooksBinary(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()
	buf := make([]byte, 4096)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return true
	}
	return bytes.IndexByte(buf[:n], 0) >= 0
}

func projectContextRank(name string) (int, bool) {
	lower := strings.ToLower(name)
	ext := strings.ToLower(filepath.Ext(lower))
	stem := strings.TrimSuffix(lower, ext)

	if lower == "makefile" || lower == "justfile" ||
		projectContextHelperStems[stem] && (ext == "" || projectContextScriptExtensions[ext]) {
		return projectContextHelperRank, true
	}
	if lower == "config.json" || lower == "settings.json" ||
		lower == "config.xml" || lower == "settings.xml" ||
		isAppSettingsFileName(lower) ||
		(stem == "config" || stem == "settings") && projectContextConfigExtensions[ext] {
		return projectContextConfigNameRank, true
	}
	if projectContextScriptExtensions[ext] {
		return projectContextScriptRank, true
	}
	if projectContextConfigExtensions[ext] {
		return projectContextConfigFormatRank, true
	}
	return 0, false
}

func selectProjectContextCandidates(candidates []projectContextCandidate) []projectContextCandidate {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		if fileKindRank(candidates[i].summary.Kind) != fileKindRank(candidates[j].summary.Kind) {
			return fileKindRank(candidates[i].summary.Kind) < fileKindRank(candidates[j].summary.Kind)
		}
		if candidates[i].summary.Size != candidates[j].summary.Size {
			return candidates[i].summary.Size > candidates[j].summary.Size
		}
		return candidates[i].summary.Path < candidates[j].summary.Path
	})

	selected := make([]projectContextCandidate, 0, min(len(candidates), maxProjectContextFiles))
	perDir := make(map[string]int)
	for _, candidate := range candidates {
		if perDir[candidate.dir] >= maxProjectContextPackageFiles {
			continue
		}
		selected = append(selected, candidate)
		perDir[candidate.dir]++
		if len(selected) == maxProjectContextFiles {
			break
		}
	}
	return selected
}

type projectContextModuleTarget struct {
	index    int
	path     string
	identity string
}

func attachProjectContextToTopology(topology *model.ProjectTopology, candidates []projectContextCandidate) {
	if len(candidates) == 0 || len(topology.Modules) == 0 {
		return
	}
	existing := surfacedTopologyFilePaths(topology)
	remaining := make([]projectContextCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if !existing[normalizeTopologyFilePath(candidate.summary.Path)] {
			remaining = append(remaining, candidate)
		}
	}
	candidates = selectProjectContextCandidates(remaining)
	targets := projectContextModuleTargets(topology.Modules)
	for _, candidate := range candidates {
		target := projectContextOwner(targets, candidate.summary.Path)
		if target < 0 {
			continue
		}
		attachProjectContextFile(&topology.Modules[target], candidate)
		existing[normalizeTopologyFilePath(candidate.summary.Path)] = true
	}
}

func projectContextModuleTargets(modules []model.Module) []projectContextModuleTarget {
	var targets []projectContextModuleTarget
	for idx := range modules {
		if modules[idx].Coverage {
			continue
		}
		targets = append(targets, projectContextModuleTarget{
			index: idx, path: normalizeRelativeDir(filepath.Clean(modules[idx].Path)),
			identity: projectContextModuleIdentity(modules[idx]),
		})
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].identity < targets[j].identity })
	return targets
}

func projectContextModuleIdentity(module model.Module) string {
	return stableModuleContentIdentity(module, true)
}

func isEnrichmentSourceRootRole(role string) bool {
	switch role {
	case "Documentation", "Web Resources", "Ops/Deploy", "Resources", projectContextRole:
		return true
	default:
		return false
	}
}

func stableModuleContentIdentity(module model.Module, excludeEnrichment bool) string {
	stable := module
	stable.SourceRoots = nil
	stable.TopFiles = append([]model.FileSummary(nil), module.TopFiles...)
	stable.AuxFiles = append([]model.FileSummary(nil), module.AuxFiles...)
	sortStableFileSummaries(stable.TopFiles)
	sortStableFileSummaries(stable.AuxFiles)
	for _, sourceRoot := range module.SourceRoots {
		if excludeEnrichment && isEnrichmentSourceRootRole(sourceRoot.Role) {
			continue
		}
		rootCopy := sourceRoot
		rootCopy.Packages = make([]model.Package, len(sourceRoot.Packages))
		for idx, pkg := range sourceRoot.Packages {
			pkgCopy := pkg
			pkgCopy.TopFiles = append([]model.FileSummary(nil), pkg.TopFiles...)
			pkgCopy.AuxFiles = append([]model.FileSummary(nil), pkg.AuxFiles...)
			sortStableFileSummaries(pkgCopy.TopFiles)
			sortStableFileSummaries(pkgCopy.AuxFiles)
			rootCopy.Packages[idx] = pkgCopy
		}
		sort.Slice(rootCopy.Packages, func(i, j int) bool {
			return packageSortKey(rootCopy.Packages[i]) < packageSortKey(rootCopy.Packages[j])
		})
		stable.SourceRoots = append(stable.SourceRoots, rootCopy)
	}
	sort.Slice(stable.SourceRoots, func(i, j int) bool {
		return sourceRootSortKey(stable.SourceRoots[i]) < sourceRootSortKey(stable.SourceRoots[j])
	})
	encoded, _ := json.Marshal(stable)
	return string(encoded)
}

func sortStableFileSummaries(files []model.FileSummary) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}
		if files[i].Name != files[j].Name {
			return files[i].Name < files[j].Name
		}
		if files[i].Kind != files[j].Kind {
			return files[i].Kind < files[j].Kind
		}
		return files[i].Size < files[j].Size
	})
}

func projectContextOwner(targets []projectContextModuleTarget, candidatePath string) int {
	winner := -1
	winnerDepth := -1
	for _, target := range targets {
		if !sameOrDescendantRelativePath(target.path, candidatePath) {
			continue
		}
		depth := topologyPackageSpecificity(target.path)
		if depth > winnerDepth {
			winner = target.index
			winnerDepth = depth
		}
	}
	if winner >= 0 {
		return winner
	}
	if len(targets) == 0 {
		return -1
	}
	for _, target := range targets {
		if target.path == "" {
			return target.index
		}
	}
	winnerTarget := targets[0]
	for _, target := range targets[1:] {
		if target.path < winnerTarget.path || target.path == winnerTarget.path && target.identity < winnerTarget.identity {
			winnerTarget = target
		}
	}
	return winnerTarget.index
}

func attachProjectContextFile(module *model.Module, candidate projectContextCandidate) {
	root := model.SourceRoot{Path: candidate.dir, Role: projectContextRole, FileCount: 1}
	pkgName := filepath.Base(candidate.dir)
	if candidate.dir == "" {
		pkgName = "root"
	}
	pkg := model.Package{Name: pkgName, Path: candidate.dir, FileCount: 1, TopFiles: []model.FileSummary{candidate.summary}}
	pkg.Heaviest = heaviestFromSummary(candidate.summary)
	root.Packages = []model.Package{pkg}

	for idx := range module.SourceRoots {
		if module.SourceRoots[idx].Path == root.Path && module.SourceRoots[idx].Role == root.Role {
			module.SourceRoots[idx].FileCount++
			module.SourceRoots[idx].Packages[0].FileCount++
			module.SourceRoots[idx].Packages[0].TopFiles = addTopFile(module.SourceRoots[idx].Packages[0].TopFiles, candidate.summary, maxProjectContextPackageFiles)
			module.SourceRoots[idx].Packages[0].Heaviest = heaviestFromSummary(module.SourceRoots[idx].Packages[0].TopFiles[0])
			return
		}
	}
	module.SourceRoots = append(module.SourceRoots, root)
}
