// This file owns C++ summary pairing, selection, and ranking shared across the
// detector and topology finalization stages.
package scanner

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
)

func heaviestCppFile(files []model.FileSummary) model.HeaviestFile {
	var heaviest model.FileSummary
	found := false
	for _, file := range files {
		if strings.EqualFold(file.Name, "CMakeLists.txt") {
			continue
		}
		if !found || file.Size > heaviest.Size || file.Size == heaviest.Size && file.Path < heaviest.Path {
			heaviest = file
			found = true
		}
	}
	if !found {
		return model.HeaviestFile{}
	}
	return heaviestFromSummary(heaviest)
}

type cppUnit struct{ files []model.FileSummary }

func selectCppPackageFiles(files []model.FileSummary, limit int) []model.FileSummary {
	return selectCppFiles(files, limit, false)
}

func selectCppModuleFiles(files []model.FileSummary, limit int) []model.FileSummary {
	return selectCppFiles(files, limit, true)
}

func selectCppFiles(files []model.FileSummary, limit int, mirrored bool) []model.FileSummary {
	files = uniqueCppFiles(files)
	pairs := cppPairs(files, mirrored)
	used := make(map[string]bool)
	units := make([]cppUnit, 0, len(files))
	for _, file := range files {
		if used[file.Path] {
			continue
		}
		unit := cppUnit{files: []model.FileSummary{file}}
		if other, ok := pairs[file.Path]; ok && !used[other] {
			for _, candidate := range files {
				if candidate.Path == other {
					unit.files = append(unit.files, candidate)
					break
				}
			}
		}
		for _, member := range unit.files {
			used[member.Path] = true
		}
		sortCppFileSummaries(unit.files)
		units = append(units, unit)
	}
	sort.SliceStable(units, func(i, j int) bool { return cppFileLess(units[i].files[0], units[j].files[0]) })
	if len(units) > limit {
		units = units[:limit]
	}
	selected := make([]model.FileSummary, 0, limit)
	for _, unit := range units {
		selected = append(selected, unit.files...)
	}
	sortCppFileSummaries(selected)
	return selected
}

func cppPairs(files []model.FileSummary, allowMirrored bool) map[string]string {
	pairs := make(map[string]string)
	used := make(map[string]bool)
	groupCppPairs(files, false, pairs, used)
	if allowMirrored {
		groupCppPairs(files, true, pairs, used)
	}
	return pairs
}

func groupCppPairs(files []model.FileSummary, mirrored bool, pairs map[string]string, used map[string]bool) {
	type sides struct{ headers, impls []model.FileSummary }
	groups := make(map[string]*sides)
	for _, file := range files {
		if used[file.Path] || !isCppPairFile(file.Name) {
			continue
		}
		key := filepath.Join(filepath.Dir(file.Path), cppBase(file.Name))
		if mirrored {
			parts := strings.Split(filepath.Clean(file.Path), string(filepath.Separator))
			if len(parts) < 2 || parts[0] != "src" && parts[0] != "include" {
				continue
			}
			key = filepath.Join(append(parts[1:len(parts)-1], cppBase(file.Name))...)
		}
		group := groups[key]
		if group == nil {
			group = &sides{}
			groups[key] = group
		}
		if isCppHeader(file.Name) {
			group.headers = append(group.headers, file)
		} else {
			group.impls = append(group.impls, file)
		}
	}
	for _, group := range groups {
		if len(group.headers) != 1 || len(group.impls) != 1 {
			continue
		}
		header, impl := group.headers[0], group.impls[0]
		pairs[header.Path], pairs[impl.Path] = impl.Path, header.Path
		used[header.Path], used[impl.Path] = true, true
	}
}

func isCppPairFile(name string) bool { return isCppHeader(name) || isCppImplementation(name) }

func isCppHeader(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".h", ".hpp", ".hh", ".hxx":
		return true
	default:
		return false
	}
}

func isCppImplementation(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".c", ".cc", ".cpp", ".cxx":
		return true
	default:
		return false
	}
}

func cppBase(name string) string { return strings.TrimSuffix(name, filepath.Ext(name)) }

func uniqueCppFiles(files []model.FileSummary) []model.FileSummary {
	byPath := make(map[string]model.FileSummary, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	unique := make([]model.FileSummary, 0, len(byPath))
	for _, file := range byPath {
		unique = append(unique, file)
	}
	sortCppFileSummaries(unique)
	return unique
}

func cppFileRank(file model.FileSummary) int {
	switch filepath.ToSlash(strings.ToLower(file.Path)) {
	case "cmakelists.txt":
		return 0
	case "main.cpp":
		return 1
	case "src/main.cpp":
		return 2
	default:
		return 3
	}
}

func cppFileLess(left, right model.FileSummary) bool {
	if cppFileRank(left) != cppFileRank(right) {
		return cppFileRank(left) < cppFileRank(right)
	}
	if topologyFilePriority(left) != topologyFilePriority(right) {
		return topologyFilePriority(left) > topologyFilePriority(right)
	}
	if left.Size != right.Size {
		return left.Size > right.Size
	}
	return left.Path < right.Path
}

func sortCppFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool { return cppFileLess(files[i], files[j]) })
}

func sortCppModuleFileSummaries(module *model.Module) {
	ownedPaths := make(map[string]bool)
	for sourceRootIdx := range module.SourceRoots {
		sourceRoot := &module.SourceRoots[sourceRootIdx]
		if !isCppOwnedSourceRoot(sourceRoot) {
			continue
		}
		for _, pkg := range sourceRoot.Packages {
			for _, file := range pkg.TopFiles {
				ownedPaths[file.Path] = true
			}
		}
	}
	sort.SliceStable(module.TopFiles, func(i, j int) bool {
		left, right := module.TopFiles[i], module.TopFiles[j]
		leftRank, rightRank := 3, 3
		if ownedPaths[left.Path] {
			leftRank = cppFileRank(left)
		}
		if ownedPaths[right.Path] {
			rightRank = cppFileRank(right)
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if topologyFilePriority(left) != topologyFilePriority(right) {
			return topologyFilePriority(left) > topologyFilePriority(right)
		}
		if left.Size != right.Size {
			return left.Size > right.Size
		}
		return left.Path < right.Path
	})
}

func isFirstClassCppModule(module *model.Module) bool {
	return module != nil && module.Language == "C++" && module.Path == "" && !module.Coverage
}

func isCppOwnedSourceRoot(sourceRoot *model.SourceRoot) bool {
	if sourceRoot == nil {
		return false
	}
	switch sourceRoot.Role {
	case "Main Source", "Main Header", "Test Source":
		return true
	default:
		return false
	}
}

func shouldKeepCppContextSourceRootSeparate(module *model.Module, existing *model.SourceRoot, incoming model.SourceRoot) bool {
	return isFirstClassCppModule(module) && isCppOwnedSourceRoot(existing) && !isCppOwnedSourceRoot(&incoming)
}
