package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

const (
	maxCppSourceDepth     = 6
	maxCppDetectorEntries = defaults.MaxTotalScanFiles / 4
)

var cppIgnoredDirs = cloneExclusions(markerDiscoveryIgnoredDirs,
	"out", "third-party", "generated", "gen", "coverage", ".cache",
)

var cppConventionalRoots = []struct {
	name string
	role string
}{
	{"src", "Main Source"},
	{"include", "Main Header"},
	{"test", "Test Source"},
	{"tests", "Test Source"},
}

// CppDetector recognizes only obvious, bounded root and conventional-tree C++ layouts.
// It deliberately does not parse build files or discover child modules.
type CppDetector struct {
	maxFilesPerDir      int
	maxEntries          int
	languageSourceCount int
}

// NewCppDetector returns a bounded C++ detector.
func NewCppDetector() *CppDetector {
	return &CppDetector{maxFilesPerDir: defaults.MaxFilesPerDirectory, maxEntries: maxCppDetectorEntries}
}

type cppCandidate struct {
	summary    model.FileSummary
	pkgPath    string
	sourceRoot string
	role       string
}

func (c *CppDetector) Detect(root string) ([]model.Module, error) {
	c.languageSourceCount = 0
	remaining := c.maxEntries
	if remaining <= 0 {
		remaining = maxCppDetectorEntries
	}
	candidates := make([]cppCandidate, 0)
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	candidates = append(candidates, c.scanDirect(root, root, "", "Main Source", entries, &remaining)...)
	for _, conventional := range cppConventionalRoots {
		path := filepath.Join(root, conventional.name)
		info, statErr := os.Stat(path)
		if statErr != nil || !info.IsDir() {
			continue
		}
		candidates = append(candidates, c.scanTree(root, path, conventional.name, conventional.role, &remaining)...)
	}
	for _, candidate := range candidates {
		if isCppActivationFile(candidate.summary.Path) {
			for _, accepted := range candidates {
				if isCppLanguageSourceFile(accepted.summary.Name) {
					c.languageSourceCount++
				}
			}
			return []model.Module{buildCppModule(root, candidates)}, nil
		}
	}
	return nil, nil
}

func isCppLanguageSourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cpp", ".cc", ".cxx":
		return true
	default:
		return false
	}
}

func (c *CppDetector) dirLimit() int {
	if c.maxFilesPerDir > 0 {
		return c.maxFilesPerDir
	}
	return defaults.MaxFilesPerDirectory
}

func (c *CppDetector) scanDirect(root, dir, sourceRoot, role string, entries []os.DirEntry, remaining *int) []cppCandidate {
	if len(entries) > c.dirLimit() {
		entries = entries[:c.dirLimit()]
	}
	var candidates []cppCandidate
	for _, entry := range entries {
		if *remaining <= 0 {
			break
		}
		*remaining--
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if !isAcceptedCppFile(entry.Name()) || shouldOmitFile(root, path, entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		candidates = append(candidates, cppCandidate{
			summary: model.FileSummary{Name: entry.Name(), Path: relativePath(root, path), Size: info.Size(), Kind: filekind.Classify(path)},
			pkgPath: relativeDirFromFile(root, path), sourceRoot: sourceRoot, role: role,
		})
	}
	return candidates
}

func (c *CppDetector) scanTree(root, treeRoot, sourceRoot, role string, remaining *int) []cppCandidate {
	var candidates []cppCandidate
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if *remaining <= 0 || depth > maxCppSourceDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		if len(entries) > c.dirLimit() {
			entries = entries[:c.dirLimit()]
		}
		for _, entry := range entries {
			if *remaining <= 0 {
				return
			}
			*remaining--
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				lower := strings.ToLower(entry.Name())
				if depth >= maxCppSourceDepth || shouldSkipDir(lower, cppIgnoredDirs) || strings.HasPrefix(lower, "cmake-build-") {
					continue
				}
				walk(path, depth+1)
				continue
			}
			if !isAcceptedCppFile(entry.Name()) || shouldOmitFile(root, path, entry.Name()) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			candidates = append(candidates, cppCandidate{
				summary: model.FileSummary{Name: entry.Name(), Path: relativePath(root, path), Size: info.Size(), Kind: filekind.Classify(path)},
				pkgPath: relativeDirFromFile(root, path), sourceRoot: sourceRoot, role: role,
			})
		}
	}
	walk(treeRoot, 0)
	return candidates
}

func isAcceptedCppFile(name string) bool {
	if strings.EqualFold(name, "CMakeLists.txt") {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cpp", ".cc", ".cxx", ".c", ".h", ".hpp", ".hh", ".hxx":
		return true
	default:
		return false
	}
}

func isCppActivationFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".cpp" && ext != ".cc" && ext != ".cxx" {
		return false
	}
	dir := normalizeRelativeDir(filepath.Dir(path))
	return dir == "" || dir == "src" || strings.HasPrefix(dir, "src"+string(filepath.Separator))
}

func buildCppModule(root string, candidates []cppCandidate) model.Module {
	module := model.Module{Name: filepath.Base(root), Language: "C++"}
	byRoot := make(map[string]map[string][]cppCandidate)
	rootRoles := make(map[string]string)
	for _, candidate := range candidates {
		if byRoot[candidate.sourceRoot] == nil {
			byRoot[candidate.sourceRoot] = make(map[string][]cppCandidate)
		}
		byRoot[candidate.sourceRoot][candidate.pkgPath] = append(byRoot[candidate.sourceRoot][candidate.pkgPath], candidate)
		rootRoles[candidate.sourceRoot] = candidate.role
		module.FileCount++
		module.TotalBytes += candidate.summary.Size
	}
	for _, rootPath := range []string{"", "src", "include", "test", "tests"} {
		packages := byRoot[rootPath]
		if len(packages) == 0 {
			continue
		}
		paths := make([]string, 0, len(packages))
		for path := range packages {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		sourceRoot := model.SourceRoot{Path: rootPath, Role: rootRoles[rootPath]}
		for _, path := range paths {
			files := make([]model.FileSummary, 0, len(packages[path]))
			for _, candidate := range packages[path] {
				files = append(files, candidate.summary)
			}
			pkg := model.Package{Name: cppPackageName(root, path), Path: path, FileCount: len(files)}
			pkg.TopFiles = selectCppPackageFiles(files, packageTopFileLimit(path, maxPackageTopFiles))
			pkg.Heaviest = heaviestCppFile(files)
			sourceRoot.FileCount += pkg.FileCount
			sourceRoot.Packages = append(sourceRoot.Packages, pkg)
			module.TopFiles = append(module.TopFiles, pkg.TopFiles...)
		}
		module.SourceRoots = append(module.SourceRoots, sourceRoot)
	}
	module.TopFiles = selectCppModuleFiles(module.TopFiles, maxRootPackageTopFiles)
	all := make([]model.FileSummary, 0, len(candidates))
	for _, candidate := range candidates {
		all = append(all, candidate.summary)
	}
	module.Heaviest = heaviestCppFile(all)
	return module
}

func cppPackageName(root, path string) string {
	if path == "" {
		return filepath.Base(root)
	}
	return filepath.Base(path)
}
