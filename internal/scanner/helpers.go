package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

var commonIgnoredDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"target":       true,
	"build":        true,
}

const maxInitialMarkerDiscoveryDepth = 4

var markerDiscoveryIgnoredDirs = cloneExclusions(commonIgnoredDirs,
	"vendor",
	"dist",
	".gocache",
	".idea",
	".vscode",
)

func cloneExclusions(base map[string]bool, extras ...string) map[string]bool {
	cloned := make(map[string]bool, len(base)+len(extras))
	for name, skip := range base {
		cloned[name] = skip
	}
	for _, name := range extras {
		cloned[name] = true
	}
	return cloned
}

func shouldSkipDir(name string, exclusions map[string]bool) bool {
	return exclusions[name]
}

func discoverProjectMarkers(root string, markerNames ...string) ([]string, error) {
	if len(markerNames) == 0 {
		return nil, nil
	}

	markers := make(map[string]bool, len(markerNames))
	for _, name := range markerNames {
		markers[name] = true
	}

	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path != root && shouldSkipDir(d.Name(), markerDiscoveryIgnoredDirs) {
				return filepath.SkipDir
			}
			depth, ok := discoveryDepth(root, path)
			if !ok || depth > maxInitialMarkerDiscoveryDepth {
				return filepath.SkipDir
			}
			return nil
		}

		if !markers[d.Name()] {
			return nil
		}
		depth, ok := discoveryDepth(root, filepath.Dir(path))
		if !ok || depth > maxInitialMarkerDiscoveryDepth {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(paths)
	return paths, nil
}

func discoveryDepth(root, path string) (int, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0, false
	}
	if rel == "." || rel == "" {
		return 0, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return 0, false
	}
	return strings.Count(rel, string(filepath.Separator)) + 1, true
}

func shouldSkipTopLevelOpsDir(root, path, name string) bool {
	if filepath.Dir(path) != root {
		return false
	}
	return filegroups.IsOpsTopLevelDirName(name)
}

func shouldOmitFile(root, path, name string) bool {
	if promptpolicy.IsSensitivePath(relativePath(root, path)) {
		return true
	}
	if isOmittedNoiseName(name) || isOmittedCompiledBinary(name) {
		return true
	}
	return isRootExtensionlessBinary(root, path, name)
}

func isOmittedNoiseName(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db",
		"go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		".gitignore", ".dockerignore":
		return true
	default:
		return false
	}
}

func isOmittedCompiledBinary(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".exe", ".dll", ".so", ".dylib", ".class", ".jar", ".war":
		return true
	default:
		return false
	}
}

func isRootExtensionlessBinary(root, path, name string) bool {
	if filepath.Ext(name) != "" || isTextControlFile(name) {
		return false
	}
	return relativeDirFromFile(root, path) == "" && filekind.Classify(path) == model.FileKindBinary
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	// The scanners use "" as the project-root sentinel instead of ".".
	if err != nil || rel == "." {
		return ""
	}
	return rel
}

func relativeDirFromFile(root, path string) string {
	return normalizeRelativeDir(filepath.Dir(relativePath(root, path)))
}

func normalizeRelativeDir(path string) string {
	if path == "." {
		return ""
	}
	return path
}

func addTopFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		pi := topologyFilePriority(files[i])
		pj := topologyFilePriority(files[j])
		if pi != pj {
			return pi > pj
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

func addAuxFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sort.Slice(files, func(i, j int) bool {
		if auxFileKindRank(files[i].Kind) != auxFileKindRank(files[j].Kind) {
			return auxFileKindRank(files[i].Kind) < auxFileKindRank(files[j].Kind)
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

func fileKindRank(kind string) int {
	switch kind {
	case "", model.FileKindSource:
		return 0
	case model.FileKindAsset:
		return 1
	case model.FileKindBinary:
		return 2
	default:
		return 0
	}
}

func auxFileKindRank(kind string) int {
	switch kind {
	case model.FileKindBinary:
		return 0
	case model.FileKindAsset:
		return 1
	default:
		return 2
	}
}

func heaviestFromSummary(file model.FileSummary) model.HeaviestFile {
	return model.HeaviestFile{
		Name: file.Name,
		Path: file.Path,
		Size: file.Size,
		Kind: file.Kind,
	}
}
