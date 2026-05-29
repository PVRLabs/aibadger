package scanner

// This file owns deterministic ordering for topology output.

import (
	"sort"

	"github.com/PVRLabs/aibadger/internal/model"
)

func sortTopology(t *model.ProjectTopology) {
	sort.SliceStable(t.Modules, func(i, j int) bool {
		return moduleSortKey(t.Modules[i]) < moduleSortKey(t.Modules[j])
	})
	for moduleIdx := range t.Modules {
		module := &t.Modules[moduleIdx]
		sortModuleFileSummaries(module)
		sort.SliceStable(module.SourceRoots, func(i, j int) bool {
			return sourceRootSortKey(module.SourceRoots[i]) < sourceRootSortKey(module.SourceRoots[j])
		})
		for sourceRootIdx := range module.SourceRoots {
			sourceRoot := &module.SourceRoots[sourceRootIdx]
			sort.SliceStable(sourceRoot.Packages, func(i, j int) bool {
				return packageSortKey(sourceRoot.Packages[i]) < packageSortKey(sourceRoot.Packages[j])
			})
			for packageIdx := range sourceRoot.Packages {
				sortPackageFileSummaries(module, &sourceRoot.Packages[packageIdx])
				sortAuxFileSummaries(sourceRoot.Packages[packageIdx].AuxFiles)
			}
		}
	}
}

func moduleSortKey(module model.Module) string {
	return module.Path + "\x00" + module.Name + "\x00" + module.Language
}

func sourceRootSortKey(sourceRoot model.SourceRoot) string {
	return sourceRoot.Path + "\x00" + sourceRoot.Role
}

func packageSortKey(pkg model.Package) string {
	return pkg.Path + "\x00" + pkg.Name
}

func sortModuleFileSummaries(module *model.Module) {
	if module.Language == "Python" {
		sortPythonFileSummaries(module.TopFiles)
		return
	}
	sortFileSummaries(module.TopFiles)
}

func sortPackageFileSummaries(module *model.Module, pkg *model.Package) {
	if module.Language == "Python" {
		sortPythonFileSummaries(pkg.TopFiles)
		return
	}
	if (module.Language == "JavaScript" || module.Language == "TypeScript") && pkg.Path == "" {
		sortNodeRootFileSummaries(pkg.TopFiles)
		return
	}
	sortFileSummaries(pkg.TopFiles)
}

func sortFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		pi := topologyFilePriority(files[i])
		pj := topologyFilePriority(files[j])
		if pi != pj {
			return pi > pj
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}

func sortPythonFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		if pythonFileRank(files[i].Name) != pythonFileRank(files[j].Name) {
			return pythonFileRank(files[i].Name) < pythonFileRank(files[j].Name)
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}

func sortNodeRootFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		pi := nodeOverviewPriority(files[i].Path)
		pj := nodeOverviewPriority(files[j].Path)
		if pi != pj {
			return pi < pj
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}

func sortAuxFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool {
		if auxFileKindRank(files[i].Kind) != auxFileKindRank(files[j].Kind) {
			return auxFileKindRank(files[i].Kind) < auxFileKindRank(files[j].Kind)
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
}
