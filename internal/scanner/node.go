package scanner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/util"
)

const maxNodeRootOverviewFiles = maxRootPackageTopFiles

// NodeDetector handles Node-family projects rooted at package.json files.
type NodeDetector struct {
	Exclusions map[string]bool
}

// NewNodeDetector returns a new Node detector.
func NewNodeDetector() *NodeDetector {
	return &NodeDetector{
		Exclusions: cloneExclusions(commonIgnoredDirs, "dist"),
	}
}

// Detect scans for package.json files and creates one module per package root.
func (n *NodeDetector) Detect(root string) ([]model.Module, error) {
	var modules []model.Module
	seenModules := make(map[string]bool)
	workspaceRoots := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if shouldSkipDir(d.Name(), n.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Name() != "package.json" {
			return nil
		}

		packageJSON, ok := n.readPackageJSON(path)
		if !ok || !packageJSON.isEligibleModuleRoot() {
			return nil
		}

		modulePath := filepath.Dir(path)
		relPath := relativePath(root, modulePath)
		if seenModules[relPath] {
			return nil
		}

		for _, workspaceRoot := range n.workspaceModuleRoots(root, modulePath, packageJSON) {
			workspaceRoots[workspaceRoot] = true
		}

		seenModules[relPath] = true
		modules = append(modules, n.analyzeModule(root, relPath, packageJSON))
		return nil
	})
	if err != nil {
		return nil, err
	}

	workspacePaths := make([]string, 0, len(workspaceRoots))
	for relPath := range workspaceRoots {
		if relPath == "" || seenModules[relPath] {
			continue
		}
		workspacePaths = append(workspacePaths, relPath)
	}
	sort.Strings(workspacePaths)
	for _, relPath := range workspacePaths {
		packageJSON, ok := n.readPackageJSON(filepath.Join(root, relPath, "package.json"))
		if !ok {
			continue
		}
		seenModules[relPath] = true
		modules = append(modules, n.analyzeModule(root, relPath, packageJSON))
	}

	return modules, nil
}

func (n *NodeDetector) analyzeModule(projectRoot, relPath string, packageJSON nodePackageJSON) model.Module {
	fullModulePath := filepath.Join(projectRoot, relPath)
	module := model.Module{
		Name:     n.moduleName(projectRoot, relPath, packageJSON),
		Path:     relPath,
		Language: "JavaScript",
	}

	if module.Name == "" {
		module.Name = filepath.Base(projectRoot)
	}

	if packageJSON.TypeScript {
		module.Language = "TypeScript"
	}

	for _, rootName := range []string{"src", "app", "lib", "server", "test", "tests"} {
		sr, ok := n.findSourceRoot(projectRoot, fullModulePath, rootName)
		if !ok {
			continue
		}
		module.SourceRoots = append(module.SourceRoots, sr)
		module.FileCount += sr.FileCount
		limit := moduleTopFileLimit(module.Path, maxPackageTopFiles)
		for _, pkg := range sr.Packages {
			for _, file := range pkg.TopFiles {
				module.TopFiles = addTopFile(module.TopFiles, file, limit)
			}
		}
	}

	n.addRootOverviewPackage(&module, projectRoot, fullModulePath, packageJSON)
	if len(module.TopFiles) > 0 {
		module.Heaviest = heaviestFromSummary(module.TopFiles[0])
	}

	return module
}

func (n *NodeDetector) moduleName(projectRoot, relPath string, packageJSON nodePackageJSON) string {
	if packageJSON.Name != "" {
		return packageJSON.Name
	}
	if relPath == "" {
		return filepath.Base(projectRoot)
	}
	return filepath.Base(relPath)
}

func (n *NodeDetector) workspaceModuleRoots(projectRoot, modulePath string, packageJSON nodePackageJSON) []string {
	patterns := packageJSON.workspacePatterns()
	patterns = append(patterns, readPNPMWorkspacePatterns(filepath.Join(modulePath, "pnpm-workspace.yaml"))...)
	if len(patterns) == 0 {
		return nil
	}

	found := make(map[string]bool)
	for _, pattern := range patterns {
		for _, relPath := range expandWorkspacePattern(projectRoot, modulePath, pattern) {
			if relPath != "" {
				found[relPath] = true
			}
		}
	}

	roots := make([]string, 0, len(found))
	for relPath := range found {
		roots = append(roots, relPath)
	}
	sort.Strings(roots)
	return roots
}

func expandWorkspacePattern(projectRoot, modulePath, pattern string) []string {
	normalized, ok := normalizeWorkspacePattern(pattern)
	if !ok {
		return nil
	}

	if !strings.Contains(normalized, "*") {
		if hasWorkspacePackageJSON(modulePath, normalized) {
			return []string{relativePath(projectRoot, filepath.Join(modulePath, normalized))}
		}
		return nil
	}

	baseDir := strings.TrimSuffix(normalized, string(filepath.Separator)+"*")
	entries, err := os.ReadDir(filepath.Join(modulePath, baseDir))
	if err != nil {
		return nil
	}

	var roots []string
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipDir(entry.Name(), commonIgnoredDirs) {
			continue
		}
		childRelPath := filepath.Join(baseDir, entry.Name())
		if hasWorkspacePackageJSON(modulePath, childRelPath) {
			roots = append(roots, relativePath(projectRoot, filepath.Join(modulePath, childRelPath)))
		}
	}
	return roots
}

func normalizeWorkspacePattern(pattern string) (string, bool) {
	candidate := strings.TrimSpace(pattern)
	candidate, ok := normalizeRelativePath(candidate)
	if !ok {
		return "", false
	}
	if !strings.Contains(candidate, "*") {
		return candidate, true
	}
	if strings.Count(candidate, "*") != 1 {
		return "", false
	}
	if !strings.HasSuffix(candidate, string(filepath.Separator)+"*") {
		return "", false
	}
	baseDir := strings.TrimSuffix(candidate, string(filepath.Separator)+"*")
	if baseDir == "" {
		return "", false
	}
	if strings.Contains(baseDir, "*") {
		return "", false
	}
	return candidate, true
}

func hasWorkspacePackageJSON(modulePath, relPath string) bool {
	return util.FileExists(filepath.Join(modulePath, relPath, "package.json"))
}

func readPNPMWorkspacePatterns(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var patterns []string
	inPackages := false
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if !inPackages {
			if line == "packages:" {
				inPackages = true
			}
			continue
		}

		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, "-") {
			break
		}
		if !strings.HasPrefix(line, "-") {
			continue
		}

		pattern := strings.TrimSpace(strings.TrimPrefix(line, "-"))
		pattern = strings.Trim(pattern, "\"'")
		if pattern == "" {
			continue
		}
		patterns = append(patterns, pattern)
	}

	return patterns
}

func (n *NodeDetector) readPackageJSON(path string) (nodePackageJSON, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nodePackageJSON{}, false
	}

	var pkg nodePackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nodePackageJSON{}, false
	}

	pkg.TypeScript = pkg.hasTypeScriptSignals()
	return pkg, true
}

type nodePackageJSON struct {
	Name            string            `json:"name"`
	Main            string            `json:"main"`
	Module          string            `json:"module"`
	Types           string            `json:"types"`
	Bin             any               `json:"bin"`
	Workspaces      any               `json:"workspaces"`
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	TypeScript      bool              `json:"-"`
}

func (p nodePackageJSON) isEligibleModuleRoot() bool {
	if p.Name != "" || p.Main != "" || p.Module != "" || p.Types != "" || p.Bin != nil {
		return true
	}
	if len(p.workspacePatterns()) > 0 {
		return true
	}
	if len(p.Scripts) > 0 || len(p.Dependencies) > 0 || len(p.DevDependencies) > 0 {
		return true
	}
	return false
}

func (p nodePackageJSON) hasTypeScriptSignals() bool {
	if p.Types != "" {
		return true
	}
	if hasNodeDependency(nodePackageDependencies(p), "typescript") {
		return true
	}
	return false
}

func hasKey(values map[string]string, key string) bool {
	if len(values) == 0 {
		return false
	}
	_, ok := values[key]
	return ok
}

func (p nodePackageJSON) workspacePatterns() []string {
	switch workspaces := p.Workspaces.(type) {
	case []any:
		return workspacePatternList(workspaces)
	case map[string]any:
		packages, ok := workspaces["packages"].([]any)
		if !ok {
			return nil
		}
		return workspacePatternList(packages)
	default:
		return nil
	}
}

func workspacePatternList(values []any) []string {
	patterns := make([]string, 0, len(values))
	for _, value := range values {
		pattern, ok := value.(string)
		if !ok {
			continue
		}
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		patterns = append(patterns, pattern)
	}
	return patterns
}

func (n *NodeDetector) findSourceRoot(projectRoot, modulePath, rootName string) (model.SourceRoot, bool) {
	fullRootPath := filepath.Join(modulePath, rootName)
	info, err := os.Stat(fullRootPath)
	if err != nil || !info.IsDir() {
		return model.SourceRoot{}, false
	}

	sr := model.SourceRoot{
		Path: relativePath(projectRoot, fullRootPath),
		Role: n.inferSourceRole(rootName),
	}
	n.scanSourceRoot(&sr, projectRoot)
	if sr.FileCount == 0 {
		return model.SourceRoot{}, false
	}
	return sr, true
}

func (n *NodeDetector) inferSourceRole(rootName string) string {
	switch rootName {
	case "app":
		return "Entry Source"
	case "lib":
		return "Library Source"
	case "server":
		return "Server Source"
	case "test", "tests":
		return "Test Source"
	case "src":
		return "Main Source"
	default:
		return "Main Source"
	}
}

func (n *NodeDetector) addRootOverviewPackage(module *model.Module, projectRoot, fullModulePath string, packageJSON nodePackageJSON) {
	overview := n.rootOverviewPackage(projectRoot, fullModulePath, packageJSON)
	if overview.FileCount == 0 {
		return
	}

	if len(module.SourceRoots) == 0 {
		module.SourceRoots = append(module.SourceRoots, model.SourceRoot{
			Path: "",
			Role: "Module Overview",
		})
	}

	module.SourceRoots[0].Packages = append(module.SourceRoots[0].Packages, overview)
	module.SourceRoots[0].FileCount += overview.FileCount
	limit := moduleTopFileLimit(module.Path, maxPackageTopFiles)
	for _, file := range overview.TopFiles {
		module.TopFiles = addTopFile(module.TopFiles, file, limit)
	}
}

func (n *NodeDetector) rootOverviewPackage(projectRoot, fullModulePath string, packageJSON nodePackageJSON) model.Package {
	pkg := model.Package{
		Name: "root",
		Path: "",
	}

	candidates := append([]string{"package.json"}, nodeRootConfigCandidates(fullModulePath)...)
	candidates = append(candidates, nodeRootEntryCandidates(fullModulePath)...)
	candidates = append(candidates, nodePackageEntryCandidates(packageJSON, fullModulePath)...)
	candidates = append(candidates, nodeStackOverviewCandidates(packageJSON, fullModulePath)...)

	seen := make(map[string]bool, len(candidates))
	type nodeOverviewFile struct {
		summary model.FileSummary
		rank    int
	}
	var overviewFiles []nodeOverviewFile
	for _, relPath := range candidates {
		if relPath == "" || seen[relPath] {
			continue
		}
		seen[relPath] = true
		if shouldOmitFile(projectRoot, filepath.Join(fullModulePath, relPath), filepath.Base(relPath)) {
			continue
		}
		info, err := os.Stat(filepath.Join(fullModulePath, relPath))
		if err != nil || info.IsDir() {
			continue
		}
		pkg.FileCount++
		summary := model.FileSummary{
			Name: filepath.Base(relPath),
			Path: relativePath(projectRoot, filepath.Join(fullModulePath, relPath)),
			Size: info.Size(),
		}
		overviewFiles = append(overviewFiles, nodeOverviewFile{
			summary: summary,
			rank:    nodeOverviewPriority(summary.Path),
		})
	}

	sort.SliceStable(overviewFiles, func(i, j int) bool {
		if overviewFiles[i].rank != overviewFiles[j].rank {
			return overviewFiles[i].rank < overviewFiles[j].rank
		}
		if overviewFiles[i].summary.Size != overviewFiles[j].summary.Size {
			return overviewFiles[i].summary.Size > overviewFiles[j].summary.Size
		}
		return overviewFiles[i].summary.Path < overviewFiles[j].summary.Path
	})
	if len(overviewFiles) > maxNodeRootOverviewFiles {
		overviewFiles = overviewFiles[:maxNodeRootOverviewFiles]
	}
	for _, file := range overviewFiles {
		pkg.TopFiles = append(pkg.TopFiles, file.summary)
	}
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
	return pkg
}

func nodeOverviewPriority(relPath string) int {
	base := strings.ToLower(filepath.Base(relPath))
	switch base {
	case "package.json":
		return 0
	case "tsconfig.json", "jsconfig.json":
		return 1
	case "next.config.js", "next.config.ts", "next.config.mjs", "next.config.cjs",
		"vite.config.js", "vite.config.ts", "vite.config.mjs", "vite.config.cjs",
		"nest-cli.json":
		return 2
	}

	dir := strings.ToLower(filepath.Dir(relPath))
	if dir == "." || dir == "" {
		switch base {
		case "server.ts", "server.js":
			return 3
		case "main.ts", "main.js":
			return 4
		case "index.ts", "index.js":
			return 5
		case "index.html":
			return 6
		case "app.tsx", "app.jsx", "app.ts", "app.js":
			return 7
		}
	}

	if strings.HasPrefix(relPath, "pages"+string(filepath.Separator)) || strings.HasPrefix(relPath, "app"+string(filepath.Separator)) {
		return 8
	}
	if strings.HasPrefix(relPath, "components"+string(filepath.Separator)) ||
		strings.HasPrefix(relPath, "hooks"+string(filepath.Separator)) ||
		strings.HasPrefix(relPath, "src"+string(filepath.Separator)+"components"+string(filepath.Separator)) ||
		strings.HasPrefix(relPath, "src"+string(filepath.Separator)+"hooks"+string(filepath.Separator)) {
		return 9
	}
	if strings.HasPrefix(relPath, "server"+string(filepath.Separator)) {
		return 10
	}
	if strings.HasPrefix(relPath, "apps"+string(filepath.Separator)) {
		return 11
	}
	if strings.HasPrefix(relPath, "libs"+string(filepath.Separator)) {
		return 12
	}
	if strings.HasPrefix(relPath, "bin"+string(filepath.Separator)) {
		return 13
	}
	if strings.HasSuffix(base, ".d.ts") {
		return 14
	}
	if strings.HasPrefix(relPath, "src"+string(filepath.Separator)) {
		return 15
	}
	return 16
}

func nodeRootConfigCandidates(fullModulePath string) []string {
	var candidates []string
	for _, name := range []string{"tsconfig.json", "jsconfig.json"} {
		if util.FileExists(filepath.Join(fullModulePath, name)) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func nodeRootEntryCandidates(fullModulePath string) []string {
	var candidates []string
	for _, name := range []string{
		"server.ts", "server.js",
		"main.ts", "main.js",
		"index.ts", "index.js",
		"index.html",
	} {
		if util.FileExists(filepath.Join(fullModulePath, name)) {
			candidates = append(candidates, name)
		}
	}
	return candidates
}

func nodePackageEntryCandidates(pkg nodePackageJSON, fullModulePath string) []string {
	var candidates []string
	for _, relPath := range []string{pkg.Main, pkg.Module, pkg.Types} {
		if normalized, ok := normalizeNodeCandidatePath(relPath, fullModulePath); ok {
			candidates = append(candidates, normalized)
		}
	}

	switch bin := pkg.Bin.(type) {
	case string:
		if normalized, ok := normalizeNodeCandidatePath(bin, fullModulePath); ok {
			candidates = append(candidates, normalized)
		}
	case map[string]any:
		for _, value := range bin {
			pathValue, ok := value.(string)
			if !ok {
				continue
			}
			if normalized, ok := normalizeNodeCandidatePath(pathValue, fullModulePath); ok {
				candidates = append(candidates, normalized)
			}
		}
	}

	for _, script := range pkg.Scripts {
		for _, token := range obviousScriptPathTokens(script) {
			if normalized, ok := normalizeNodeCandidatePath(token, fullModulePath); ok {
				candidates = append(candidates, normalized)
			}
		}
	}

	return candidates
}

func nodeStackOverviewCandidates(pkg nodePackageJSON, fullModulePath string) []string {
	var candidates []string
	deps := nodePackageDependencies(pkg)

	if hasReactOverviewSignal(deps, fullModulePath) {
		candidates = append(candidates,
			nodeExistingCandidates(fullModulePath,
				"App.jsx", "App.tsx", "src/App.jsx", "src/App.tsx",
				"main.jsx", "main.tsx", "index.jsx", "index.tsx",
				filepath.Join("src", "main.jsx"), filepath.Join("src", "main.tsx"),
				filepath.Join("src", "index.jsx"), filepath.Join("src", "index.tsx"),
			)...,
		)
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "components")
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "hooks")
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, filepath.Join("src", "components"))
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, filepath.Join("src", "hooks"))
	}

	if hasNextOverviewSignal(deps, fullModulePath) {
		candidates = append(candidates,
			nodeExistingCandidates(fullModulePath,
				"next.config.js", "next.config.ts", "next.config.mjs", "next.config.cjs",
				filepath.Join("pages", "index.jsx"), filepath.Join("pages", "index.tsx"),
				filepath.Join("pages", "index.js"), filepath.Join("pages", "index.ts"),
				filepath.Join("app", "page.jsx"), filepath.Join("app", "page.tsx"),
				filepath.Join("app", "page.js"), filepath.Join("app", "page.ts"),
			)...,
		)
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "pages")
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "app")
	}

	if hasVueOverviewSignal(deps, fullModulePath) {
		candidates = append(candidates,
			nodeExistingCandidates(fullModulePath,
				"src/main.ts", "src/main.js", "main.ts", "main.js",
				"src/App.vue", "App.vue",
			)...,
		)
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "components")
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, filepath.Join("src", "components"))
	}

	if hasViteOverviewSignal(deps, fullModulePath) {
		candidates = append(candidates,
			nodeExistingCandidates(fullModulePath,
				"vite.config.js", "vite.config.ts", "vite.config.mjs", "vite.config.cjs",
				"main.jsx", "main.tsx", "main.js", "main.ts",
				filepath.Join("src", "main.jsx"), filepath.Join("src", "main.tsx"),
				filepath.Join("src", "main.js"), filepath.Join("src", "main.ts"),
			)...,
		)
	}

	if hasNestOverviewSignal(deps, fullModulePath) {
		candidates = append(candidates,
			nodeExistingCandidates(fullModulePath, "nest-cli.json", filepath.Join("src", "main.ts"), filepath.Join("src", "main.js"))...,
		)
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "apps")
		candidates = appendRepresentativeDirCandidate(candidates, fullModulePath, "libs")
	}

	return candidates
}

func nodeExistingCandidates(fullModulePath string, relPaths ...string) []string {
	var candidates []string
	for _, relPath := range relPaths {
		if normalized, ok := normalizeNodeCandidatePath(relPath, fullModulePath); ok {
			candidates = append(candidates, normalized)
		}
	}
	return candidates
}

func appendRepresentativeDirCandidate(candidates []string, fullModulePath, relDir string) []string {
	if relPath, ok := firstNodeOverviewFileUnderDir(fullModulePath, relDir); ok {
		return append(candidates, relPath)
	}
	return candidates
}

func firstNodeOverviewFileUnderDir(fullModulePath, relDir string) (string, bool) {
	dirPath := filepath.Join(fullModulePath, relDir)
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		return "", false
	}

	var found string
	filepath.WalkDir(dirPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if d.IsDir() {
			if path != dirPath && shouldSkipDir(d.Name(), commonIgnoredDirs) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isNodeOverviewFile(d.Name()) {
			return nil
		}
		relToModule, relErr := filepath.Rel(fullModulePath, path)
		if relErr != nil {
			return nil
		}
		found = relToModule
		return nil
	})
	if found == "" {
		return "", false
	}
	return found, true
}

func isNodeOverviewFile(name string) bool {
	if isNodeSourceFile(name) {
		return true
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".vue":
		return true
	default:
		return false
	}
}

func obviousScriptPathTokens(script string) []string {
	fields := strings.Fields(script)
	var tokens []string
	for _, field := range fields {
		token := strings.TrimSpace(field)
		token = strings.Trim(token, "\"'")
		if token == "" {
			continue
		}
		if strings.HasPrefix(token, "-") {
			continue
		}
		if strings.Contains(token, "=") {
			continue
		}
		if isCommandToken(token) {
			continue
		}
		if !looksLikeNodePathToken(token) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func isCommandToken(token string) bool {
	switch token {
	case "node", "tsx", "ts-node", "ts-node-esm", "nodemon", "vite", "vitest", "jest", "mocha", "tsx-watch":
		return true
	default:
		return false
	}
}

func looksLikeNodePathToken(token string) bool {
	if strings.Contains(token, "/") || strings.Contains(token, "\\") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(token))
	switch ext {
	case ".js", ".jsx", ".cjs", ".mjs", ".ts", ".tsx", ".cts", ".mts":
		return true
	default:
		return false
	}
}

func normalizeRelativePath(candidate string) (string, bool) {
	if candidate == "" {
		return "", false
	}
	if filepath.IsAbs(candidate) {
		return "", false
	}
	candidate = filepath.Clean(filepath.FromSlash(candidate))
	if candidate == "." || candidate == "" {
		return "", false
	}
	if candidate == ".." || strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}

func normalizeNodeCandidatePath(relPath, fullModulePath string) (string, bool) {
	candidate := strings.TrimSpace(relPath)
	candidate = strings.Trim(candidate, "\"'")
	candidate, ok := normalizeRelativePath(candidate)
	if !ok {
		return "", false
	}
	fullPath := filepath.Join(fullModulePath, candidate)
	if !util.FileExists(fullPath) {
		return "", false
	}
	relToModule, err := filepath.Rel(fullModulePath, fullPath)
	if err != nil {
		return "", false
	}
	if relToModule == ".." || strings.HasPrefix(relToModule, ".."+string(filepath.Separator)) {
		return "", false
	}
	return relToModule, true
}

func (n *NodeDetector) scanSourceRoot(sr *model.SourceRoot, projectRoot string) {
	fullRootPath := filepath.Join(projectRoot, sr.Path)
	packageMap := make(map[string]*model.Package)

	filepath.WalkDir(fullRootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}

		if d.IsDir() {
			if path != fullRootPath && shouldSkipDir(d.Name(), n.Exclusions) {
				return filepath.SkipDir
			}
			return nil
		}

		if !isNodeSourceFile(d.Name()) {
			return nil
		}

		info, infoErr := d.Info()
		if infoErr != nil {
			return nil
		}
		sr.FileCount++
		recordNodeFile(packageMap, fullRootPath, projectRoot, path, d.Name(), info.Size())
		return nil
	})

	for _, pkg := range packageMap {
		sr.Packages = append(sr.Packages, *pkg)
	}
	sort.Slice(sr.Packages, func(i, j int) bool {
		return packageSortKey(sr.Packages[i]) < packageSortKey(sr.Packages[j])
	})
}

func isNodeSourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".js", ".jsx", ".cjs", ".mjs", ".ts", ".tsx", ".cts", ".mts":
		return true
	default:
		return false
	}
}

func recordNodeFile(packageMap map[string]*model.Package, fullRootPath, projectRoot, path, name string, size int64) {
	pkgPath := filepath.Dir(path)
	relPkgPath := normalizeRelativeDir(relativePath(fullRootPath, pkgPath))
	relToProject := relativePath(projectRoot, path)

	pkg, exists := packageMap[relPkgPath]
	if !exists {
		pkg = &model.Package{
			Name: nodePackageName(relPkgPath),
			Path: relativePath(projectRoot, pkgPath),
		}
		packageMap[relPkgPath] = pkg
	}

	pkg.FileCount++
	limit := packageTopFileLimit(relativePath(projectRoot, pkgPath), maxPackageTopFiles)
	pkg.TopFiles = addTopFile(pkg.TopFiles, model.FileSummary{
		Name: name,
		Path: relToProject,
		Size: size,
	}, limit)
	if len(pkg.TopFiles) > 0 {
		pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
	}
}

func nodePackageName(relPkgPath string) string {
	if relPkgPath == "" {
		return "root"
	}
	return filepath.Base(relPkgPath)
}
