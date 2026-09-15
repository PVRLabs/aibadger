package scanner

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filegroups"
	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

// semanticSourceOwnership records the source boundaries of successful
// specialized detectors. It deliberately does not use their bounded file
// summaries: those summaries describe what was reported, not what the detector
// semantically owns.
type semanticSourceOwnership struct {
	projectRoot string
	claims      []semanticSourceClaim
	webRoots    []string
}

type semanticSourceClaim struct {
	language   string
	moduleRoot string
	areas      []sourceClaimArea
	exactPaths map[string]bool
	exclusions map[string]bool
}

type sourceClaimArea struct {
	root      string
	recursive bool
}

func newSemanticSourceOwnership(projectRoot string, modules []model.Module) semanticSourceOwnership {
	ownership := semanticSourceOwnership{projectRoot: filepath.Clean(projectRoot)}
	for _, module := range modules {
		moduleRoot, ok := normalizeProjectRelativePath(module.Path)
		if !ok && module.Path != "" && module.Path != "." {
			continue
		}
		claim := semanticSourceClaim{language: module.Language, moduleRoot: moduleRoot}
		// Only Go and Java delegate to scanModuleWebResources.
		if !module.Coverage && (module.Language == "Go" || module.Language == "Java") {
			for _, dir := range moduleWebResourceDirs() {
				ownership.webRoots = append(ownership.webRoots, joinRelativePath(moduleRoot, dir))
			}
		}
		switch module.Language {
		case "Go":
			claim.areas = goSourceClaimAreas(module)
			claim.exclusions = commonIgnoredDirs
		case "Java":
			claim.areas = javaSourceClaimAreas(module)
			claim.exclusions = commonIgnoredDirs
		case "Python":
			claim.exclusions = NewPythonDetector().Exclusions
			claim.areas = pythonSourceClaimAreas(projectRoot)
		case "JavaScript", "TypeScript":
			detector := NewNodeDetector()
			claim.exclusions = detector.Exclusions
			claim.areas, claim.exactPaths = nodeSourceClaimAreas(detector, projectRoot, moduleRoot)
		case "C#":
			claim.areas = []sourceClaimArea{{root: moduleRoot, recursive: true}}
			claim.exclusions = csharpIgnoredDirs
		case "C++":
			claim.exclusions = cppIgnoredDirs
			claim.areas = []sourceClaimArea{{root: moduleRoot}}
			for _, conventional := range cppConventionalRoots {
				claim.areas = append(claim.areas, sourceClaimArea{
					root:      joinRelativePath(moduleRoot, conventional.name),
					recursive: true,
				})
			}
		default:
			continue
		}
		ownership.claims = append(ownership.claims, claim)
	}
	return ownership
}

func (o semanticSourceOwnership) ownsWebResource(path string) bool {
	rel, _, ok := normalizeSourcePath(o.projectRoot, path)
	if !ok {
		return false
	}
	for _, root := range o.webRoots {
		if sameOrDescendantRelativePath(root, rel) {
			return true
		}
	}
	return false
}

// Owns reports whether a successful specialized detector owns path as source.
// path may be absolute or project-relative, but it must remain within the
// project root.
func (o semanticSourceOwnership) Owns(path string) bool {
	rel, _, ok := normalizeSourcePath(o.projectRoot, path)
	if !ok {
		return false
	}
	for _, claim := range o.claims {
		if claim.owns(o.projectRoot, rel) {
			return true
		}
	}
	return false
}

func (c semanticSourceClaim) owns(projectRoot, rel string) bool {
	if !sourceFileMatchesLanguage(c.language, filepath.Base(rel)) {
		return false
	}
	if c.exactPaths[rel] {
		return true
	}
	if pathUsesExcludedDir(rel, c.moduleRoot, c.exclusions) {
		return false
	}
	for _, area := range c.areas {
		if !pathInClaimArea(rel, area) {
			continue
		}
		if c.language == "C#" && crossesNestedCSharpProject(projectRoot, c.moduleRoot, rel) {
			return false
		}
		if c.language == "C++" && pathUsesCMakeBuildDir(rel, area.root) {
			return false
		}
		return true
	}
	return false
}

func sourceFileMatchesLanguage(language, name string) bool {
	switch language {
	case "Go":
		return isGoSourceFile(name)
	case "Java":
		return isJavaSourceFile(name)
	case "Python":
		return isPythonSourceFile(name)
	case "JavaScript", "TypeScript":
		return isNodeSourceFile(name)
	case "C#":
		return strings.EqualFold(filepath.Ext(name), ".cs")
	case "C++":
		return isAcceptedCppFile(name) && !strings.EqualFold(name, "CMakeLists.txt")
	default:
		return false
	}
}

func goSourceClaimAreas(module model.Module) []sourceClaimArea {
	areas := make([]sourceClaimArea, 0, len(module.SourceRoots))
	for _, sourceRoot := range module.SourceRoots {
		switch sourceRoot.Role {
		case "Entry Point", "Internal Source", "Library Source":
			areas = append(areas, sourceClaimArea{root: sourceRoot.Path, recursive: true})
		case "Main Source":
			areas = append(areas, sourceClaimArea{root: sourceRoot.Path})
		}
	}
	return areas
}

func javaSourceClaimAreas(module model.Module) []sourceClaimArea {
	areas := make([]sourceClaimArea, 0, len(module.SourceRoots))
	for _, sourceRoot := range module.SourceRoots {
		switch sourceRoot.Role {
		case "Main Source", "Test Source":
			areas = append(areas, sourceClaimArea{root: sourceRoot.Path, recursive: true})
		}
	}
	return areas
}

func pythonSourceClaimAreas(projectRoot string) []sourceClaimArea {
	detector := NewPythonDetector()
	areas := []sourceClaimArea{
		{root: "src", recursive: true},
		{root: "tests", recursive: true},
		{root: ""},
	}
	for _, rootName := range detector.shallowPackageDirs(projectRoot) {
		areas = append(areas, sourceClaimArea{root: rootName, recursive: true})
	}
	return areas
}

func nodeSourceClaimAreas(detector *NodeDetector, projectRoot, moduleRoot string) ([]sourceClaimArea, map[string]bool) {
	areas := make([]sourceClaimArea, 0, 6)
	for _, name := range []string{"src", "app", "lib", "server", "test", "tests"} {
		areas = append(areas, sourceClaimArea{root: joinRelativePath(moduleRoot, name), recursive: true})
	}

	exact := make(map[string]bool)
	fullModulePath := filepath.Join(projectRoot, moduleRoot)
	pkg, ok := detector.readPackageJSON(filepath.Join(fullModulePath, "package.json"))
	if !ok {
		return areas, exact
	}
	candidates := nodeRootEntryCandidates(fullModulePath)
	candidates = append(candidates, nodePackageEntryCandidates(pkg, fullModulePath)...)
	candidates = append(candidates, nodeStackOverviewCandidates(pkg, fullModulePath)...)
	for _, candidate := range candidates {
		if isNodeSourceFile(filepath.Base(candidate)) {
			exact[joinRelativePath(moduleRoot, candidate)] = true
		}
	}
	return areas, exact
}

func pathInClaimArea(rel string, area sourceClaimArea) bool {
	if area.recursive {
		return sameOrDescendantRelativePath(area.root, rel)
	}
	return normalizeRelativeDir(filepath.Dir(rel)) == area.root
}

func pathUsesExcludedDir(rel, boundary string, exclusions map[string]bool) bool {
	if len(exclusions) == 0 || !sameOrDescendantRelativePath(boundary, rel) {
		return false
	}
	dir := normalizeRelativeDir(filepath.Dir(rel))
	local, ok := relativeWithin(boundary, dir)
	if !ok || local == "" {
		return false
	}
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		if exclusions[strings.ToLower(part)] {
			return true
		}
	}
	return false
}

func pathUsesCMakeBuildDir(rel, boundary string) bool {
	dir := normalizeRelativeDir(filepath.Dir(rel))
	local, ok := relativeWithin(boundary, dir)
	if !ok || local == "" {
		return false
	}
	for _, part := range strings.Split(local, string(filepath.Separator)) {
		if strings.HasPrefix(strings.ToLower(part), "cmake-build-") {
			return true
		}
	}
	return false
}

func crossesNestedCSharpProject(projectRoot, moduleRoot, rel string) bool {
	dir := normalizeRelativeDir(filepath.Dir(rel))
	local, ok := relativeWithin(moduleRoot, dir)
	if !ok || local == "" {
		return false
	}
	current := moduleRoot
	parts := strings.Split(local, string(filepath.Separator))
	for _, part := range parts {
		current = joinRelativePath(current, part)
		if current != moduleRoot && ownsCSharpProject(filepath.Join(projectRoot, current)) {
			return true
		}
	}
	return false
}

func sameOrDescendantRelativePath(parent, child string) bool {
	_, ok := relativeWithin(parent, child)
	return ok
}

func relativeWithin(parent, child string) (string, bool) {
	parent = normalizeRelativeDir(filepath.Clean(parent))
	child = normalizeRelativeDir(filepath.Clean(child))
	rel, err := filepath.Rel(filepath.Join(string(filepath.Separator), parent), filepath.Join(string(filepath.Separator), child))
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return normalizeRelativeDir(rel), true
}

func joinRelativePath(base, path string) string {
	return normalizeRelativeDir(filepath.Clean(filepath.Join(base, path)))
}

func normalizeProjectRelativePath(path string) (string, bool) {
	if path == "" || path == "." {
		return "", true
	}
	if filepath.IsAbs(path) {
		return "", false
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", false
	}
	return normalizeRelativeDir(clean), true
}

func normalizeSourcePath(projectRoot, path string) (rel, full string, ok bool) {
	if filepath.IsAbs(path) {
		full = filepath.Clean(path)
		rel = relativePath(projectRoot, full)
		if rel == "" && full != filepath.Clean(projectRoot) {
			return "", "", false
		}
	} else {
		var valid bool
		rel, valid = normalizeProjectRelativePath(path)
		if !valid {
			return "", "", false
		}
		full = filepath.Join(projectRoot, rel)
	}
	if rel == "" || !sameOrDescendantRelativePath("", rel) {
		return "", "", false
	}
	return rel, full, true
}

// classifyGenericSourceCandidate is used only by mixed-repository source
// augmentation. The standalone Generic detector intentionally retains its
// broader behavior.
func classifyGenericSourceCandidate(projectRoot, path string) (string, bool) {
	rel, full, ok := normalizeSourcePath(projectRoot, path)
	if !ok {
		return "", false
	}
	name := filepath.Base(rel)
	language, ok := genericExtensionLanguages[strings.ToLower(filepath.Ext(name))]
	if !ok || shouldOmitFile(projectRoot, full, name) || isUnderIgnoredDir(projectRoot, full, NewGenericDetector().Exclusions) || (language == "C#" && isUnderCSharpObjDir(rel)) {
		return "", false
	}
	if filekind.Classify(full) != model.FileKindSource || isIdentityManifest(name) || isOperationalConfigFile(name) || isTextControlFile(name) {
		return "", false
	}
	dir := normalizeRelativeDir(filepath.Dir(rel))
	if isUnderAugmentationControlArea(dir, name) || isSharedWebResourcePath(rel) {
		return "", false
	}
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		return "", false
	}
	return language, true
}

func isUnderCSharpObjDir(rel string) bool {
	for _, segment := range strings.Split(filepath.Dir(rel), string(filepath.Separator)) {
		if strings.EqualFold(segment, "obj") {
			return true
		}
	}
	return false
}

func isUnderAugmentationControlArea(dir, name string) bool {
	if isAugmentationScriptControlPath(dir, name) {
		return true
	}
	for current := dir; current != "" && current != "."; current = normalizeRelativeDir(filepath.Dir(current)) {
		if filegroups.IsOpsDirectoryPath(current) {
			return true
		}
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	return false
}

func isAugmentationScriptControlPath(dir, name string) bool {
	lowerDir := strings.ToLower(filepath.Clean(dir))
	if lowerDir != "scripts" && !strings.HasPrefix(lowerDir, "scripts"+string(filepath.Separator)) {
		return false
	}
	localDir := strings.TrimPrefix(lowerDir, "scripts"+string(filepath.Separator))
	for _, segment := range strings.Split(localDir, string(filepath.Separator)) {
		if isAugmentationOperationalSegment(segment) {
			return true
		}
	}
	stem := strings.TrimSuffix(strings.ToLower(name), strings.ToLower(filepath.Ext(name)))
	return filegroups.HasOperationalNameToken(stem)
}

func isAugmentationOperationalSegment(segment string) bool {
	return filegroups.IsOpsTopLevelDirName(segment) ||
		filegroups.HasOperationalNameToken(segment) ||
		(strings.HasSuffix(segment, "s") && filegroups.HasOperationalNameToken(strings.TrimSuffix(segment, "s")))
}
