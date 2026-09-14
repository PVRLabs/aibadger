package scanner

// This file owns surfaced file de-duplication and winner selection.

import (
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
)

type topologyFileCandidate struct {
	summary      model.FileSummary
	modulePath   string
	moduleName   string
	moduleLang   string
	sourceRoot   string
	sourceRole   string
	packagePath  string
	inTopFiles   bool
	priority     int
	normalizedID string
}

// Deduplicate surfaced file summaries by repo-relative path, then rebuild ownership rollups.
func deduplicateTopologyFiles(t *model.ProjectTopology) {
	winners := make(map[string]topologyFileCandidate)
	candidateGroups := make(map[string][]topologyFileCandidate)

	for moduleIdx := range t.Modules {
		module := &t.Modules[moduleIdx]
		for sourceRootIdx := range module.SourceRoots {
			sourceRoot := &module.SourceRoots[sourceRootIdx]
			for packageIdx := range sourceRoot.Packages {
				pkg := &sourceRoot.Packages[packageIdx]
				for _, file := range pkg.TopFiles {
					recordTopologyFileCandidate(winners, module, sourceRoot, pkg, file, true)
					normalizedPath := normalizeTopologyFilePath(file.Path)
					if normalizedPath != "" {
						candidateGroups[normalizedPath] = append(candidateGroups[normalizedPath], topologyFileCandidate{
							summary:     file,
							modulePath:  module.Path,
							moduleName:  module.Name,
							moduleLang:  module.Language,
							sourceRoot:  sourceRoot.Path,
							sourceRole:  sourceRoot.Role,
							packagePath: pkg.Path,
							inTopFiles:  true,
							priority:    topologyFilePriority(file),
						})
					}
				}
				for _, file := range pkg.AuxFiles {
					recordTopologyFileCandidate(winners, module, sourceRoot, pkg, file, false)
					normalizedPath := normalizeTopologyFilePath(file.Path)
					if normalizedPath != "" {
						candidateGroups[normalizedPath] = append(candidateGroups[normalizedPath], topologyFileCandidate{
							summary:     file,
							modulePath:  module.Path,
							moduleName:  module.Name,
							moduleLang:  module.Language,
							sourceRoot:  sourceRoot.Path,
							sourceRole:  sourceRoot.Role,
							packagePath: pkg.Path,
							inTopFiles:  false,
							priority:    topologyFilePriority(file),
						})
					}
				}
			}
		}
	}

	adjustTopologyOwnershipStats(t, candidateGroups, winners)
	pruneEmptyTopologyOwners(t)

	for moduleIdx := range t.Modules {
		module := &t.Modules[moduleIdx]
		module.TopFiles = nil
		module.AuxFiles = nil
		if !isFirstClassCppModule(module) {
			module.Heaviest = model.HeaviestFile{}
		}
		for sourceRootIdx := range module.SourceRoots {
			sourceRoot := &module.SourceRoots[sourceRootIdx]
			for packageIdx := range sourceRoot.Packages {
				pkg := &sourceRoot.Packages[packageIdx]
				pkg.TopFiles = nil
				pkg.AuxFiles = nil
				if !isFirstClassCppModule(module) || !isCppOwnedSourceRoot(sourceRoot) {
					pkg.Heaviest = model.HeaviestFile{}
				}
			}
		}
	}

	for _, winner := range winners {
		module := findModuleByCandidate(t.Modules, winner)
		if module == nil {
			continue
		}
		pkg := findPackageInModule(module, winner.sourceRoot, winner.sourceRole, winner.packagePath)
		if pkg == nil {
			continue
		}

		limit := 3
		sourceRoot := findSourceRootInModule(module, winner.sourceRoot, winner.sourceRole)
		if sourceRoot != nil {
			switch sourceRoot.Role {
			case "Documentation":
				limit = 5
			case "Web Resources":
				limit = 10
			case "Ops/Deploy":
				limit = maxOpsPackageFiles
			}
		}
		if strings.HasSuffix(strings.ToLower(winner.summary.Name), ".md") {
			limit = 5
		}
		pkgLimit := packageTopFileLimit(pkg.Path, limit)

		if winner.inTopFiles {
			pkg.TopFiles = addTopologyPackageTopFile(pkg.TopFiles, winner.summary, module, sourceRoot, pkgLimit)
		} else {
			pkg.AuxFiles = addTopologyAuxFile(pkg.AuxFiles, winner.summary, module, limit)
		}
	}

	for moduleIdx := range t.Modules {
		module := &t.Modules[moduleIdx]
		var cppOwnedTopFiles []model.FileSummary
		var cppContextTopFiles []model.FileSummary
		for sourceRootIdx := range module.SourceRoots {
			sourceRoot := &module.SourceRoots[sourceRootIdx]
			limit := 3
			switch sourceRoot.Role {
			case "Documentation":
				limit = 5
			case "Web Resources":
				limit = 10
			case "Ops/Deploy":
				limit = maxOpsPackageFiles
			}
			for packageIdx := range sourceRoot.Packages {
				pkg := &sourceRoot.Packages[packageIdx]
				// We don't easily know if the package contains MD here without re-scanning
				// but we can just use the sourceRoot's limit as a baseline.
				// Actually, we can check if any of the pkg.TopFiles are MD.
				pkgLimit := limit
				for _, f := range pkg.TopFiles {
					if strings.HasSuffix(strings.ToLower(f.Name), ".md") {
						pkgLimit = 5
						break
					}
				}

				if len(pkg.TopFiles) > 0 && (!isFirstClassCppModule(module) || !isCppOwnedSourceRoot(sourceRoot)) {
					pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
				}
				moduleLimit := moduleTopFileLimit(module.Path, pkgLimit)
				for _, file := range pkg.TopFiles {
					if isFirstClassCppModule(module) {
						if isCppOwnedTopologyFile(sourceRoot, file) {
							cppOwnedTopFiles = append(cppOwnedTopFiles, file)
						} else {
							cppContextTopFiles = addTopFile(cppContextTopFiles, file, moduleLimit)
						}
						continue
					}
					module.TopFiles = addTopologyTopFile(module.TopFiles, file, module, moduleLimit)
				}
				for _, file := range pkg.AuxFiles {
					module.AuxFiles = addTopologyAuxFile(module.AuxFiles, file, module, pkgLimit)
				}
			}
		}
		if isFirstClassCppModule(module) {
			module.TopFiles = append(selectCppModuleFiles(cppOwnedTopFiles, maxRootPackageTopFiles), cppContextTopFiles...)
		}
		if len(module.TopFiles) > 0 && !isFirstClassCppModule(module) {
			module.Heaviest = heaviestFromSummary(module.TopFiles[0])
		}
	}
}

func isCppOwnedTopologyFile(sourceRoot *model.SourceRoot, file model.FileSummary) bool {
	return isCppOwnedSourceRoot(sourceRoot) && isAcceptedCppFile(file.Name)
}

func addTopologyTopFile(files []model.FileSummary, file model.FileSummary, module *model.Module, limit int) []model.FileSummary {
	if isFirstClassCppModule(module) {
		return append(files, file)
	}
	if isFirstClassCSharpModule(module) {
		return addCSharpTopFile(files, file, maxRootPackageTopFiles)
	}
	if module != nil && module.Language == "Generic" {
		return addGenericTopFile(files, file, maxGenericPackageFiles)
	}
	return addTopFile(files, file, limit)
}

func addTopologyPackageTopFile(files []model.FileSummary, file model.FileSummary, module *model.Module, sourceRoot *model.SourceRoot, limit int) []model.FileSummary {
	if isFirstClassCppModule(module) && isCppOwnedSourceRoot(sourceRoot) {
		// The detector already applied its logical package cap. Retain both
		// physical members when final path de-duplication rebuilds ownership.
		return append(files, file)
	}
	if isFirstClassCSharpModule(module) {
		if normalizeRelativeDir(filepath.Dir(file.Path)) == module.Path {
			limit = maxRootPackageTopFiles
		}
		return addCSharpTopFile(files, file, limit)
	}
	if module != nil && module.Language == "Generic" {
		return addGenericTopFile(files, file, maxGenericPackageFiles)
	}
	return addTopFile(files, file, limit)
}

func addTopologyAuxFile(files []model.FileSummary, file model.FileSummary, module *model.Module, limit int) []model.FileSummary {
	if module != nil && module.Language == "Generic" {
		return addAuxFile(files, file, maxGenericPackageFiles)
	}
	return addAuxFile(files, file, limit)
}

func recordTopologyFileCandidate(winners map[string]topologyFileCandidate, module *model.Module, sourceRoot *model.SourceRoot, pkg *model.Package, file model.FileSummary, inTopFiles bool) {
	normalizedPath := normalizeTopologyFilePath(file.Path)
	if normalizedPath == "" {
		return
	}

	candidate := topologyFileCandidate{
		summary:      file,
		modulePath:   module.Path,
		moduleName:   module.Name,
		moduleLang:   module.Language,
		sourceRoot:   sourceRoot.Path,
		sourceRole:   sourceRoot.Role,
		packagePath:  pkg.Path,
		inTopFiles:   inTopFiles,
		priority:     topologyFilePriority(file),
		normalizedID: normalizedPath,
	}

	current, exists := winners[normalizedPath]
	if !exists || shouldReplaceTopologyFileCandidate(current, candidate) {
		winners[normalizedPath] = candidate
	}
}

func normalizeTopologyFilePath(path string) string {
	cleaned := filepath.Clean(path)
	if cleaned == "." {
		return ""
	}
	return cleaned
}

func shouldReplaceTopologyFileCandidate(current, candidate topologyFileCandidate) bool {
	if candidate.priority != current.priority {
		return candidate.priority > current.priority
	}
	if candidate.inTopFiles != current.inTopFiles {
		return candidate.inTopFiles
	}
	if candidate.summary.Size != current.summary.Size {
		return candidate.summary.Size > current.summary.Size
	}
	if topologyPackageSpecificity(candidate.packagePath) != topologyPackageSpecificity(current.packagePath) {
		return topologyPackageSpecificity(candidate.packagePath) > topologyPackageSpecificity(current.packagePath)
	}
	return topologyFileOwnerKey(candidate) < topologyFileOwnerKey(current)
}

func topologyFileOwnerKey(candidate topologyFileCandidate) string {
	return candidate.modulePath + "\x00" + candidate.moduleName + "\x00" + candidate.moduleLang + "\x00" + candidate.sourceRoot + "\x00" + candidate.sourceRole + "\x00" + candidate.packagePath + "\x00" + candidate.summary.Name + "\x00" + candidate.summary.Kind
}

func topologyPackageSpecificity(packagePath string) int {
	if packagePath == "" {
		return 0
	}
	return strings.Count(packagePath, string(filepath.Separator)) + 1
}

func sameTopologyFileCandidate(left, right topologyFileCandidate) bool {
	return left.modulePath == right.modulePath &&
		left.moduleName == right.moduleName &&
		left.moduleLang == right.moduleLang &&
		left.sourceRoot == right.sourceRoot &&
		left.sourceRole == right.sourceRole &&
		left.packagePath == right.packagePath &&
		left.inTopFiles == right.inTopFiles &&
		left.summary.Name == right.summary.Name &&
		left.summary.Path == right.summary.Path &&
		left.summary.Size == right.summary.Size &&
		left.summary.Kind == right.summary.Kind
}
