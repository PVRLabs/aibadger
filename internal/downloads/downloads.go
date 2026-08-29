// Package downloads resolves the current user's existing Downloads directory.
package downloads

import (
	"fmt"
	"os"
	"path/filepath"
)

// Resolve returns the user's existing Downloads directory. It never creates a
// directory or substitutes another destination when Downloads is unavailable.
func Resolve() (string, error) {
	return resolveDownloadsDirectory(nativeDownloadsDirectory, os.Stat)
}

func resolveDownloadsDirectory(pathFunc func() (string, error), statFunc func(string) (os.FileInfo, error)) (string, error) {
	path, err := pathFunc()
	if err != nil {
		return "", fmt.Errorf("resolve Downloads directory: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("resolve Downloads directory: resolver returned an empty path")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolve Downloads directory: resolver returned a non-absolute path %q", path)
	}

	info, err := statFunc(path)
	if err != nil {
		return "", fmt.Errorf("validate Downloads directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate Downloads directory %q: path is not a directory", path)
	}
	return path, nil
}
