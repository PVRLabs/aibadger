package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

// JavaDetector handles Maven and Gradle projects.
type JavaDetector struct{}

// NewJavaDetector returns a new instance of JavaDetector.
func NewJavaDetector() *JavaDetector {
	return &JavaDetector{}
}

// Detect scans for Java modules and populates the topology.
func (j *JavaDetector) Detect(root string) ([]model.Module, error) {
	var modules []model.Module
	seenModules := make(map[string]bool)

	// Find all module markers (pom.xml, build.gradle, build.gradle.kts)
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

		name := d.Name()
		if name == "pom.xml" || name == "build.gradle" || name == "build.gradle.kts" {
			modulePath := filepath.Dir(path)
			relPath := relativePath(root, modulePath)
			if seenModules[relPath] {
				return nil
			}
			seenModules[relPath] = true

			module := j.analyzeModule(root, relPath)
			if module.FileCount > 0 {
				modules = append(modules, module)
			}
		}

		return nil
	})

	return modules, err
}

func (j *JavaDetector) analyzeModule(root, relPath string) model.Module {
	module := model.Module{
		Name:     filepath.Base(relPath),
		Path:     relPath,
		Language: "Java",
	}
	if module.Name == "." || module.Name == "" {
		module.Name = filepath.Base(root)
	}

	fullModulePath := filepath.Join(root, relPath)

	// Identify Source Roots
	roots := []string{
		"src/main/java",
		"src/test/java",
		"src/main",
		"src/java",
		"src",
	}

	var foundRoots []model.SourceRoot
	for _, r := range roots {
		// Probe a small set of conventional Java source-root layouts.
		if j.sourceRootCoveredBySelected(root, fullModulePath, r, foundRoots) {
			continue
		}
		sr, ok := j.findSourceRoot(root, fullModulePath, r)
		if ok {
			foundRoots = append(foundRoots, sr)
		}
	}
	module.SourceRoots = foundRoots

	// Calculate totals and heaviest file from source roots
	for _, sr := range foundRoots {
		module.FileCount += sr.FileCount
	}

	mergeWebSourceRoots(&module, scanModuleWebResources(root, fullModulePath))

	module.TopFiles = j.findTopFiles(fullModulePath, root, moduleTopFileLimit(module.Path, maxPackageTopFiles))
	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}

	return module
}

func (j *JavaDetector) sourceRootCoveredBySelected(projectRoot, modulePath, rootSuffix string, selected []model.SourceRoot) bool {
	candidate := filepath.Join(modulePath, rootSuffix)
	for _, sr := range selected {
		selectedPath := filepath.Join(projectRoot, sr.Path)
		rel, err := filepath.Rel(candidate, selectedPath)
		if err != nil || rel == "." {
			continue
		}
		if !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != ".." && !filepath.IsAbs(rel) {
			return true
		}
	}
	return false
}

func (j *JavaDetector) inferSourceRole(path string) string {
	if strings.Contains(path, "test") {
		return "Test Source"
	}
	return "Main Source"
}

func (j *JavaDetector) scanSourceRoot(sr *model.SourceRoot, projectRoot string) {
	fullRootPath := filepath.Join(projectRoot, sr.Path)
	packageMap := make(map[string]*model.Package)

	filepath.WalkDir(fullRootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		if strings.HasSuffix(d.Name(), ".java") {
			if promptpolicy.IsSensitivePath(relativePath(projectRoot, path)) {
				return nil
			}
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			sr.FileCount++
			recordJavaFile(packageMap, fullRootPath, projectRoot, path, d.Name(), info.Size())
		}
		return nil
	})

	for _, pkg := range packageMap {
		sr.Packages = append(sr.Packages, *pkg)
	}
}

func (j *JavaDetector) findHeaviest(modulePath, projectRoot string) model.HeaviestFile {
	topFiles := j.findTopFiles(modulePath, projectRoot, moduleTopFileLimit(relativePath(projectRoot, modulePath), maxPackageTopFiles))
	if len(topFiles) == 0 {
		return model.HeaviestFile{}
	}
	return heaviestFromSummary(topFiles[0])
}

func (j *JavaDetector) findTopFiles(modulePath, projectRoot string, limit int) []model.FileSummary {
	var topFiles []model.FileSummary
	filepath.WalkDir(modulePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}

		// Only consider primary source files
		if !strings.HasSuffix(d.Name(), ".java") {
			return nil
		}
		if promptpolicy.IsSensitivePath(relativePath(projectRoot, path)) {
			return nil
		}

		info, err := d.Info()
		if err != nil {
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

func (j *JavaDetector) findSourceRoot(projectRoot, modulePath, rootSuffix string) (model.SourceRoot, bool) {
	srPath := filepath.Join(modulePath, rootSuffix)
	info, err := os.Stat(srPath)
	if err != nil || !info.IsDir() {
		return model.SourceRoot{}, false
	}

	sr := model.SourceRoot{
		Path: relativePath(projectRoot, srPath),
		Role: j.inferSourceRole(rootSuffix),
	}
	j.scanSourceRoot(&sr, projectRoot)
	if sr.FileCount == 0 {
		return model.SourceRoot{}, false
	}
	return sr, true
}

func recordJavaFile(packageMap map[string]*model.Package, fullRootPath, projectRoot, path, name string, size int64) {
	pkgPath := filepath.Dir(path)
	relPkgPath := normalizeRelativeDir(relativePath(fullRootPath, pkgPath))
	relToProject := relativePath(projectRoot, path)

	pkg, exists := packageMap[relPkgPath]
	if !exists {
		pkg = &model.Package{
			Name:      javaPackageName(relPkgPath),
			Path:      relativePath(projectRoot, pkgPath),
			FileCount: 0,
		}
		packageMap[relPkgPath] = pkg
	}

	pkg.FileCount++
	limit := packageTopFileLimit(relativePath(projectRoot, pkgPath), maxPackageTopFiles)
	pkg.TopFiles = addTopFile(pkg.TopFiles, model.FileSummary{
		Name: name,
		Path: relToProject,
		Size: size,
	}, limit)
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

func javaPackageName(relPkgPath string) string {
	pkgName := strings.ReplaceAll(relPkgPath, string(os.PathSeparator), ".")
	if pkgName == "" {
		return "default"
	}
	return pkgName
}
