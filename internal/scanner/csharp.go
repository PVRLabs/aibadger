package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

const (
	maxCSharpSourceDepth    = 6
	maxCSharpProjectEntries = defaults.MaxTotalScanFiles / 4
)

var csharpIgnoredDirs = cloneExclusions(markerDiscoveryIgnoredDirs,
	"bin", "obj", ".vs", "packages", "testresults", "coverage",
)

// CSharpDetector handles projects rooted at .csproj files without evaluating
// project XML or invoking the .NET toolchain.
type CSharpDetector struct {
	maxFilesPerDir      int
	languageSourceCount int
}

// NewCSharpDetector returns a bounded C# detector.
func NewCSharpDetector() *CSharpDetector {
	return &CSharpDetector{maxFilesPerDir: defaults.MaxFilesPerDirectory}
}

// Detect discovers bounded .csproj markers and builds directory-backed C# topology.
func (c *CSharpDetector) Detect(root string) ([]model.Module, error) {
	return c.detectWithBudgets(root, defaults.MaxTotalScanFiles, maxCSharpProjectEntries)
}

func (c *CSharpDetector) detectWithBudgets(root string, totalBudget, projectBudget int) ([]model.Module, error) {
	c.languageSourceCount = 0
	markers, err := discoverProjectMarkers(root, ".csproj")
	if err != nil {
		return nil, err
	}

	markersByDir := make(map[string][]string)
	for _, marker := range markers {
		if isUnderIgnoredDir(root, marker, csharpIgnoredDirs) {
			continue
		}
		dir := filepath.Dir(marker)
		markersByDir[dir] = append(markersByDir[dir], marker)
	}
	dirs := make([]string, 0, len(markersByDir))
	for dir := range markersByDir {
		dirs = append(dirs, dir)
		sort.Strings(markersByDir[dir])
	}
	sort.Strings(dirs)

	remaining := totalBudget
	modules := make([]model.Module, 0, len(dirs))
	for _, dir := range dirs {
		projectRemaining := min(remaining, projectBudget)
		before := projectRemaining
		modules = append(modules, c.analyzeModule(root, dir, markersByDir[dir], &projectRemaining))
		remaining -= before - projectRemaining
	}
	return modules, nil
}

func (c *CSharpDetector) analyzeModule(projectRoot, moduleRoot string, markers []string, remaining *int) model.Module {
	relModule := relativePath(projectRoot, moduleRoot)
	name := filepath.Base(moduleRoot)
	if relModule == "" {
		name = filepath.Base(projectRoot)
	}
	module := model.Module{Name: name, Path: relModule, Language: "C#"}
	sourceRoot := model.SourceRoot{Path: relModule, Role: "Main Source"}
	packages := make(map[string]*model.Package)

	for _, marker := range markers {
		c.recordFile(projectRoot, moduleRoot, marker, false, packages, &sourceRoot, &module)
	}
	for _, solution := range colocatedSolutions(moduleRoot) {
		c.recordFile(projectRoot, moduleRoot, solution, false, packages, &sourceRoot, &module)
	}
	for _, settings := range colocatedAppSettings(moduleRoot) {
		c.recordFile(projectRoot, moduleRoot, settings, false, packages, &sourceRoot, &module)
	}
	c.scanModuleFiles(projectRoot, moduleRoot, packages, &sourceRoot, &module, remaining)

	packagePaths := make([]string, 0, len(packages))
	for path := range packages {
		packagePaths = append(packagePaths, path)
	}
	sort.Strings(packagePaths)
	for _, path := range packagePaths {
		sourceRoot.Packages = append(sourceRoot.Packages, *packages[path])
	}
	module.SourceRoots = []model.SourceRoot{sourceRoot}
	for _, pkg := range sourceRoot.Packages {
		for _, file := range pkg.TopFiles {
			module.TopFiles = addCSharpTopFile(module.TopFiles, file, maxRootPackageTopFiles)
		}
	}
	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}
	return module
}

func (c *CSharpDetector) scanModuleFiles(projectRoot, moduleRoot string, packages map[string]*model.Package, sourceRoot *model.SourceRoot, module *model.Module, remaining *int) {
	var walk func(string, int)
	walk = func(dir string, depth int) {
		if *remaining <= 0 || depth > maxCSharpSourceDepth {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		limit := c.maxFilesPerDir
		if limit <= 0 {
			limit = defaults.MaxFilesPerDirectory
		}
		if len(entries) > limit {
			entries = entries[:limit]
		}
		for _, entry := range entries {
			if *remaining <= 0 {
				return
			}
			*remaining--
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				if shouldSkipDir(strings.ToLower(entry.Name()), csharpIgnoredDirs) || depth >= maxCSharpSourceDepth || ownsCSharpProject(path) {
					continue
				}
				walk(path, depth+1)
				continue
			}
			isSource := strings.EqualFold(filepath.Ext(entry.Name()), ".cs")
			if (isSource || isCSharpCompanionFile(moduleRoot, path)) && !shouldOmitFile(projectRoot, path, entry.Name()) {
				c.recordFile(projectRoot, moduleRoot, path, isSource, packages, sourceRoot, module)
			}
		}
	}
	walk(moduleRoot, 0)
}

// isCSharpCompanionFile recognizes only conventional, module-relative C#
// application context. It deliberately does not infer project semantics.
func isCSharpCompanionFile(moduleRoot, path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".razor" || ext == ".cshtml" || ext == ".xaml" {
		return true
	}

	rel := strings.ToLower(relativePath(moduleRoot, path))
	return rel == filepath.Join("properties", "launchsettings.json") ||
		strings.HasPrefix(rel, "wwwroot"+string(filepath.Separator))
}

func (c *CSharpDetector) recordFile(projectRoot, moduleRoot, path string, languageSource bool, packages map[string]*model.Package, sourceRoot *model.SourceRoot, module *model.Module) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || promptpolicy.IsSensitivePath(relativePath(projectRoot, path)) {
		return
	}
	pkgPath := relativePath(projectRoot, filepath.Dir(path))
	pkg := packages[pkgPath]
	if pkg == nil {
		pkg = &model.Package{Name: filepath.Base(filepath.Dir(path)), Path: pkgPath}
		packages[pkgPath] = pkg
	}
	file := model.FileSummary{Name: filepath.Base(path), Path: relativePath(projectRoot, path), Size: info.Size(), Kind: filekind.Classify(path)}
	pkg.FileCount++
	if isUnderCSharpWWWRoot(moduleRoot, path) && (file.Kind == model.FileKindAsset || file.Kind == model.FileKindBinary) {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, maxPackageTopFiles)
	} else {
		pkg.TopFiles = addCSharpTopFile(pkg.TopFiles, file, packageTopFileLimit(relativePath(moduleRoot, filepath.Dir(path)), maxPackageTopFiles))
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
	}
	sourceRoot.FileCount++
	module.FileCount++
	module.TotalBytes += info.Size()
	if languageSource {
		c.languageSourceCount++
	}
}

func isUnderCSharpWWWRoot(moduleRoot, path string) bool {
	rel := strings.ToLower(relativePath(moduleRoot, path))
	return strings.HasPrefix(rel, "wwwroot"+string(filepath.Separator))
}

func colocatedSolutions(moduleRoot string) []string {
	entries, err := os.ReadDir(moduleRoot)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".sln") {
			paths = append(paths, filepath.Join(moduleRoot, entry.Name()))
		}
	}
	return paths
}

func colocatedAppSettings(moduleRoot string) []string {
	entries, err := os.ReadDir(moduleRoot)
	if err != nil {
		return nil
	}
	var paths []string
	for _, entry := range entries {
		if !entry.IsDir() && isAppSettingsFileName(entry.Name()) {
			paths = append(paths, filepath.Join(moduleRoot, entry.Name()))
		}
	}
	return paths
}

func ownsCSharpProject(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".csproj") {
			return true
		}
	}
	return false
}

func addCSharpTopFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		ri, rj := csharpFileRank(files[i].Name), csharpFileRank(files[j].Name)
		if ri != rj {
			return ri < rj
		}
		if files[i].Size != files[j].Size {
			return files[i].Size > files[j].Size
		}
		return files[i].Path < files[j].Path
	})
	if len(files) > limit {
		files = files[:limit]
	}
	return files
}

func csharpFileRank(name string) int {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".csproj") {
		return 0
	}
	switch lower {
	case "program.cs":
		return 1
	case "startup.cs":
		return 2
	case "globalusings.cs", "assemblyinfo.cs":
		return 4
	default:
		if isAppSettingsFileName(name) {
			return 3
		}
		switch strings.ToLower(filepath.Ext(name)) {
		case ".cs":
			return 5
		case ".razor", ".cshtml", ".xaml":
			return 6
		case ".sln":
			return 8
		default:
			return 7
		}
	}
}
