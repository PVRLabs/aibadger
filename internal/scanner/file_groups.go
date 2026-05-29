package scanner

import (
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/model"
)

func isTextControlFile(name string) bool {
	switch strings.ToLower(name) {
	case "dockerfile",
		"makefile",
		"taskfile.yml",
		"justfile",
		"readme.md",
		"agents.md",
		"license":
		return true
	default:
		return filegroups.IsCriticalGuidanceDoc(strings.ToLower(name))
	}
}

func isConfigFileName(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".toml", ".yaml", ".yml", ".json", ".xml", ".ini", ".conf", ".properties":
		return true
	default:
		return false
	}
}

func isCriticalGuidanceDoc(base string) bool {
	return filegroups.IsCriticalGuidanceDoc(base)
}

func isIdentityManifest(base string) bool {
	return filegroups.IsIdentityManifest(base)
}

func isOperationalConfigFile(base string) bool {
	return filegroups.IsOperationalConfigFile(base)
}

func isArchitectureLikeDoc(base string) bool {
	return filegroups.IsArchitectureLikeDoc(base)
}

func isPlanningArtifactDoc(base string) bool {
	return filegroups.IsPlanningArtifactDoc(base)
}

func isRootWebResourceName(name string) bool {
	return filegroups.IsRootWebResourceName(name)
}

func isRootStaticSiteEntryPath(lowerPath, base string) bool {
	return filegroups.IsRootStaticSiteEntryPath(lowerPath, base)
}

func isShallowDocumentationPath(lowerPath string) bool {
	return filegroups.IsShallowDocumentationPath(lowerPath)
}

func isKnownStaticWebPath(lowerPath string) bool {
	return filegroups.IsKnownStaticWebPath(lowerPath)
}

func topologyFilePriority(file model.FileSummary) int {
	lowerPath := strings.ToLower(file.Path)
	base := strings.ToLower(filepath.Base(file.Path))
	ext := strings.ToLower(filepath.Ext(base))

	if isCriticalGuidanceDoc(base) {
		return 100
	}
	if isIdentityManifest(base) {
		return 98
	}
	if isOperationalConfigFile(base) {
		return 95
	}
	if priority := highSignalDocPriority(lowerPath, base); priority > 0 {
		return priority
	}
	if isRootStaticSiteEntryPath(lowerPath, base) {
		return 82
	}
	if isRootWebResourceName(base) {
		return 80
	}
	if isKnownStaticWebPath(lowerPath) {
		return 80
	}
	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return 70
	}
	if file.Kind == model.FileKindAsset {
		return 35
	}
	if file.Kind == model.FileKindBinary {
		return 30
	}
	return 60
}

func highSignalDocPriority(lowerPath, base string) int {
	if filepath.Ext(base) != ".md" {
		return 0
	}
	if strings.Contains(lowerPath, "archive"+string(filepath.Separator)) ||
		strings.HasSuffix(lowerPath, "archive") {
		return 0
	}
	if !isShallowDocumentationPath(lowerPath) {
		return 0
	}
	if isArchitectureLikeDoc(base) {
		return 92
	}
	if isPlanningArtifactDoc(base) {
		return 85
	}
	return 90
}
