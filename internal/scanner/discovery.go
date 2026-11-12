package scanner

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/PVRLabs/aibadger/internal/dirpolicy"
)

// Filter before entering subtrees or charging detector traversal budgets.
func walkDiscoveryDir(root string, visit fs.WalkDirFunc, policy dirpolicy.Policy, projectRoots ...string) error {
	projectRoot := root
	if len(projectRoots) > 0 {
		projectRoot = projectRoots[0]
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() && policy.SkipDirectoryPath(relativePath(projectRoot, path)) {
			return filepath.SkipDir
		}
		if err == nil && !entry.IsDir() && policy.SkipPath(relativePath(projectRoot, path)) {
			return nil
		}
		return visit(path, entry, err)
	})
}

func readDiscoveryDir(path string, policy dirpolicy.Policy, projectRoots ...string) ([]os.DirEntry, error) {
	if len(projectRoots) > 0 && policy.SkipDirectoryPath(relativePath(projectRoots[0], path)) {
		return nil, nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	kept := entries[:0]
	for _, entry := range entries {
		if !entry.IsDir() || !policy.Skip(entry.Name()) {
			kept = append(kept, entry)
		}
	}
	return kept, nil
}

func discoverProjectMarkers(root string, markers ...string) ([]string, error) {
	return discoverProjectMarkersWithPolicy(root, dirpolicy.Policy{}, markers...)
}
