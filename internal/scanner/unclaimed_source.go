package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

const genericSourceRole = "Generic Source"

// collectGenericAugmentation returns unclaimed source coverage and project-context
// candidates from the same bounded file walk.
func (d *GenericDetector) collectGenericAugmentation(root string, ownership semanticSourceOwnership) ([]model.Module, []projectContextCandidate, error) {
	packagesByLanguage := make(map[string]map[string]*model.Package)
	bytesByLanguage := make(map[string]int64)
	sourcesByLanguage := make(map[string]int)
	recordedPaths := make(map[string]bool)
	var contextCandidates []projectContextCandidate
	headerIndex := newCppCompanionHeaderIndex(root, ownership, d.maxFilesPerDir)

	err := d.walkFiles(root, func(path string, entry os.DirEntry, info os.FileInfo) {
		language, ok := classifyGenericSourceCandidate(root, path)
		if !ok {
			if candidate, contextOK := classifyProjectContextCandidate(root, path, info.Size()); contextOK {
				contextCandidates = append(contextCandidates, candidate)
			}
			return
		}
		if ownership.Owns(path) || ownership.ownsWebResource(path) {
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
		recordCoverageFile(pkg, file, language)
		recordedPaths[file.Path] = true
		bytesByLanguage[language] += info.Size()
		sourcesByLanguage[language]++

		if language == "C++" || language == "C" {
			if header, found := headerIndex.find(path); found && !recordedPaths[header.Path] {
				recordCoverageFile(pkg, header, language)
				recordedPaths[header.Path] = true
				bytesByLanguage[language] += header.Size
			}
		}
	})
	if err != nil {
		return nil, nil, err
	}

	languages := make([]string, 0, len(packagesByLanguage))
	for language := range packagesByLanguage {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	modules := make([]model.Module, 0, len(languages))
	for _, language := range languages {
		module := model.Module{Name: language + " Source", Path: "", Language: language, Coverage: true, TotalBytes: bytesByLanguage[language], LanguageSourceCount: sourcesByLanguage[language]}
		sourceRoot := model.SourceRoot{Path: "", Role: genericSourceRole}
		packagePaths := make([]string, 0, len(packagesByLanguage[language]))
		for path := range packagesByLanguage[language] {
			packagePaths = append(packagePaths, path)
		}
		sort.Strings(packagePaths)
		for _, path := range packagePaths {
			pkg := packagesByLanguage[language][path]
			if language == "C++" || language == "C" {
				pkg.TopFiles = selectCppPackageFiles(pkg.TopFiles, maxGenericPackageFiles)
				pkg.Heaviest = heaviestCppFile(pkg.TopFiles)
			}
			sourceRoot.Packages = append(sourceRoot.Packages, *pkg)
			sourceRoot.FileCount += pkg.FileCount
			for _, file := range pkg.TopFiles {
				if language == "C++" || language == "C" {
					module.TopFiles = append(module.TopFiles, file)
				} else {
					module.TopFiles = addGenericTopFile(module.TopFiles, file, maxGenericPackageFiles)
				}
			}
		}
		if language == "C++" || language == "C" {
			module.TopFiles = selectCppModuleFiles(module.TopFiles, maxGenericPackageFiles)
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
	return modules, contextCandidates, nil
}

func recordCoverageFile(pkg *model.Package, file model.FileSummary, language string) {
	pkg.FileCount++
	if language == "C++" || language == "C" {
		pkg.TopFiles = append(pkg.TopFiles, file)
		return
	}
	pkg.TopFiles = addGenericTopFile(pkg.TopFiles, file, maxGenericPackageFiles)
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

type cppCompanionHeaderIndex struct {
	projectRoot     string
	ownership       semanticSourceOwnership
	maxFilesPerDir  int
	genericExcluded map[string]bool
	byDirectory     map[string]map[string][]model.FileSummary
}

func newCppCompanionHeaderIndex(root string, ownership semanticSourceOwnership, maxFilesPerDir int) *cppCompanionHeaderIndex {
	genericExclusions := NewGenericDetector().Exclusions
	return &cppCompanionHeaderIndex{
		projectRoot:     root,
		ownership:       ownership,
		maxFilesPerDir:  maxFilesPerDir,
		genericExcluded: genericExclusions,
		byDirectory:     make(map[string]map[string][]model.FileSummary),
	}
}

func (i *cppCompanionHeaderIndex) find(implementation string) (model.FileSummary, bool) {
	dir := filepath.Clean(filepath.Dir(implementation))
	byStem, ok := i.byDirectory[dir]
	if !ok {
		byStem = i.indexDirectory(dir)
		i.byDirectory[dir] = byStem
	}
	stem := strings.TrimSuffix(filepath.Base(implementation), filepath.Ext(implementation))
	matches := byStem[stem]
	if len(matches) != 1 {
		return model.FileSummary{}, false
	}
	return matches[0], true
}

func (i *cppCompanionHeaderIndex) indexDirectory(dir string) map[string][]model.FileSummary {
	byStem := make(map[string][]model.FileSummary)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return byStem
	}
	// Generic coverage treats a positive setting as a per-directory entry
	// limit; zero or negative retains its existing unlimited behavior.
	if i.maxFilesPerDir > 0 && len(entries) > i.maxFilesPerDir {
		entries = entries[:i.maxFilesPerDir]
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !isCppHeader(name) {
			continue
		}
		path := filepath.Join(dir, name)
		if i.ownership.Owns(path) || shouldOmitFile(i.projectRoot, path, name) ||
			isUnderIgnoredDir(i.projectRoot, path, i.genericExcluded) || isUnderIgnoredDir(i.projectRoot, path, cppIgnoredDirs) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		file := model.FileSummary{Name: name, Path: relativePath(i.projectRoot, path), Size: info.Size(), Kind: filekind.Classify(path)}
		byStem[stem] = append(byStem[stem], file)
	}
	return byStem
}
