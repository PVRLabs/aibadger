package filegroups

import (
	"path/filepath"
	"strings"
)

var (
	criticalGuidanceDocs = []string{
		"agents.md",
		"readme.md",
		"contributing.md",
		"claude.md",
		"gemini.md",
		"codex.md",
	}

	identityManifests = []string{
		"package.json",
		"go.mod",
		"pom.xml",
		"build.gradle",
		"build.gradle.kts",
		"pyproject.toml",
		"cargo.toml",
	}

	operationalConfigs = []string{
		"tsconfig.json",
		"jsconfig.json",
		"vite.config.js",
		"vite.config.ts",
		"vite.config.mjs",
		"vite.config.cjs",
		"next.config.js",
		"next.config.ts",
		"next.config.mjs",
		"next.config.cjs",
		"dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"makefile",
		"taskfile.yml",
		"taskfile.yaml",
		"justfile",
		"go.sum",
		"requirements.txt",
		"setup.py",
		"setup.cfg",
		"package.xml",
		"cmakelists.txt",
	}

	architectureLikeDocPrefixes = []string{
		"spec",
		"architecture",
		"design",
		"ui-spec",
	}
)

var rootWebResourceNames = []string{
	"sitemap.xml",
	"robots.txt",
	"manifest.json",
	"site.webmanifest",
	"favicon.ico",
	"apple-touch-icon.png",
	"favicon-32x32.png",
	"favicon-16x16.png",
	"browserconfig.xml",
	"og-image.png",
	"og-image.jpg",
	"social-preview.png",
	"social-preview.jpg",
	"social-card.png",
	"social-card.jpg",
	"opengraph.png",
	"opengraph.jpg",
}

func IsCriticalGuidanceDoc(base string) bool {
	return containsLowerName(criticalGuidanceDocs, base)
}

func IsRootWebResourceName(name string) bool {
	return containsLowerName(rootWebResourceNames, name)
}

func IsIdentityManifest(base string) bool {
	return containsLowerName(identityManifests, base)
}

func IsOperationalConfigFile(base string) bool {
	return containsLowerName(operationalConfigs, base)
}

func IsArchitectureLikeDoc(base string) bool {
	return hasAnyLowerPrefix(base, architectureLikeDocPrefixes)
}

func IsPlanningArtifactDoc(base string) bool {
	return strings.Contains(base, "log") ||
		strings.Contains(base, "journal") ||
		strings.HasPrefix(base, "plan")
}

func IsShallowDocumentationPath(lowerPath string) bool {
	isRoot := !strings.Contains(lowerPath, string(filepath.Separator))
	isDocs := (strings.HasPrefix(lowerPath, "docs"+string(filepath.Separator)) ||
		strings.HasPrefix(lowerPath, "doc"+string(filepath.Separator))) &&
		strings.Count(lowerPath, string(filepath.Separator)) == 1
	return isRoot || isDocs
}

func IsKnownStaticWebPath(lowerPath string) bool {
	segments := strings.Split(lowerPath, string(filepath.Separator))
	for idx, segment := range segments {
		if segment == "public" || segment == "static" || segment == "assets" {
			return true
		}
		if idx >= 4 &&
			segments[idx-4] == "src" &&
			segments[idx-3] == "main" &&
			segments[idx-2] == "resources" &&
			segment == "static" {
			return true
		}
	}
	return false
}

func containsLowerName(list []string, name string) bool {
	for _, candidate := range list {
		if candidate == name {
			return true
		}
	}
	return false
}

func hasAnyLowerPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
