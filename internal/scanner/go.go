package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

// GoDetector handles go.mod projects.
type GoDetector struct{}

// NewGoDetector returns a new Go detector.
func NewGoDetector() *GoDetector {
	return &GoDetector{}
}

// Detect discovers Go project markers near root and builds Go package topology.
func (g *GoDetector) Detect(root string) ([]model.Module, error) {
	var modules []model.Module
	moduleRoots := make(map[string]bool)

	markers, err := discoverProjectMarkers(root, "go.mod", "go.work")
	if err != nil {
		return nil, err
	}

	for _, marker := range markers {
		switch filepath.Base(marker) {
		case "go.mod":
			moduleRoots[relativePath(root, filepath.Dir(marker))] = true
		case "go.work":
			for _, relPath := range g.workspaceModuleRoots(root, marker) {
				moduleRoots[relPath] = true
			}
		}
	}

	relPaths := make([]string, 0, len(moduleRoots))
	for relPath := range moduleRoots {
		relPaths = append(relPaths, relPath)
	}
	sort.Strings(relPaths)

	for _, relPath := range relPaths {
		module := g.analyzeModule(root, relPath)
		if module.FileCount > 0 {
			modules = append(modules, module)
		}
	}
	return modules, nil
}

func (g *GoDetector) workspaceModuleRoots(projectRoot, workFilePath string) []string {
	data, err := os.ReadFile(workFilePath)
	if err != nil {
		return nil
	}

	workDir := filepath.Dir(workFilePath)
	found := make(map[string]bool)
	inUseBlock := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(stripGoWorkLineComment(rawLine))
		if line == "" {
			continue
		}

		if inUseBlock {
			if line == ")" {
				inUseBlock = false
				continue
			}
			if strings.HasSuffix(line, ")") {
				inUseBlock = false
				line = strings.TrimSpace(strings.TrimSuffix(line, ")"))
			}
			if relPath, ok := goWorkModuleRelPath(projectRoot, workDir, line); ok {
				found[relPath] = true
			}
			continue
		}

		if !strings.HasPrefix(line, "use") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(line, "use"))
		if rest == "(" {
			inUseBlock = true
			continue
		}
		if strings.HasPrefix(rest, "(") {
			inUseBlock = true
			rest = strings.TrimSpace(strings.TrimPrefix(rest, "("))
		}
		if strings.HasSuffix(rest, ")") {
			inUseBlock = false
			rest = strings.TrimSpace(strings.TrimSuffix(rest, ")"))
		}
		if relPath, ok := goWorkModuleRelPath(projectRoot, workDir, rest); ok {
			found[relPath] = true
		}
	}

	roots := make([]string, 0, len(found))
	for relPath := range found {
		roots = append(roots, relPath)
	}
	sort.Strings(roots)
	return roots
}

func stripGoWorkLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func goWorkModuleRelPath(projectRoot, workDir, candidate string) (string, bool) {
	fields := strings.Fields(candidate)
	if len(fields) == 0 {
		return "", false
	}
	candidate = strings.Trim(fields[0], "\"'")
	if candidate == "" || filepath.IsAbs(candidate) {
		return "", false
	}

	cleaned := filepath.Clean(filepath.FromSlash(candidate))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", false
	}

	modulePath := filepath.Join(workDir, cleaned)
	if !hasGoMod(modulePath) {
		return "", false
	}
	relPath, err := filepath.Rel(projectRoot, modulePath)
	if err != nil || filepath.IsAbs(relPath) {
		return "", false
	}
	if relPath == "." {
		return "", true
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relPath, true
}

func hasGoMod(modulePath string) bool {
	info, err := os.Stat(filepath.Join(modulePath, "go.mod"))
	return err == nil && !info.IsDir()
}

func (g *GoDetector) analyzeModule(projectRoot, relPath string) model.Module {
	fullModulePath := filepath.Join(projectRoot, relPath)
	module := model.Module{
		Name:     g.moduleName(fullModulePath, relPath, projectRoot),
		Path:     relPath,
		Language: "Go",
	}

	for _, rootName := range []string{"cmd", "internal", "pkg"} {
		sr, totalBytes, ok := g.findSourceRoot(projectRoot, fullModulePath, rootName)
		if ok {
			module.SourceRoots = append(module.SourceRoots, sr)
			module.FileCount += sr.FileCount
			module.TotalBytes += totalBytes
		}
	}

	if g.hasRootGoFiles(fullModulePath) {
		sr := model.SourceRoot{
			Path: relativePath(projectRoot, fullModulePath),
			Role: "Main Source",
		}
		totalBytes := g.scanSourceRoot(&sr, projectRoot, false)
		if sr.FileCount > 0 {
			module.SourceRoots = append(module.SourceRoots, sr)
			module.FileCount += sr.FileCount
			module.TotalBytes += totalBytes
		}
	}

	mergeWebSourceRoots(&module, scanModuleWebResources(projectRoot, fullModulePath))

	module.TopFiles = g.findTopFiles(fullModulePath, projectRoot, moduleTopFileLimit(module.Path, maxPackageTopFiles))
	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}
	return module
}

func (g *GoDetector) moduleName(modulePath, relPath, projectRoot string) string {
	name := goModulePath(filepath.Join(modulePath, "go.mod"))
	if name != "" {
		return name
	}
	if relPath == "" {
		return filepath.Base(projectRoot)
	}
	return filepath.Base(relPath)
}

func (g *GoDetector) findSourceRoot(projectRoot, modulePath, rootName string) (model.SourceRoot, int64, bool) {
	fullRootPath := filepath.Join(modulePath, rootName)
	info, err := os.Stat(fullRootPath)
	if err != nil || !info.IsDir() {
		return model.SourceRoot{}, 0, false
	}

	sr := model.SourceRoot{
		Path: relativePath(projectRoot, fullRootPath),
		Role: g.inferSourceRole(rootName),
	}
	totalBytes := g.scanSourceRoot(&sr, projectRoot, true)
	if sr.FileCount == 0 {
		return model.SourceRoot{}, 0, false
	}
	return sr, totalBytes, true
}

func (g *GoDetector) scanSourceRoot(sr *model.SourceRoot, projectRoot string, recursive bool) int64 {
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
			if path != fullRootPath && shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isGoSourceFile(d.Name()) && !isGoResourceFile(d.Name()) {
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
		recordGoFile(packageMap, fullRootPath, projectRoot, path, d.Name(), info.Size())
		return nil
	})

	for _, pkg := range packageMap {
		sr.Packages = append(sr.Packages, *pkg)
	}
	return totalBytes
}

func (g *GoDetector) hasRootGoFiles(modulePath string) bool {
	entries, err := os.ReadDir(modulePath)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() && isGoSourceFile(entry.Name()) {
			return true
		}
	}
	return false
}

func (g *GoDetector) inferSourceRole(path string) string {
	pathLower := strings.ToLower(path)
	if pathLower == "cmd" || strings.HasPrefix(pathLower, "cmd"+string(os.PathSeparator)) {
		return "Entry Point"
	}
	if pathLower == "internal" || strings.HasPrefix(pathLower, "internal"+string(os.PathSeparator)) {
		return "Internal Source"
	}
	if pathLower == "pkg" || strings.HasPrefix(pathLower, "pkg"+string(os.PathSeparator)) {
		return "Library Source"
	}
	return "Main Source"
}

func (g *GoDetector) findHeaviest(modulePath, projectRoot string) model.HeaviestFile {
	topFiles := g.findTopFiles(modulePath, projectRoot, moduleTopFileLimit(relativePath(projectRoot, modulePath), maxPackageTopFiles))
	if len(topFiles) == 0 {
		return model.HeaviestFile{}
	}
	return heaviestFromSummary(topFiles[0])
}

func (g *GoDetector) findTopFiles(modulePath, projectRoot string, limit int) []model.FileSummary {
	var topFiles []model.FileSummary
	filepath.WalkDir(modulePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isGoSourceFile(d.Name()) {
			return nil
		}
		if promptpolicy.IsSensitivePath(relativePath(projectRoot, path)) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		topFiles = addTopFile(topFiles, model.FileSummary{
			Name: d.Name(),
			Path: relativePath(projectRoot, path),
			Size: info.Size(),
		}, limit)
		return nil
	})
	return topFiles
}

func recordGoFile(packageMap map[string]*model.Package, fullRootPath, projectRoot, path, name string, size int64) {
	pkgPath := filepath.Dir(path)
	relPkgPath := normalizeRelativeDir(relativePath(fullRootPath, pkgPath))
	relToProject := relativePath(projectRoot, path)

	pkg, exists := packageMap[relPkgPath]
	if !exists {
		pkg = &model.Package{
			Name: goPackageName(relPkgPath),
			Path: relativePath(projectRoot, pkgPath),
		}
		packageMap[relPkgPath] = pkg
	}

	pkg.FileCount++
	limit := packageTopFileLimit(relativePath(projectRoot, pkgPath), maxPackageTopFiles)
	kind := filekind.Classify(path)
	file := model.FileSummary{
		Name: name,
		Path: relToProject,
		Size: size,
		Kind: kind,
	}
	if kind == model.FileKindAsset || kind == model.FileKindBinary {
		pkg.AuxFiles = addAuxFile(pkg.AuxFiles, file, limit)
	} else {
		pkg.TopFiles = addTopFile(pkg.TopFiles, file, limit)
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
	}
}

func goPackageName(relPkgPath string) string {
	if relPkgPath == "" {
		return "root"
	}
	return filepath.Base(relPkgPath)
}

func goModulePath(goModPath string) string {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return ""
	}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func isGoSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go")
}

func isGoResourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".sql", ".yaml", ".yml", ".html", ".htm",
		".tmpl", ".gotmpl", ".tpl",
		".proto", ".json", ".xml", ".txt",
		".png":
		return true
	}
	return false
}
