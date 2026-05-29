package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/model"
)

const maxOpsPackageFiles = 5

// scanOpsResources finds shallow operational and deployment context without
// reading file contents or affecting language detector output.
func scanOpsResources(root string) ([]model.SourceRoot, error) {
	packages := make(map[string]*model.Package)

	if err := scanRootOpsFiles(root, packages); err != nil {
		return nil, err
	}
	if err := scanOpsDirectories(root, packages); err != nil {
		return nil, err
	}

	return opsPackagesToSourceRoots(packages), nil
}

func scanRootOpsFiles(root string, packages map[string]*model.Package) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !filegroups.IsRootOpsFileName(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		recordOpsFile(root, path, entry, packages)
	}
	return nil
}

func scanOpsDirectories(root string, packages map[string]*model.Package) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".github" {
			if err := scanOpsDir(root, filepath.Join(root, ".github", "workflows"), packages, false); err != nil {
				return err
			}
			continue
		}
		if !filegroups.IsOpsTopLevelDirName(name) {
			continue
		}
		if err := scanOpsDir(root, filepath.Join(root, name), packages, true); err != nil {
			return err
		}
	}
	return nil
}

func scanOpsDir(root, dir string, packages map[string]*model.Package, includeNested bool) error {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			if includeNested && !shouldSkipDir(entry.Name(), commonIgnoredDirs) {
				if err := scanOpsDirDirectFiles(root, path, packages); err != nil {
					return err
				}
			}
			continue
		}
		recordOpsFile(root, path, entry, packages)
	}
	return nil
}

func scanOpsDirDirectFiles(root, dir string, packages map[string]*model.Package) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		recordOpsFile(root, filepath.Join(dir, entry.Name()), entry, packages)
	}
	return nil
}

func recordOpsFile(root, path string, entry os.DirEntry, packages map[string]*model.Package) {
	name := entry.Name()
	if shouldOmitOpsFile(root, path, name) || shouldOmitOpsFileName(name) {
		return
	}
	if relativeDirFromFile(root, path) != "" && !filegroups.IsOpsContextFileName(name) {
		return
	}

	info, err := entry.Info()
	if err != nil {
		return
	}

	kind := opsFileKind(name)
	if kind == model.FileKindBinary || kind == model.FileKindAsset {
		return
	}

	dir := relativeDirFromFile(root, path)
	pkg := getOrCreateOpsPackage(packages, dir)
	file := model.FileSummary{
		Name: name,
		Path: relativePath(root, path),
		Size: info.Size(),
		Kind: kind,
	}

	pkg.FileCount++
	pkg.TopFiles = addOpsTopFile(pkg.TopFiles, file, packageTopFileLimit(pkg.Path, maxOpsPackageFiles))
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

func shouldOmitOpsFile(root, path, name string) bool {
	if filegroups.IsOpsEnvExampleName(name) {
		return isOmittedNoiseName(name) || isOmittedCompiledBinary(name)
	}
	return shouldOmitFile(root, path, name)
}

func getOrCreateOpsPackage(packages map[string]*model.Package, dir string) *model.Package {
	if pkg, ok := packages[dir]; ok {
		return pkg
	}

	name := dir
	if name == "" {
		name = "root"
	}
	pkg := &model.Package{
		Name: name,
		Path: dir,
	}
	packages[dir] = pkg
	return pkg
}

func opsPackagesToSourceRoots(packages map[string]*model.Package) []model.SourceRoot {
	sourceRoots := make([]model.SourceRoot, 0, len(packages))
	paths := make([]string, 0, len(packages))
	for path := range packages {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		pkg := *packages[path]
		sourceRoots = append(sourceRoots, model.SourceRoot{
			Path:      path,
			Role:      "Ops/Deploy",
			FileCount: pkg.FileCount,
			Packages:  []model.Package{pkg},
		})
	}
	return sourceRoots
}

func addOpsTopFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		ri := filegroups.OpsFileRank(files[i].Name)
		rj := filegroups.OpsFileRank(files[j].Name)
		if ri != rj {
			return ri < rj
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

func opsFileKind(name string) string {
	if isOpsBinaryName(name) {
		return model.FileKindBinary
	}
	if isOpsAssetName(name) {
		return model.FileKindAsset
	}
	return model.FileKindSource
}

func shouldOmitOpsFileName(name string) bool {
	lowerName := strings.ToLower(name)
	if strings.Contains(lowerName, ".generated.") || strings.Contains(lowerName, ".gen.") {
		return true
	}
	switch lowerName {
	case ".env", ".env.local", ".env.production", ".env.development", ".env.test":
		return true
	default:
		return false
	}
}

func isOpsBinaryName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".dll", ".so", ".dylib", ".class", ".jar", ".war", ".ear",
		".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar",
		".pdf", ".wasm", ".bin":
		return true
	default:
		return false
	}
}

func isOpsAssetName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico",
		".woff", ".woff2", ".ttf", ".otf", ".mp3", ".mp4", ".mov", ".webm":
		return true
	default:
		return false
	}
}
