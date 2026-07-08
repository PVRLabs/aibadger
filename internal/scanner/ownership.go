package scanner

// This file owns duplicate-loser count adjustment and owner pruning.

import "github.com/PVRLabs/aibadger/internal/model"

// Remove duplicate-loser ownership from counts before rebuilding capped summary slices.
func adjustTopologyOwnershipStats(t *model.ProjectTopology, candidateGroups map[string][]topologyFileCandidate, winners map[string]topologyFileCandidate) {
	for normalizedPath, candidates := range candidateGroups {
		if len(candidates) < 2 {
			continue
		}

		winner, ok := winners[normalizedPath]
		if !ok {
			continue
		}

		skippedWinner := false
		for _, candidate := range candidates {
			if !skippedWinner && sameTopologyFileCandidate(candidate, winner) {
				skippedWinner = true
				continue
			}
			decrementTopologyOwnershipStats(t, candidate)
		}
	}
}

// Decrement ownership counters for one duplicate loser without changing summary caps or winners.
func decrementTopologyOwnershipStats(t *model.ProjectTopology, candidate topologyFileCandidate) {
	module := findModuleByCandidate(t.Modules, candidate)
	if module == nil {
		return
	}
	pkg := findPackageInModule(module, candidate.sourceRoot, candidate.packagePath)
	if pkg != nil && pkg.FileCount > 0 {
		pkg.FileCount--
	}

	if isSyntheticOverviewCandidate(candidate) {
		return
	}

	if module.FileCount > 0 {
		module.FileCount--
	}
	if module.TotalBytes > 0 && candidate.summary.Kind != model.FileKindAsset && candidate.summary.Kind != model.FileKindBinary {
		module.TotalBytes -= candidate.summary.Size
		if module.TotalBytes < 0 {
			module.TotalBytes = 0
		}
	}

	sourceRoot := findSourceRootInModule(module, candidate.sourceRoot)
	if sourceRoot != nil && sourceRoot.FileCount > 0 {
		sourceRoot.FileCount--
	}
}

func isSyntheticOverviewCandidate(candidate topologyFileCandidate) bool {
	return candidate.packagePath == "" && candidate.sourceRoot != ""
}

func findModuleByPath(modules []model.Module, modulePath string) *model.Module {
	for idx := range modules {
		if modules[idx].Path == modulePath {
			return &modules[idx]
		}
	}
	return nil
}

func findModuleByCandidate(modules []model.Module, candidate topologyFileCandidate) *model.Module {
	for idx := range modules {
		if modules[idx].Path == candidate.modulePath &&
			modules[idx].Name == candidate.moduleName &&
			modules[idx].Language == candidate.moduleLang {
			return &modules[idx]
		}
	}
	if candidate.moduleName != "" || candidate.moduleLang != "" {
		return nil
	}
	return findModuleByPath(modules, candidate.modulePath)
}

func findSourceRootInModule(module *model.Module, sourceRootPath string) *model.SourceRoot {
	for sourceRootIdx := range module.SourceRoots {
		sourceRoot := &module.SourceRoots[sourceRootIdx]
		if sourceRoot.Path == sourceRootPath {
			return sourceRoot
		}
	}
	return nil
}

func pruneEmptyTopologyOwners(t *model.ProjectTopology) {
	for moduleIdx := range t.Modules {
		module := &t.Modules[moduleIdx]
		prunedSourceRoots := make([]model.SourceRoot, 0, len(module.SourceRoots))
		for _, sourceRoot := range module.SourceRoots {
			prunedPackages := make([]model.Package, 0, len(sourceRoot.Packages))
			for _, pkg := range sourceRoot.Packages {
				if pkg.FileCount == 0 {
					continue
				}
				prunedPackages = append(prunedPackages, pkg)
			}
			sourceRoot.Packages = prunedPackages
			if sourceRoot.FileCount == 0 {
				continue
			}
			prunedSourceRoots = append(prunedSourceRoots, sourceRoot)
		}
		module.SourceRoots = prunedSourceRoots
	}
}

func findPackageInModule(module *model.Module, sourceRootPath, packagePath string) *model.Package {
	for sourceRootIdx := range module.SourceRoots {
		sourceRoot := &module.SourceRoots[sourceRootIdx]
		if sourceRoot.Path != sourceRootPath {
			continue
		}
		for packageIdx := range sourceRoot.Packages {
			pkg := &sourceRoot.Packages[packageIdx]
			if pkg.Path == packagePath {
				return pkg
			}
		}
	}
	return nil
}
