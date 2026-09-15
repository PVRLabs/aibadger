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

func heaviestCppFile(files []model.FileSummary) model.HeaviestFile {
	var heaviest model.FileSummary
	found := false
	for _, file := range files {
		if strings.EqualFold(file.Name, "CMakeLists.txt") {
			continue
		}
		if !found || file.Size > heaviest.Size || file.Size == heaviest.Size && file.Path < heaviest.Path {
			heaviest = file
			found = true
		}
	}
	if !found {
		return model.HeaviestFile{}
	}
	return heaviestFromSummary(heaviest)
}

type cppUnit struct{ files []model.FileSummary }

func selectCppPackageFiles(files []model.FileSummary, limit int) []model.FileSummary {
	return selectCppFiles(files, limit, false)
}
func selectCppModuleFiles(files []model.FileSummary, limit int) []model.FileSummary {
	return selectCppFiles(files, limit, true)
}

func selectCppFiles(files []model.FileSummary, limit int, mirrored bool) []model.FileSummary {
	files = uniqueCppFiles(files)
	pairs := cppPairs(files, mirrored)
	used := make(map[string]bool)
	units := make([]cppUnit, 0, len(files))
	for _, file := range files {
		if used[file.Path] {
			continue
		}
		unit := cppUnit{files: []model.FileSummary{file}}
		if other, ok := pairs[file.Path]; ok && !used[other] {
			for _, candidate := range files {
				if candidate.Path == other {
					unit.files = append(unit.files, candidate)
					break
				}
			}
		}
		for _, member := range unit.files {
			used[member.Path] = true
		}
		sortCppFileSummaries(unit.files)
		units = append(units, unit)
	}
	sort.SliceStable(units, func(i, j int) bool { return cppFileLess(units[i].files[0], units[j].files[0]) })
	if len(units) > limit {
		units = units[:limit]
	}
	selected := make([]model.FileSummary, 0, limit)
	for _, unit := range units {
		selected = append(selected, unit.files...)
	}
	sortCppFileSummaries(selected)
	return selected
}

func cppPairs(files []model.FileSummary, allowMirrored bool) map[string]string {
	pairs := make(map[string]string)
	used := make(map[string]bool)
	groupCppPairs(files, false, pairs, used)
	if allowMirrored {
		groupCppPairs(files, true, pairs, used)
	}
	return pairs
}

func groupCppPairs(files []model.FileSummary, mirrored bool, pairs map[string]string, used map[string]bool) {
	type sides struct{ headers, impls []model.FileSummary }
	groups := make(map[string]*sides)
	for _, file := range files {
		if used[file.Path] || !isCppPairFile(file.Name) {
			continue
		}
		key := filepath.Join(filepath.Dir(file.Path), cppBase(file.Name))
		if mirrored {
			parts := strings.Split(filepath.Clean(file.Path), string(filepath.Separator))
			if len(parts) < 2 || parts[0] != "src" && parts[0] != "include" {
				continue
			}
			key = filepath.Join(append(parts[1:len(parts)-1], cppBase(file.Name))...)
		}
		group := groups[key]
		if group == nil {
			group = &sides{}
			groups[key] = group
		}
		if isCppHeader(file.Name) {
			group.headers = append(group.headers, file)
		} else {
			group.impls = append(group.impls, file)
		}
	}
	for _, group := range groups {
		if len(group.headers) != 1 || len(group.impls) != 1 {
			continue
		}
		header, impl := group.headers[0], group.impls[0]
		pairs[header.Path], pairs[impl.Path] = impl.Path, header.Path
		used[header.Path], used[impl.Path] = true, true
	}
}

func isCppPairFile(name string) bool { return isCppHeader(name) || isCppImplementation(name) }
func isCppHeader(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".h", ".hpp", ".hh", ".hxx":
		return true
	default:
		return false
	}
}
func isCppImplementation(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".c", ".cc", ".cpp", ".cxx":
		return true
	default:
		return false
	}
}
func cppBase(name string) string { return strings.TrimSuffix(name, filepath.Ext(name)) }

func uniqueCppFiles(files []model.FileSummary) []model.FileSummary {
	byPath := make(map[string]model.FileSummary, len(files))
	for _, file := range files {
		byPath[file.Path] = file
	}
	unique := make([]model.FileSummary, 0, len(byPath))
	for _, file := range byPath {
		unique = append(unique, file)
	}
	sortCppFileSummaries(unique)
	return unique
}

func cppFileRank(file model.FileSummary) int {
	switch filepath.ToSlash(strings.ToLower(file.Path)) {
	case "cmakelists.txt":
		return 0
	case "main.cpp":
		return 1
	case "src/main.cpp":
		return 2
	default:
		return 3
	}
}
func cppFileLess(left, right model.FileSummary) bool {
	if cppFileRank(left) != cppFileRank(right) {
		return cppFileRank(left) < cppFileRank(right)
	}
	if topologyFilePriority(left) != topologyFilePriority(right) {
		return topologyFilePriority(left) > topologyFilePriority(right)
	}
	if left.Size != right.Size {
		return left.Size > right.Size
	}
	return left.Path < right.Path
}
func sortCppFileSummaries(files []model.FileSummary) {
	sort.SliceStable(files, func(i, j int) bool { return cppFileLess(files[i], files[j]) })
}

func sortCppModuleFileSummaries(module *model.Module) {
	ownedPaths := make(map[string]bool)
	for sourceRootIdx := range module.SourceRoots {
		sourceRoot := &module.SourceRoots[sourceRootIdx]
		if !isCppOwnedSourceRoot(sourceRoot) {
			continue
		}
		for _, pkg := range sourceRoot.Packages {
			for _, file := range pkg.TopFiles {
				ownedPaths[file.Path] = true
			}
		}
	}
	sort.SliceStable(module.TopFiles, func(i, j int) bool {
		left, right := module.TopFiles[i], module.TopFiles[j]
		leftRank, rightRank := 3, 3
		if ownedPaths[left.Path] {
			leftRank = cppFileRank(left)
		}
		if ownedPaths[right.Path] {
			rightRank = cppFileRank(right)
		}
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if topologyFilePriority(left) != topologyFilePriority(right) {
			return topologyFilePriority(left) > topologyFilePriority(right)
		}
		if left.Size != right.Size {
			return left.Size > right.Size
		}
		return left.Path < right.Path
	})
}
func isFirstClassCppModule(module *model.Module) bool {
	return module != nil && module.Language == "C++" && module.Path == "" && !module.Coverage
}

func isCppOwnedSourceRoot(sourceRoot *model.SourceRoot) bool {
	if sourceRoot == nil {
		return false
	}
	switch sourceRoot.Role {
	case "Main Source", "Main Header", "Test Source":
		return true
	default:
		return false
	}
}

func shouldKeepCppContextSourceRootSeparate(module *model.Module, existing *model.SourceRoot, incoming model.SourceRoot) bool {
	return isFirstClassCppModule(module) && isCppOwnedSourceRoot(existing) && !isCppOwnedSourceRoot(&incoming)
}
