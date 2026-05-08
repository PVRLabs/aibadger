package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

// GoDetector handles go.mod projects.
type GoDetector struct{}

// NewGoDetector returns a new Go detector.
func NewGoDetector() *GoDetector {
	return &GoDetector{}
}

// Detect scans all go.mod modules under root and builds Go package topology.
func (g *GoDetector) Detect(root string) ([]model.Module, error) {
	var modules []model.Module

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Name() != "go.mod" {
			return nil
		}

		modulePath := filepath.Dir(path)
		relPath := relativePath(root, modulePath)
		module := g.analyzeModule(root, relPath)
		if module.FileCount > 0 {
			modules = append(modules, module)
		}
		return nil
	})

	return modules, err
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

	module.TopFiles = g.findTopFiles(fullModulePath, projectRoot)
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
	topFiles := g.findTopFiles(modulePath, projectRoot)
	if len(topFiles) == 0 {
		return model.HeaviestFile{}
	}
	return heaviestFromSummary(topFiles[0])
}

func (g *GoDetector) findTopFiles(modulePath, projectRoot string) []model.FileSummary {
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
		}, 3)
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
	pkg.TopFiles = addTopFile(pkg.TopFiles, model.FileSummary{
		Name: name,
		Path: relToProject,
		Size: size,
	}, 3)
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
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
