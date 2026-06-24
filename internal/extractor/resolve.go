package extractor

// This file resolves requested command paths to root-relative project paths.

import (
	"path/filepath"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/util"
)

func (e *Extractor) resolveCommandPath(path string) string {
	candidate := filepath.Join(e.ProjectRoot, path)
	if util.FileExists(candidate) && isWithinProjectRoot(e.ProjectRoot, candidate) {
		return path
	}

	if e.Topology == nil {
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
