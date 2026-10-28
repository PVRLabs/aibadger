package extractor

// This file resolves requested command paths to root-relative project paths.

import (
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/util"
)

// resolveFileCommandPath preserves literal files before recovering Markdown's
// escaped underscores. Recovery requires an exact local match, never a fuzzy one.
func (e *Extractor) resolveFileCommandPath(path string) string {
	if !strings.Contains(path, `\_`) {
		return e.resolveCommandPath(path)
	}
	candidate := filepath.Join(e.ProjectRoot, path)
	if !isWithinProjectRoot(e.ProjectRoot, candidate) {
		return ""
	}
	resolved, err := e.resolveExactCommandPath(path)
	if resolved != "" || err != nil {
		return resolved
	}
	repaired := strings.ReplaceAll(path, `\_`, `_`)
	if resolved, _ := e.resolveExactCommandPath(repaired); resolved != "" {
		return resolved
	}
	return e.resolveUnmatchedCommandPath(path)
}

func (e *Extractor) resolveCommandPath(path string) string {
	if resolved, _ := e.resolveExactCommandPath(path); resolved != "" {
		return resolved
	}
	return e.resolveUnmatchedCommandPath(path)
}

// resolveExactCommandPath is the shared exact lookup for ordinary and recovered
// selectors. Keep filesystem validation here so each candidate is probed once.
func (e *Extractor) resolveExactCommandPath(path string) (string, error) {
	candidate := filepath.Join(e.ProjectRoot, path)
	exists, err := util.FileExistsAndNotDir(candidate)
	if err != nil {
		return "", err
	}
	if exists && isWithinProjectRoot(e.ProjectRoot, candidate) {
		return path, nil
	}
	return "", nil
}

func (e *Extractor) resolveUnmatchedCommandPath(path string) string {
	if e.Topology == nil {
		return ""
	}

	if filepath.IsAbs(filepath.FromSlash(path)) || strings.HasPrefix(path, "..") {
		return ""
	}

	return e.resolveFuzzyPath(path)
}

func (e *Extractor) resolveFuzzyPath(target string) string {
	targetBase := filepath.Base(target)

	for _, m := range e.Topology.Modules {
		if matchedPath := matchModuleFile(e.ProjectRoot, m, targetBase); matchedPath != "" {
			return matchedPath
		}
	}
	return ""
}

func matchModuleFile(projectRoot string, module model.Module, targetBase string) string {
	// Prefer the module's heaviest file before probing source roots.
	if module.Heaviest.Name == targetBase {
		return module.Heaviest.Path
	}

	for _, sr := range module.SourceRoots {
		if matchedPath := matchSourceRootFile(projectRoot, sr, targetBase); matchedPath != "" {
			return matchedPath
		}
	}
	return ""
}

func matchSourceRootFile(projectRoot string, sourceRoot model.SourceRoot, targetBase string) string {
	potential := filepath.Join(sourceRoot.Path, targetBase)
	if util.FileExists(filepath.Join(projectRoot, potential)) {
		return potential
	}
	return ""
}
