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

	opsTopLevelDirs = []string{
		"deploy",
		"deployment",
		"deployments",
		"ops",
		"infra",
		"infrastructure",
		"ci",
		"build-scripts",
		"automation",
		"provision",
		"provisioning",
		"release",
		"releases",
		"packaging",
		"dist-scripts",
		"docker",
		"containers",
		"k8s",
		"kubernetes",
		"helm",
		"terraform",
		"tofu",
		"opentofu",
		"ansible",
		"puppet",
		"chef",
		"salt",
		"packer",
		"nomad",
		"systemd",
		"supervisor",
		"nginx",
		"apache",
		"cloud",
		"aws",
		"gcp",
		"azure",
		".gitlab",
		".circleci",
		".buildkite",
	}

	rootOpsFileNames = []string{
		"makefile",
		"dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		"taskfile.yml",
		"taskfile.yaml",
		"justfile",
		".gitlab-ci.yml",
		".gitlab-ci.yaml",
	}

	opsContextFileNames = []string{
		"dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		"makefile",
		"taskfile.yml",
		"taskfile.yaml",
		"justfile",
		"flake.nix",
		"shell.nix",
	}

	opsContextExtensions = []string{
		".sh",
		".bash",
		".zsh",
		".fish",
		".ps1",
		".bat",
		".cmd",
		".py",
		".rb",
		".pl",
		".js",
		".ts",
		".mjs",
		".cjs",
		".sql",
		".service",
		".timer",
		".yml",
		".yaml",
		".json",
		".toml",
		".tf",
		".tfvars",
		".hcl",
		".ini",
		".conf",
		".properties",
	}

	opsDocKeywords = []string{
		"readme",
		"manual",
		"runbook",
		"deploy",
		"deployment",
		"provision",
		"release",
		"ops",
		"infra",
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

func IsRootStaticSiteEntryPath(lowerPath, base string) bool {
	return base == "index.html" && !strings.Contains(lowerPath, string(filepath.Separator))
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

func IsOpsTopLevelDirName(name string) bool {
	return containsLowerName(opsTopLevelDirs, normalizeName(name))
}

func IsOpsDirectoryPath(path string) bool {
	lowerPath := normalizePath(path)
	if lowerPath == "" || lowerPath == "." {
		return false
	}
	segments := strings.Split(lowerPath, string(filepath.Separator))
	if len(segments) == 1 {
		return IsOpsTopLevelDirName(segments[0])
	}
	return len(segments) == 2 && segments[0] == ".github" && segments[1] == "workflows"
}

func IsRootOpsFileName(name string) bool {
	lowerName := normalizeName(name)
	if IsOpsEnvExampleName(lowerName) || containsLowerName(rootOpsFileNames, lowerName) {
		return true
	}
	return hasAnyLowerSuffix(lowerName, []string{".sh", ".bash", ".zsh", ".fish", ".ps1", ".bat", ".cmd"})
}

func IsOpsEnvExampleName(name string) bool {
	switch normalizeName(name) {
	case ".env.example", ".env.template", ".env.sample":
		return true
	default:
		return false
	}
}

func IsOpsContextFileName(name string) bool {
	lowerName := normalizeName(name)
	if IsOpsEnvExampleName(lowerName) || containsLowerName(opsContextFileNames, lowerName) {
		return true
	}
	if strings.HasSuffix(lowerName, ".md") {
		return hasAnyLowerSubstring(lowerName, opsDocKeywords)
	}
	return hasAnyLowerSuffix(lowerName, opsContextExtensions)
}

func OpsFileRank(name string) int {
	lowerName := normalizeName(name)
	switch {
	case lowerName == "readme.md":
		return 0
	case IsOpsEnvExampleName(lowerName):
		return 1
	case containsLowerName(rootOpsFileNames, lowerName) || containsLowerName(opsContextFileNames, lowerName):
		return 2
	case strings.HasSuffix(lowerName, ".md") && hasAnyLowerSubstring(lowerName, []string{"runbook", "manual", "deploy", "deployment", "provision", "release"}):
		return 3
	case hasAnyLowerSubstring(lowerName, []string{"deploy", "deployment", "provision", "release", "start", "stop", "restart", "run", "diagnose", "check", "health", "status", "backup", "restore", "import", "export", "migration", "schema", "seed"}):
		return 4
	case IsOpsContextFileName(lowerName):
		return 5
	default:
		return 100
	}
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

func hasAnyLowerSuffix(name string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func hasAnyLowerSubstring(name string, substrings []string) bool {
	for _, substring := range substrings {
		if strings.Contains(name, substring) {
			return true
		}
	}
	return false
}

func normalizeName(name string) string {
	return strings.ToLower(filepath.Base(filepath.Clean(name)))
}

func normalizePath(path string) string {
	return strings.ToLower(filepath.Clean(path))
}
