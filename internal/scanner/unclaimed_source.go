package scanner

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/PVRLabs/aibadger/internal/model"
)

const genericSourceRole = "Generic Source"

// collectUnclaimedSource builds source-only coverage groups. Scanner
// orchestration intentionally does not call this until the integration phase.
func (d *GenericDetector) collectUnclaimedSource(root string, ownership semanticSourceOwnership) ([]model.Module, error) {
	packagesByLanguage := make(map[string]map[string]*model.Package)
	bytesByLanguage := make(map[string]int64)

	err := d.walkFiles(root, func(path string, entry os.DirEntry, info os.FileInfo) {
		language, ok := classifyGenericSourceCandidate(root, path)
		if !ok || ownership.Owns(path) {
			return
		}
		packages := packagesByLanguage[language]
		if packages == nil {
			packages = make(map[string]*model.Package)
			packagesByLanguage[language] = packages
		}
		dir := relativeDirFromFile(root, path)
		pkg := getOrCreateGenericPackage(packages, dir)
		file := model.FileSummary{Name: entry.Name(), Path: relativePath(root, path), Size: info.Size(), Kind: model.FileKindSource}
		pkg.FileCount++
		pkg.TopFiles = addGenericTopFile(pkg.TopFiles, file, maxGenericPackageFiles)
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
		bytesByLanguage[language] += info.Size()
	})
	if err != nil {
		return nil, err
	}

	languages := make([]string, 0, len(packagesByLanguage))
	for language := range packagesByLanguage {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	modules := make([]model.Module, 0, len(languages))
	for _, language := range languages {
		module := model.Module{Name: language + " Source", Path: "", Language: language, Coverage: true, TotalBytes: bytesByLanguage[language]}
		sourceRoot := model.SourceRoot{Path: "", Role: genericSourceRole}
		packagePaths := make([]string, 0, len(packagesByLanguage[language]))
		for path := range packagesByLanguage[language] {
			packagePaths = append(packagePaths, path)
		}
		sort.Strings(packagePaths)
		for _, path := range packagePaths {
			pkg := packagesByLanguage[language][path]
			sourceRoot.Packages = append(sourceRoot.Packages, *pkg)
			sourceRoot.FileCount += pkg.FileCount
			for _, file := range pkg.TopFiles {
				module.TopFiles = addGenericTopFile(module.TopFiles, file, maxGenericPackageFiles)
			}
		}
		module.FileCount = sourceRoot.FileCount
		if len(module.TopFiles) > 0 {
			module.Heaviest = heaviestFromSummary(module.TopFiles[0])
		}
		if module.FileCount > 0 {
			module.SourceRoots = []model.SourceRoot{sourceRoot}
			modules = append(modules, module)
		}
	}
	return modules, nil
}

func collectUnclaimedSource(root string, ownership semanticSourceOwnership) ([]model.Module, error) {
	return NewGenericDetector().collectUnclaimedSource(filepath.Clean(root), ownership)
}
