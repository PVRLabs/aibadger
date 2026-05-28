package scanner

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

const maxGenericPackageFiles = 10

// GenericDetector handles projects with no specific structure (e.g., Python, simple Go, etc.)
type GenericDetector struct {
	Exclusions     map[string]bool
	maxFilesPerDir int
}

// NewGenericDetector creates a new GenericDetector.
func NewGenericDetector() *GenericDetector {
	return &GenericDetector{
		Exclusions: cloneExclusions(commonIgnoredDirs,
			".idea",
			".mypy_cache",
			".pytest_cache",
			".ruff_cache",
			".venv",
			".vscode",
			"__pycache__",
			"bin",
			"coverage",
			"dist",
			"vendor",
			"venv",
		),
	}
}

// Detect walks the directory and builds a single-module topology by grouping files into packages based on their directory.
func (d *GenericDetector) Detect(root string) ([]model.Module, error) {
	module := model.Module{
		Name:        filepath.Base(root),
		Path:        root,
		Language:    "Generic",
		SourceRoots: []model.SourceRoot{},
	}

	sourceRoot := model.SourceRoot{
		Path:     root,
		Packages: []model.Package{},
	}

	packages := make(map[string]*model.Package)
	extCounts := make(map[string]int)

	var perDirFiles map[string]int
	if d.maxFilesPerDir > 0 {
		perDirFiles = make(map[string]int)
	}
	totalProcessed := 0

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.maxFilesPerDir > 0 {
			dir := filepath.Dir(path)
			perDirFiles[dir]++
			if perDirFiles[dir] > d.maxFilesPerDir {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if entry.IsDir() {
			if shouldSkipDir(entry.Name(), d.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}

		totalProcessed++
		if totalProcessed > defaults.MaxTotalScanFiles {
			return filepath.SkipAll
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		if shouldOmitFile(root, path, entry.Name()) {
			return nil
		}
		recordGenericFile(root, path, entry.Name(), info.Size(), packages, extCounts)

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Guess language
	module.Language = d.guessLanguage(extCounts)

	// Convert packages map to slice
	for _, pkg := range packages {
		sourceRoot.Packages = append(sourceRoot.Packages, *pkg)
		sourceRoot.FileCount += pkg.FileCount
		module.FileCount += pkg.FileCount
		for _, file := range pkg.TopFiles {
			module.TopFiles = addGenericTopFile(module.TopFiles, file, maxGenericPackageFiles)
		}
		for _, file := range pkg.AuxFiles {
			module.AuxFiles = addAuxFile(module.AuxFiles, file, maxGenericPackageFiles)
		}
	}
	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}

	module.SourceRoots = append(module.SourceRoots, sourceRoot)

	return []model.Module{module}, nil
}

func recordGenericFile(root, path, name string, size int64, packages map[string]*model.Package, extCounts map[string]int) {
	relPath := relativePath(root, path)
	dir := relativeDirFromFile(root, path)
	pkg := getOrCreateGenericPackage(packages, dir)
	kind := filekind.Classify(path)

	ext := filepath.Ext(path)
	if ext != "" && kind == model.FileKindSource {
		extCounts[ext]++
	}

	pkg.FileCount++
	file := model.FileSummary{
		Name: name,
		Path: relPath,
		Size: size,
		Kind: kind,
	}
	if kind == model.FileKindAsset || kind == model.FileKindBinary {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, maxGenericPackageFiles)
	} else {
		pkg.TopFiles = addGenericTopFile(pkg.TopFiles, file, maxGenericPackageFiles)
	}
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

func addGenericTopFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		if genericTopFileRank(files[i]) != genericTopFileRank(files[j]) {
			return genericTopFileRank(files[i]) < genericTopFileRank(files[j])
		}
		if fileKindRank(files[i].Kind) != fileKindRank(files[j].Kind) {
			return fileKindRank(files[i].Kind) < fileKindRank(files[j].Kind)
		}
		if files[i].Size == files[j].Size {
			return files[i].Path < files[j].Path
		}
		return files[i].Size > files[j].Size
	})
	if len(files) > limit {
		return files[:limit]
	}
	return files
}

func genericTopFileRank(file model.FileSummary) int {
	if isTextControlFile(file.Name) || isConfigFileName(file.Name) {
		return 0
	}
	return 1
}

func getOrCreateGenericPackage(packages map[string]*model.Package, dir string) *model.Package {
	if pkg, ok := packages[dir]; ok {
		return pkg
	}

	pkg := &model.Package{
		Path:      dir,
		Name:      dir,
		FileCount: 0,
	}
	packages[dir] = pkg
	return pkg
}

// guessLanguage determines the primary language based on a frequency map of file extensions.
func (d *GenericDetector) guessLanguage(counts map[string]int) string {
	extToLang := map[string]string{
		".go":    "Go",
		".java":  "Java",
		".py":    "Python",
		".js":    "JavaScript",
		".ts":    "TypeScript",
		".cpp":   "C++",
		".c":     "C",
		".rs":    "Rust",
		".rb":    "Ruby",
		".php":   "PHP",
		".cs":    "C#",
		".kt":    "Kotlin",
		".swift": "Swift",
	}

	maxCount := 0
	lang := "Generic"
	for ext, count := range counts {
		if l, ok := extToLang[ext]; ok {
			if count > maxCount {
				maxCount = count
				lang = l
			}
		}
	}
	return lang
}
