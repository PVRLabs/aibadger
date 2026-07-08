package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

const maxGenericResourcePackageFiles = 5

// scanGenericResources finds schema-like resources outside language-specific
// source roots without depending on a language detector.
func scanGenericResources(root string) ([]model.SourceRoot, error) {
	packages := make(map[string]*model.Package)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if entry.IsDir() {
			if path != root && shouldSkipDir(entry.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			if path != root && shouldSkipGenericResourceDir(root, path, entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		name := entry.Name()
		if !isGenericResourceFile(name) || shouldOmitFile(root, path, name) {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		recordGenericResourceFile(root, path, name, info.Size(), packages)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return genericResourcePackagesToSourceRoots(packages), nil
}

func shouldSkipGenericResourceDir(root, path, name string) bool {
	relPath := relativePath(root, path)
	if relPath == "" {
		return false
	}
	if filegroups.IsOpsDirectoryPath(relPath) || isUnderTopLevelOpsDir(root, path) {
		return true
	}

	lowerName := strings.ToLower(name)
	if lowerName == "docs" || lowerName == "doc" || isStandaloneWebResourceDirName(lowerName) {
		return true
	}
	for _, dir := range standaloneWebResourceDirs() {
		if relPath == dir || strings.HasPrefix(relPath, dir+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func isGenericResourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sql", ".proto", ".graphql", ".gql":
		return true
	default:
		return false
	}
}

func recordGenericResourceFile(root, path, name string, size int64, packages map[string]*model.Package) {
	dir := relativeDirFromFile(root, path)
	pkg := getOrCreateGenericResourcePackage(packages, dir)
	kind := filekind.Classify(path)
	file := model.FileSummary{
		Name: name,
		Path: relativePath(root, path),
		Size: size,
		Kind: kind,
	}

	pkg.FileCount++
	if kind == model.FileKindAsset || kind == model.FileKindBinary {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, maxGenericResourcePackageFiles)
	} else {
		pkg.TopFiles = addTopFile(pkg.TopFiles, file, packageTopFileLimit(pkg.Path, maxGenericResourcePackageFiles))
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
	}
}

func getOrCreateGenericResourcePackage(packages map[string]*model.Package, dir string) *model.Package {
	if pkg, ok := packages[dir]; ok {
		return pkg
	}

	name := filepath.Base(dir)
	if dir == "" {
		name = "root"
	}
	pkg := &model.Package{
		Name: name,
		Path: dir,
	}
	packages[dir] = pkg
	return pkg
}

func genericResourcePackagesToSourceRoots(packages map[string]*model.Package) []model.SourceRoot {
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	sourceRoots := make([]model.SourceRoot, 0, len(paths))
	for _, path := range paths {
		pkg := *packages[path]
		sourceRoots = append(sourceRoots, model.SourceRoot{
			Path:      path,
			Role:      "Resources",
			FileCount: pkg.FileCount,
			Packages:  []model.Package{pkg},
		})
	}
	return sourceRoots
}

func attachGenericResourcesToTopology(topology *model.ProjectTopology, sourceRoots []model.SourceRoot) {
	if len(sourceRoots) == 0 || len(topology.Modules) == 0 {
		return
	}
	sourceRoots = filterExistingGenericResourceFiles(topology, sourceRoots)
	if len(sourceRoots) == 0 {
		return
	}
	module := docsTargetModule(topology.Modules)
	if module == nil {
		return
	}
	for _, sourceRoot := range sourceRoots {
		mergeGenericResourceSourceRoot(module, sourceRoot)
	}
}

func isStandaloneWebResourceDirName(name string) bool {
	for _, dir := range standaloneWebResourceDirs() {
		if name == dir {
			return true
		}
	}
	return false
}

func filterExistingGenericResourceFiles(topology *model.ProjectTopology, sourceRoots []model.SourceRoot) []model.SourceRoot {
	existing := surfacedTopologyFilePaths(topology)
	var filteredRoots []model.SourceRoot
	for _, sourceRoot := range sourceRoots {
		filteredPackages := make([]model.Package, 0, len(sourceRoot.Packages))
		sourceRoot.FileCount = 0
		for _, pkg := range sourceRoot.Packages {
			var topFiles []model.FileSummary
			var auxFiles []model.FileSummary
			for _, file := range pkg.TopFiles {
				if existing[normalizeTopologyFilePath(file.Path)] {
					continue
				}
				topFiles = addTopFile(topFiles, file, packageTopFileLimit(pkg.Path, maxGenericResourcePackageFiles))
			}
			for _, file := range pkg.AuxFiles {
				if existing[normalizeTopologyFilePath(file.Path)] {
					continue
				}
				auxFiles = addAuxFile(auxFiles, file, maxGenericResourcePackageFiles)
			}
			if len(topFiles) == 0 && len(auxFiles) == 0 {
				continue
			}
			pkg.TopFiles = topFiles
			pkg.AuxFiles = auxFiles
			pkg.FileCount = len(topFiles) + len(auxFiles)
			if len(pkg.TopFiles) > 0 {
				pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
			}
			sourceRoot.FileCount += pkg.FileCount
			filteredPackages = append(filteredPackages, pkg)
		}
		if sourceRoot.FileCount == 0 {
			continue
		}
		sourceRoot.Packages = filteredPackages
		filteredRoots = append(filteredRoots, sourceRoot)
	}
	return filteredRoots
}

func surfacedTopologyFilePaths(topology *model.ProjectTopology) map[string]bool {
	paths := make(map[string]bool)
	for _, module := range topology.Modules {
		for _, sourceRoot := range module.SourceRoots {
			for _, pkg := range sourceRoot.Packages {
				for _, file := range pkg.TopFiles {
					if path := normalizeTopologyFilePath(file.Path); path != "" {
						paths[path] = true
					}
				}
				for _, file := range pkg.AuxFiles {
					if path := normalizeTopologyFilePath(file.Path); path != "" {
						paths[path] = true
					}
				}
			}
		}
	}
	return paths
}

func mergeGenericResourceSourceRoot(module *model.Module, resourcesRoot model.SourceRoot) {
	if len(resourcesRoot.Packages) == 0 {
		return
	}
	for idx := range module.SourceRoots {
		if module.SourceRoots[idx].Path == resourcesRoot.Path {
			module.SourceRoots[idx].FileCount += resourcesRoot.FileCount
			mergeGenericResourcePackages(&module.SourceRoots[idx], resourcesRoot.Packages)
			return
		}
	}
	module.SourceRoots = append(module.SourceRoots, resourcesRoot)
}

func mergeGenericResourcePackages(sourceRoot *model.SourceRoot, packages []model.Package) {
	for _, resourcePackage := range packages {
		merged := false
		for idx := range sourceRoot.Packages {
			if sourceRoot.Packages[idx].Path != resourcePackage.Path {
				continue
			}
			sourceRoot.Packages[idx].FileCount += resourcePackage.FileCount
			for _, file := range resourcePackage.TopFiles {
				sourceRoot.Packages[idx].TopFiles = addTopFile(sourceRoot.Packages[idx].TopFiles, file, packageTopFileLimit(sourceRoot.Packages[idx].Path, maxGenericResourcePackageFiles))
			}
			for _, file := range resourcePackage.AuxFiles {
				sourceRoot.Packages[idx].AuxFiles = addAuxFile(sourceRoot.Packages[idx].AuxFiles, file, maxGenericResourcePackageFiles)
			}
			if len(sourceRoot.Packages[idx].TopFiles) > 0 {
				sourceRoot.Packages[idx].Heaviest = heaviestFromSummary(sourceRoot.Packages[idx].TopFiles[0])
			}
			merged = true
			break
		}
		if !merged {
			sourceRoot.Packages = append(sourceRoot.Packages, resourcePackage)
		}
	}
}
