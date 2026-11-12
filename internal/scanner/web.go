package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/dirpolicy"
	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

func standaloneWebResourceDirs() []string {
	return []string{"public", "static", "assets"}
}

func moduleWebResourceDirs() []string {
	return []string{"public", "static", "assets", filepath.Join("src", "main", "resources", "static")}
}

// isSharedWebResourcePath identifies layouts consumed by the repository-level
// web scanner. Module-relative layouts require a known web-owning module.
func isSharedWebResourcePath(path string) bool {
	for _, resourceDir := range standaloneWebResourceDirs() {
		if sameOrDescendantRelativePath(resourceDir, path) {
			return true
		}
	}
	return false
}

// scanWebResources finds root-level static web resources for the shared scanner path.
func scanWebResources(root string, directoryPolicies ...dirpolicy.Policy) ([]model.SourceRoot, error) {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	sourceRoots, err := scanWebResourceDirs(root, root, standaloneWebResourceDirs(), directoryPolicy)
	if err != nil {
		return nil, err
	}

	rootResources := scanKnownRootWebResources(root, directoryPolicy)
	if rootResources != nil {
		sourceRoots = append(sourceRoots, *rootResources)
	}

	return sourceRoots, nil
}

// scanModuleWebResources lets language detectors delegate known static resource directories.
func scanModuleWebResources(projectRoot, modulePath string, directoryPolicies ...dirpolicy.Policy) []model.SourceRoot {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	sourceRoots, err := scanWebResourceDirs(projectRoot, modulePath, moduleWebResourceDirs(), directoryPolicy)
	if err != nil {
		return nil
	}
	return sourceRoots
}

func scanWebResourceDirs(projectRoot, ownerRoot string, relDirs []string, directoryPolicies ...dirpolicy.Policy) ([]model.SourceRoot, error) {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	var sourceRoots []model.SourceRoot
	seen := make(map[string]bool, len(relDirs))

	for _, relDir := range relDirs {
		if relDir == "" || seen[relDir] {
			continue
		}
		seen[relDir] = true

		fullDir := filepath.Join(ownerRoot, relDir)
		info, err := os.Stat(fullDir)
		if err != nil || !info.IsDir() {
			continue
		}

		sourceRoot := model.SourceRoot{
			Path: relativePath(projectRoot, fullDir),
			Role: "Web Resources",
		}
		if err := scanWebSourceRoot(&sourceRoot, projectRoot, directoryPolicy); err != nil {
			return nil, err
		}
		if sourceRoot.FileCount == 0 {
			continue
		}
		sourceRoots = append(sourceRoots, sourceRoot)
	}

	sort.Slice(sourceRoots, func(i, j int) bool {
		return sourceRoots[i].Path < sourceRoots[j].Path
	})
	return sourceRoots, nil
}

func scanWebSourceRoot(sourceRoot *model.SourceRoot, projectRoot string, directoryPolicies ...dirpolicy.Policy) error {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	fullRootPath := filepath.Join(projectRoot, sourceRoot.Path)
	packages := make(map[string]*model.Package)

	err := walkDiscoveryDir(fullRootPath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path != fullRootPath && shouldSkipDir(entry.Name(), commonIgnoredDirs, directoryPolicy) {
				return filepath.SkipDir
			}
			return nil
		}

		info, infoErr := entry.Info()
		if infoErr != nil || shouldOmitFile(projectRoot, path, entry.Name(), directoryPolicy) {
			return nil
		}
		recordWebResourceFile(projectRoot, fullRootPath, path, entry.Name(), info.Size(), packages)
		sourceRoot.FileCount++
		return nil
	}, directoryPolicy, projectRoot)
	if err != nil {
		return err
	}

	for _, pkg := range packages {
		sourceRoot.Packages = append(sourceRoot.Packages, *pkg)
	}
	sort.Slice(sourceRoot.Packages, func(i, j int) bool {
		return sourceRoot.Packages[i].Path < sourceRoot.Packages[j].Path
	})
	return nil
}

func recordWebResourceFile(projectRoot, fullRootPath, path, name string, size int64, packages map[string]*model.Package) {
	pkgPath := filepath.Dir(path)
	relPkgPath := normalizeRelativeDir(relativePath(fullRootPath, pkgPath))
	packagePath := relativePath(projectRoot, pkgPath)

	pkg, exists := packages[packagePath]
	if !exists {
		pkg = &model.Package{
			Name:      webPackageName(relPkgPath, packagePath),
			Path:      packagePath,
			FileCount: 0,
		}
		packages[packagePath] = pkg
	}

	kind := filekind.Classify(path)
	file := model.FileSummary{
		Name: name,
		Path: relativePath(projectRoot, path),
		Size: size,
		Kind: kind,
	}
	pkg.FileCount++
	if kind == model.FileKindAsset || kind == model.FileKindBinary {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, 3)
	} else {
		pkg.TopFiles = addTopFile(pkg.TopFiles, file, packageTopFileLimit(packagePath, maxPackageTopFiles))
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
	}
}

func webPackageName(relPkgPath, packagePath string) string {
	if relPkgPath != "" {
		return filepath.Base(relPkgPath)
	}
	if packagePath == "" {
		return "root"
	}
	return filepath.Base(packagePath)
}

// scanKnownRootWebResources finds known standard web resource files at the project root
// (e.g., sitemap.xml, robots.txt, favicon.ico, web app manifests, social images).
func scanKnownRootWebResources(root string, directoryPolicies ...dirpolicy.Policy) *model.SourceRoot {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	entries, err := readDiscoveryDir(root, directoryPolicy, root)
	if err != nil {
		return nil
	}

	var files []model.FileSummary
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !filegroups.IsRootWebResourceName(strings.ToLower(name)) {
			continue
		}
		path := filepath.Join(root, name)
		if shouldOmitFile(root, path, name, directoryPolicy) {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		kind := filekind.Classify(path)
		files = append(files, model.FileSummary{
			Name: name,
			Path: relativePath(root, path),
			Size: info.Size(),
			Kind: kind,
		})
	}
	if len(files) == 0 {
		return nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	pkg := model.Package{
		Name:      "root",
		Path:      "",
		FileCount: len(files),
	}
	for _, file := range files {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, 10)
	}

	return &model.SourceRoot{
		Path:      "",
		Role:      "Web Resources",
		FileCount: len(files),
		Packages:  []model.Package{pkg},
	}
}

func attachWebResourcesToTopology(topology *model.ProjectTopology, sourceRoots []model.SourceRoot) {
	if len(sourceRoots) == 0 || len(topology.Modules) == 0 {
		return
	}
	module := docsTargetModule(topology.Modules)
	if module == nil {
		return
	}
	var missingRoots []model.SourceRoot
	for _, sourceRoot := range sourceRoots {
		if sourceRoot.Path == "" || !moduleHasSourceRootPath(module, sourceRoot.Path) {
			missingRoots = append(missingRoots, sourceRoot)
		}
	}
	mergeWebSourceRoots(module, missingRoots)
}

func mergeWebSourceRoots(module *model.Module, sourceRoots []model.SourceRoot) {
	for _, sourceRoot := range sourceRoots {
		mergeWebSourceRoot(module, sourceRoot)
	}
}

func mergeWebSourceRoot(module *model.Module, webRoot model.SourceRoot) {
	for idx := range module.SourceRoots {
		if module.SourceRoots[idx].Path == webRoot.Path && module.SourceRoots[idx].Role == webRoot.Role {
			if shouldKeepCppContextSourceRootSeparate(module, &module.SourceRoots[idx], webRoot) {
				continue
			}
			module.SourceRoots[idx].FileCount += webRoot.FileCount
			mergeWebPackages(&module.SourceRoots[idx], webRoot.Packages)
			return
		}
	}
	module.SourceRoots = append(module.SourceRoots, webRoot)
}

func moduleHasSourceRootPath(module *model.Module, sourceRootPath string) bool {
	for idx := range module.SourceRoots {
		if module.SourceRoots[idx].Path == sourceRootPath {
			return true
		}
	}
	return false
}

func mergeWebPackages(sourceRoot *model.SourceRoot, packages []model.Package) {
	for _, webPackage := range packages {
		merged := false
		for idx := range sourceRoot.Packages {
			if sourceRoot.Packages[idx].Path != webPackage.Path {
				continue
			}
			sourceRoot.Packages[idx].FileCount += webPackage.FileCount
			for _, file := range webPackage.TopFiles {
				sourceRoot.Packages[idx].TopFiles = addTopFile(sourceRoot.Packages[idx].TopFiles, file, packageTopFileLimit(sourceRoot.Packages[idx].Path, maxPackageTopFiles))
			}
			for _, file := range webPackage.AuxFiles {
				sourceRoot.Packages[idx].AuxFiles = addAuxFile(sourceRoot.Packages[idx].AuxFiles, file, 10)
			}
			if len(sourceRoot.Packages[idx].TopFiles) > 0 {
				sourceRoot.Packages[idx].Heaviest = heaviestFromSummary(sourceRoot.Packages[idx].TopFiles[0])
			}
			merged = true
			break
		}
		if !merged {
			sourceRoot.Packages = append(sourceRoot.Packages, webPackage)
		}
	}
}
