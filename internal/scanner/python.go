package scanner

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

var errPythonEvidenceFound = errors.New("python evidence found")

// PythonDetector handles Python projects using shallow markers and common layouts.
type PythonDetector struct {
	Exclusions map[string]bool
}

// NewPythonDetector returns a new Python detector.
func NewPythonDetector() *PythonDetector {
	return &PythonDetector{
		Exclusions: cloneExclusions(commonIgnoredDirs,
			".mypy_cache",
			".pytest_cache",
			".venv",
			"__pycache__",
			"dist",
			"env",
			"venv",
		),
	}
}

// Detect creates a single additive Python module when Python markers or files exist.
func (p *PythonDetector) Detect(root string) ([]model.Module, error) {
	found, err := p.hasPythonEvidence(root)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	return []model.Module{p.analyzeModule(root)}, nil
}

func (p *PythonDetector) hasPythonEvidence(root string) (bool, error) {
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name(), p.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}
		if isPythonMarker(d.Name()) || isPythonSourceFile(d.Name()) {
			return errPythonEvidenceFound
		}
		return nil
	})
	if errors.Is(err, errPythonEvidenceFound) {
		return true, nil
	}
	return false, err
}

func (p *PythonDetector) analyzeModule(projectRoot string) model.Module {
	module := model.Module{
		Name:     filepath.Base(projectRoot),
		Path:     "",
		Language: "Python",
	}

	for _, rootName := range []string{"src", "tests"} {
		sr, totalBytes, ok := p.findSourceRoot(projectRoot, rootName, true)
		if !ok {
			continue
		}
		p.addSourceRoot(&module, sr, totalBytes)
	}

	if p.hasRootPythonFiles(projectRoot) {
		sr := model.SourceRoot{
			Path: "",
			Role: "Main Source",
		}
		totalBytes := p.scanSourceRoot(&sr, projectRoot, false)
		if sr.FileCount > 0 {
			p.addSourceRoot(&module, sr, totalBytes)
		}
	}

	for _, rootName := range p.shallowPackageDirs(projectRoot) {
		sr, totalBytes, ok := p.findSourceRoot(projectRoot, rootName, true)
		if !ok {
			continue
		}
		p.addSourceRoot(&module, sr, totalBytes)
	}

	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}
	return module
}

func (p *PythonDetector) addSourceRoot(module *model.Module, sr model.SourceRoot, totalBytes int64) {
	module.SourceRoots = append(module.SourceRoots, sr)
	module.FileCount += sr.FileCount
	module.TotalBytes += totalBytes
	for _, pkg := range sr.Packages {
		for _, file := range pkg.TopFiles {
			module.TopFiles = addPythonTopFile(module.TopFiles, file, 3)
		}
	}
}

func (p *PythonDetector) findSourceRoot(projectRoot, rootName string, recursive bool) (model.SourceRoot, int64, bool) {
	fullRootPath := filepath.Join(projectRoot, rootName)
	info, err := os.Stat(fullRootPath)
	if err != nil || !info.IsDir() {
		return model.SourceRoot{}, 0, false
	}

	sr := model.SourceRoot{
		Path: relativePath(projectRoot, fullRootPath),
		Role: p.inferSourceRole(rootName),
	}
	totalBytes := p.scanSourceRoot(&sr, projectRoot, recursive)
	if sr.FileCount == 0 {
		return model.SourceRoot{}, 0, false
	}
	return sr, totalBytes, true
}

func (p *PythonDetector) scanSourceRoot(sr *model.SourceRoot, projectRoot string, recursive bool) int64 {
	fullRootPath := filepath.Join(projectRoot, sr.Path)
	packageMap := make(map[string]*model.Package)
	var totalBytes int64

	filepath.WalkDir(fullRootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != fullRootPath && !recursive {
				return filepath.SkipDir
			}
			if path != fullRootPath && shouldSkipDir(d.Name(), p.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isPythonSourceFile(d.Name()) {
			return nil
		}
		if promptpolicy.IsSensitivePath(relativePath(projectRoot, path)) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		sr.FileCount++
		totalBytes += info.Size()
		recordPythonFile(packageMap, fullRootPath, projectRoot, path, d.Name(), info.Size())
		return nil
	})

	for _, pkg := range packageMap {
		sr.Packages = append(sr.Packages, *pkg)
	}
	sort.Slice(sr.Packages, func(i, j int) bool {
		return packageSortKey(sr.Packages[i]) < packageSortKey(sr.Packages[j])
	})
	return totalBytes
}

func (p *PythonDetector) hasRootPythonFiles(projectRoot string) bool {
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && isPythonSourceFile(entry.Name()) {
			return true
		}
	}
	return false
}

func (p *PythonDetector) shallowPackageDirs(projectRoot string) []string {
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		return nil
	}

	var dirs []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || name == "src" || name == "tests" || shouldSkipDir(name, p.Exclusions) {
			continue
		}
		if p.hasDirectPythonFile(filepath.Join(projectRoot, name)) {
			dirs = append(dirs, name)
		}
	}
	sort.Strings(dirs)
	return dirs
}

func (p *PythonDetector) hasDirectPythonFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && isPythonSourceFile(entry.Name()) {
			return true
		}
	}
	return false
}

func (p *PythonDetector) inferSourceRole(path string) string {
	if path == "tests" || strings.Contains(path, string(filepath.Separator)+"tests"+string(filepath.Separator)) {
		return "Test Source"
	}
	return "Main Source"
}

func isPythonMarker(name string) bool {
	switch strings.ToLower(name) {
	case "pyproject.toml", "setup.py", "setup.cfg", "requirements.txt", "pytest.ini", "tox.ini", "noxfile.py", "conftest.py":
		return true
	default:
		return false
	}
}

func isPythonSourceFile(name string) bool {
	return strings.ToLower(filepath.Ext(name)) == ".py"
}

func recordPythonFile(packageMap map[string]*model.Package, fullRootPath, projectRoot, path, name string, size int64) {
	pkgPath := filepath.Dir(path)
	relPkgPath := normalizeRelativeDir(relativePath(fullRootPath, pkgPath))
	relToProject := relativePath(projectRoot, path)

	pkg, exists := packageMap[relPkgPath]
	if !exists {
		pkg = &model.Package{
			Name: pythonPackageName(relPkgPath),
			Path: relativePath(projectRoot, pkgPath),
		}
		packageMap[relPkgPath] = pkg
	}

	pkg.FileCount++
	pkg.TopFiles = addPythonTopFile(pkg.TopFiles, model.FileSummary{
		Name: name,
		Path: relToProject,
		Size: size,
		Kind: model.FileKindSource,
	}, 3)
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

func pythonPackageName(relPkgPath string) string {
	if relPkgPath == "" {
		return "root"
	}
	return filepath.Base(relPkgPath)
}

func addPythonTopFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		if pythonFileRank(files[i].Name) != pythonFileRank(files[j].Name) {
			return pythonFileRank(files[i].Name) < pythonFileRank(files[j].Name)
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

func pythonFileRank(name string) int {
	lower := strings.ToLower(name)
	switch {
	case lower == "conftest.py":
		return 0
	case strings.HasPrefix(lower, "test_") && strings.HasSuffix(lower, ".py"):
		return 1
	case strings.HasSuffix(lower, "_test.py"):
		return 1
	case lower == "__init__.py":
		return 3
	default:
		return 2
	}
}
