package externalcontext

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/promptpolicy"
)

const ConfigFileName = ".badger-context"

// Load reads .badger-context from projectRoot and returns explicitly listed
// read-only context directories. Missing or empty files preserve current
// behavior by returning an empty slice.
func Load(projectRoot string) ([]model.ExternalContext, error) {
	configPath := filepath.Join(projectRoot, ConfigFileName)
	file, err := os.Open(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("Invalid .badger-context: failed to read config: %w", err)
	}
	defer file.Close()

	var contexts []model.ExternalContext
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		displayPath, ok, err := parseLine(projectRoot, scanner.Text())
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		absPath, realPath, err := validateDirectory(projectRoot, displayPath)
		if err != nil {
			return nil, err
		}
		if seen[realPath] {
			continue
		}
		seen[realPath] = true
		contexts = append(contexts, model.ExternalContext{
			Path:    displayPath,
			AbsPath: realPath,
			Top:     summarizeTop(absPath, displayPath),
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("Invalid .badger-context: failed to read config: %w", err)
	}
	return contexts, nil
}

func parseLine(projectRoot, line string) (string, bool, error) {
	path := strings.TrimSpace(line)
	if path == "" || strings.HasPrefix(path, "#") {
		return "", false, nil
	}
	if strings.Contains(path, "://") {
		return "", false, fmt.Errorf("Invalid .badger-context: invalid path format: %s", path)
	}
	if strings.ContainsAny(path, "*?[]") {
		return "", false, fmt.Errorf("Invalid .badger-context: invalid path format: %s", path)
	}

	clean := filepath.Clean(path)
	if clean == "." {
		return "", false, fmt.Errorf("Invalid .badger-context: invalid path format: %s", path)
	}
	if !filepath.IsAbs(clean) {
		if _, err := filepath.Rel(projectRoot, filepath.Join(projectRoot, clean)); err != nil {
			return "", false, fmt.Errorf("Invalid .badger-context: invalid path format: %s", path)
		}
	}
	return filepath.ToSlash(clean), true, nil
}

func validateDirectory(projectRoot, displayPath string) (string, string, error) {
	absPath := displayPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(projectRoot, filepath.FromSlash(displayPath))
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return "", "", fmt.Errorf("Invalid .badger-context: invalid path format: %s", displayPath)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", fmt.Errorf("Invalid .badger-context: path does not exist: %s", displayPath)
		}
		return "", "", fmt.Errorf("Invalid .badger-context: failed to inspect path %s: %w", displayPath, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("Invalid .badger-context: path is not a directory: %s", displayPath)
	}

	realPath := absPath
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		realPath = resolved
	}
	return absPath, realPath, nil
}

func summarizeTop(absPath, displayPath string) []model.ExternalContextItem {
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return nil
	}

	files := make([]model.ExternalContextItem, 0)
	dirs := make([]model.ExternalContextItem, 0)
	for _, entry := range entries {
		name := entry.Name()
		rel := filepath.ToSlash(filepath.Join(displayPath, name))
		if promptpolicy.IsSensitivePath(rel) {
			continue
		}
		if entry.IsDir() {
			if shouldSkipExternalSummaryDir(name) {
				continue
			}
			dirs = append(dirs, model.ExternalContextItem{Name: name, IsDir: true})
			continue
		}
		fullPath := filepath.Join(absPath, name)
		if shouldOmitExternalSummaryFile(absPath, fullPath, name) {
			continue
		}
		files = append(files, model.ExternalContextItem{Name: name})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name < dirs[j].Name })
	items := append(files, dirs...)
	if len(items) > 8 {
		return items[:8]
	}
	return items
}

func shouldSkipExternalSummaryDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "target", "build":
		return true
	default:
		return false
	}
}

func shouldOmitExternalSummaryFile(root, path, name string) bool {
	if isOmittedNoiseName(name) || isOmittedCompiledBinary(name) {
		return true
	}
	return isRootExtensionlessBinary(root, path, name)
}

func isOmittedNoiseName(name string) bool {
	switch strings.ToLower(name) {
	case ".ds_store", "thumbs.db",
		"go.sum", "package-lock.json", "pnpm-lock.yaml", "yarn.lock":
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
	if filepath.Ext(name) != "" {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || filepath.Dir(rel) != "." {
		return false
	}
	return filekind.Classify(path) == model.FileKindBinary
}

// ContainsFile reports whether requestPath resolves to an existing regular
// file under one of the configured external context directories.
func ContainsFile(projectRoot string, contexts []model.ExternalContext, requestPath string) (model.ExternalContext, string, bool) {
	if strings.TrimSpace(requestPath) == "" {
		return model.ExternalContext{}, "", false
	}

	// Strategy 1: Resolve from project root (handles absolute paths and relative paths with ..)
	absPath := requestPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(projectRoot, filepath.FromSlash(requestPath))
	}
	absPath, err := filepath.Abs(absPath)
	if err == nil {
		if realPath, err := filepath.EvalSymlinks(absPath); err == nil {
			if info, err := os.Stat(realPath); err == nil && !info.IsDir() {
				for _, ctx := range contexts {
					root := ctx.AbsPath
					if resolved, err := filepath.EvalSymlinks(root); err == nil {
						root = resolved
					}
					root = filepath.Clean(root)
					rel, err := filepath.Rel(root, realPath)
					if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
						if !shouldOmitExternalContextPath(root, realPath, rel) {
							return ctx, realPath, true
						}
					}
				}
			}
		}
	}

	// Strategy 2: Resolve relative to each external root (handles bare filenames)
	for _, ctx := range contexts {
		root := ctx.AbsPath
		fullPath := filepath.Join(root, filepath.FromSlash(requestPath))
		if realPath, err := filepath.EvalSymlinks(fullPath); err == nil {
			if info, err := os.Stat(realPath); err == nil && !info.IsDir() {
				// Re-verify containment in case of symlink trickery
				if resolvedRoot, err := filepath.EvalSymlinks(root); err == nil {
					root = resolvedRoot
				}
				root = filepath.Clean(root)
				rel, err := filepath.Rel(root, realPath)
				if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					if !shouldOmitExternalContextPath(root, realPath, rel) {
						return ctx, realPath, true
					}
				}
			}
		}
	}

	return model.ExternalContext{}, "", false
}

// IsOmittedPath reports whether a path within an external context root should
// be ignored based on existing privacy and noise rules.
func IsOmittedPath(root, absPath, relPath string) bool {
	if promptpolicy.IsSensitivePath(relPath) {
		return true
	}
	for _, part := range strings.Split(filepath.ToSlash(relPath), "/") {
		if shouldSkipExternalSummaryDir(part) {
			return true
		}
	}
	return shouldOmitExternalSummaryFile(root, absPath, filepath.Base(absPath))
}

func shouldOmitExternalContextPath(root, path, rel string) bool {
	return IsOmittedPath(root, path, rel)
}

// ContainsPath reports whether a planned patch path points at configured
// external context. It does not require the target file to exist.
func ContainsPath(projectRoot string, contexts []model.ExternalContext, requestPath string) bool {
	if strings.TrimSpace(requestPath) == "" {
		return false
	}
	absPath := requestPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(projectRoot, filepath.FromSlash(requestPath))
	}
	absPath, err := filepath.Abs(absPath)
	if err != nil {
		return false
	}
	for _, ctx := range contexts {
		root := filepath.Clean(ctx.AbsPath)
		rel, err := filepath.Rel(root, filepath.Clean(absPath))
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return true
	}
	return false
}
