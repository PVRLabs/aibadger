package scanner

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestNodeDetectorTreatsEligiblePackageJSONAsModuleRoot(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"web-app","scripts":{"start":"node server.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{"name":"@acme/ui","dependencies":{"react":"^18.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "empty", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "broken", "package.json"), `{`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want 2 eligible package roots", len(modules))
	}

	rootModule := findModule(modules, "web-app")
	if rootModule == nil {
		t.Fatalf("modules = %+v, missing root module", modules)
	}
	if filepath.IsAbs(rootModule.Path) {
		t.Fatalf("root module path = %q, want root-relative path", rootModule.Path)
	}
	if rootModule.Path != "" {
		t.Fatalf("root module path = %q, want empty root-relative path", rootModule.Path)
	}
	if rootModule.Language != "JavaScript" {
		t.Fatalf("root module language = %q, want JavaScript", rootModule.Language)
	}

	uiModule := findModule(modules, "@acme/ui")
	if uiModule == nil {
		t.Fatalf("modules = %+v, missing nested package module", modules)
	}
	if filepath.IsAbs(uiModule.Path) {
		t.Fatalf("ui module path = %q, want root-relative path", uiModule.Path)
	}
	if uiModule.Path != filepath.Join("packages", "ui") {
		t.Fatalf("ui module path = %q, want packages/ui", uiModule.Path)
	}
}

func TestNodeDetectorDiscoversShallowPackageAtDepthFour(t *testing.T) {
	tmpDir := t.TempDir()
	modulePath := filepath.Join("one", "two", "three", "four")

	writeTestFile(t, filepath.Join(tmpDir, modulePath, "package.json"), `{"name":"depth-four","scripts":{"dev":"node src/index.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, modulePath, "src", "index.js"), "export const ok = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if modules[0].Name != "depth-four" {
		t.Fatalf("module.Name = %q, want depth-four", modules[0].Name)
	}
	if modules[0].Path != modulePath {
		t.Fatalf("module.Path = %q, want %q", modules[0].Path, modulePath)
	}
	if !hasSourceRoot(modules[0], filepath.Join(modulePath, "src")) {
		t.Fatalf("module.SourceRoots = %+v, missing confirmed package source root", modules[0].SourceRoots)
	}
}

func TestNodeDetectorIgnoresUnrelatedPackageDeeperThanDepthFour(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"root-app","main":"src/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export const root = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "one", "two", "three", "four", "five", "package.json"), `{"name":"depth-five","main":"src/index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "one", "two", "three", "four", "five", "src", "index.js"), "export const deep = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want only root package: %+v", len(modules), modules)
	}
	if findModule(modules, "root-app") == nil {
		t.Fatalf("modules = %+v, missing root-app", modules)
	}
	if findModule(modules, "depth-five") != nil {
		t.Fatalf("modules = %+v, should not include unrelated depth-five package", modules)
	}
}

func TestNodeDetectorSupportsExplicitWorkspacePackages(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"monorepo",
  "workspaces":["packages/ui","packages/api"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "src", "index.ts"), "export const ui = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "package.json"), `{"name":"api"}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "server", "index.js"), "export const api = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 3 {
		t.Fatalf("len(modules) = %d, want 3 workspace modules", len(modules))
	}
	if findModule(modules, "monorepo") == nil {
		t.Fatalf("modules = %+v, missing workspace root module", modules)
	}
	if uiModule := findModule(modules, "ui"); uiModule == nil || uiModule.Path != filepath.Join("packages", "ui") {
		t.Fatalf("modules = %+v, missing explicit workspace ui module", modules)
	}
	if apiModule := findModule(modules, "api"); apiModule == nil || apiModule.Path != filepath.Join("packages", "api") {
		t.Fatalf("modules = %+v, missing explicit workspace api module", modules)
	}
}

func TestNodeDetectorSupportsDeepExplicitWorkspacePackage(t *testing.T) {
	tmpDir := t.TempDir()
	deepPackagePath := filepath.Join("one", "two", "three", "four", "five")

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"monorepo",
  "workspaces":["`+filepath.ToSlash(deepPackagePath)+`"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, deepPackagePath, "package.json"), `{"name":"deep-package"}`)
	writeTestFile(t, filepath.Join(tmpDir, deepPackagePath, "src", "index.ts"), "export const deep = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want workspace root and deep package: %+v", len(modules), modules)
	}
	if findModule(modules, "monorepo") == nil {
		t.Fatalf("modules = %+v, missing monorepo root", modules)
	}
	deepModule := findModule(modules, "deep-package")
	if deepModule == nil {
		t.Fatalf("modules = %+v, missing deep workspace package", modules)
	}
	if deepModule.Path != deepPackagePath {
		t.Fatalf("deepModule.Path = %q, want %q", deepModule.Path, deepPackagePath)
	}
	if !hasSourceRoot(*deepModule, filepath.Join(deepPackagePath, "src")) {
		t.Fatalf("deepModule.SourceRoots = %+v, missing deep source root", deepModule.SourceRoots)
	}
}

func TestNodeDetectorDeduplicatesWorkspaceAndDiscoveredPackages(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"workspace-root",
  "workspaces":["packages/ui"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{"name":"ui"}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "src", "index.ts"), "export const ui = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 2 {
		t.Fatalf("len(modules) = %d, want deduplicated root and workspace package: %+v", len(modules), modules)
	}
	if findModule(modules, "workspace-root") == nil || findModule(modules, "ui") == nil {
		t.Fatalf("modules = %+v, missing expected workspace modules", modules)
	}
}

func TestNodeDetectorSupportsTrivialWorkspaceExpansion(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"workspace-root",
  "workspaces":{"packages":["apps/*"]}
}`)
	writeTestFile(t, filepath.Join(tmpDir, "apps", "web", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "apps", "web", "src", "index.ts"), "export const web = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "admin", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "apps", "admin", "src", "index.ts"), "export const admin = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "nested", "client", "package.json"), `{"name":"client"}`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if findModule(modules, "web") == nil {
		t.Fatalf("modules = %+v, missing trivially expanded web module", modules)
	}
	if findModule(modules, "admin") == nil {
		t.Fatalf("modules = %+v, missing trivially expanded admin module", modules)
	}
	if nested := findModule(modules, "client"); nested == nil || nested.Path != filepath.Join("apps", "nested", "client") {
		t.Fatalf("modules = %+v, expected standalone nested client module to remain discoverable", modules)
	}
}

func TestNodeDetectorSupportsDeepPNPMWorkspaceWithoutRootPackage(t *testing.T) {
	tmpDir := t.TempDir()
	deepPackagePath := filepath.Join("one", "two", "three", "four", "five")

	writeTestFile(t, filepath.Join(tmpDir, "pnpm-workspace.yaml"), "packages:\n  - '"+filepath.ToSlash(deepPackagePath)+"'\n")
	writeTestFile(t, filepath.Join(tmpDir, deepPackagePath, "package.json"), `{"name":"deep-pnpm"}`)
	writeTestFile(t, filepath.Join(tmpDir, deepPackagePath, "src", "index.ts"), "export const deep = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want deep pnpm package: %+v", len(modules), modules)
	}
	module := findModule(modules, "deep-pnpm")
	if module == nil {
		t.Fatalf("modules = %+v, missing deep pnpm package", modules)
	}
	if module.Path != deepPackagePath {
		t.Fatalf("module.Path = %q, want %q", module.Path, deepPackagePath)
	}
}

func TestNodeDetectorIgnoresAmbiguousWorkspacePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"ambiguous-root",
  "workspaces":["*", "apps/*/client", "packages/**", "../outside"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, "apps", "web", "client", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{}`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want only the workspace root module", len(modules))
	}
	if modules[0].Name != "ambiguous-root" {
		t.Fatalf("modules = %+v, want only ambiguous-root", modules)
	}
}

func TestNodeDetectorSupportsConservativePNPMWorkspacePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"pnpm-root"}`)
	writeTestFile(t, filepath.Join(tmpDir, "pnpm-workspace.yaml"), "packages:\n  - 'packages/*'\n  - 'apps/api'\n")
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "src", "index.ts"), "export const ui = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "api", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "apps", "api", "src", "index.ts"), "export const api = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if findModule(modules, "ui") == nil {
		t.Fatalf("modules = %+v, missing pnpm workspace ui module", modules)
	}
	if findModule(modules, "api") == nil {
		t.Fatalf("modules = %+v, missing pnpm workspace api module", modules)
	}
}

func TestNodeDetectorIgnoresAmbiguousPNPMWorkspacePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"pnpm-root"}`)
	writeTestFile(t, filepath.Join(tmpDir, "pnpm-workspace.yaml"), "packages:\n  - '**'\n  - 'apps/*/client'\n  - '../outside'\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "web", "client", "package.json"), `{}`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 || modules[0].Name != "pnpm-root" {
		t.Fatalf("modules = %+v, want only pnpm-root", modules)
	}
}

func TestNodeDetectorRequiresChildPackageJSONForWorkspaceModule(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"workspace-root",
  "workspaces":["packages/*"]
}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "ui", "src", "index.ts"), "export const ui = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "package.json"), `{}`)
	writeTestFile(t, filepath.Join(tmpDir, "packages", "api", "src", "index.ts"), "export const api = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}

	if findModule(modules, "api") == nil {
		t.Fatalf("modules = %+v, missing workspace child with package.json", modules)
	}
	if uiModule := findModule(modules, "ui"); uiModule != nil {
		t.Fatalf("modules = %+v, should not include workspace child without package.json", modules)
	}
}

func TestNodeDetectorInfersTypeScriptFromMinimalPackageSignals(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"ts-lib","devDependencies":{"typescript":"^5.0.0"}}`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}
	if modules[0].Language != "TypeScript" {
		t.Fatalf("module.Language = %q, want TypeScript", modules[0].Language)
	}
}

func TestNodeDetectorRecognizesCommonSourceRoots(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"web-app","scripts":{"dev":"node server/index.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export const src = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "app", "bootstrap.ts"), "export const app = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "lib", "util.js"), "export const util = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "server", "index.js"), "export const server = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "scripts", "build.js"), "export const build = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "app", "assets", "logo.svg"), "<svg/>")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	for _, root := range []string{"src", "app", "lib", "server"} {
		if !hasSourceRoot(module, root) {
			t.Fatalf("module.SourceRoots = %+v, missing %s", module.SourceRoots, root)
		}
	}
	if hasSourceRoot(module, "scripts") {
		t.Fatalf("module.SourceRoots = %+v, should not include scripts", module.SourceRoots)
	}
	if module.FileCount != 4 {
		t.Fatalf("module.FileCount = %d, want 4 source files from recognized roots", module.FileCount)
	}
	if !hasPackage(module, "src") {
		t.Fatalf("module packages missing src root package")
	}
	if !hasPackage(module, filepath.Join("app")) {
		t.Fatalf("module packages missing app root package")
	}
	if !hasPackage(module, filepath.Join("lib")) {
		t.Fatalf("module packages missing lib root package")
	}
	if !hasPackage(module, filepath.Join("server")) {
		t.Fatalf("module packages missing server root package")
	}

	if srcRoot := findSourceRoot(module, "src"); srcRoot == nil || srcRoot.Role != "Main Source" {
		t.Fatalf("src source root = %+v, want Main Source", srcRoot)
	}
	if appRoot := findSourceRoot(module, "app"); appRoot == nil || appRoot.Role != "Entry Source" {
		t.Fatalf("app source root = %+v, want Entry Source", appRoot)
	}
	if libRoot := findSourceRoot(module, "lib"); libRoot == nil || libRoot.Role != "Library Source" {
		t.Fatalf("lib source root = %+v, want Library Source", libRoot)
	}
	if serverRoot := findSourceRoot(module, "server"); serverRoot == nil || serverRoot.Role != "Server Source" {
		t.Fatalf("server source root = %+v, want Server Source", serverRoot)
	}
}

func TestNodeDetectorIncludesConservativeTestRoots(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"web-app","scripts":{"test":"node test/index.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export const src = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "test", "index.js"), "export const testRoot = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "api", "user.ts"), "export const testsRoot = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "tests", "fixtures", "payload.json"), "{}")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if !hasSourceRoot(module, "test") {
		t.Fatalf("module.SourceRoots = %+v, missing test source root", module.SourceRoots)
	}
	if !hasSourceRoot(module, "tests") {
		t.Fatalf("module.SourceRoots = %+v, missing tests source root", module.SourceRoots)
	}

	testRoot := findSourceRoot(module, "test")
	if testRoot == nil || testRoot.Role != "Test Source" {
		t.Fatalf("test source root = %+v, want Test Source role", testRoot)
	}
	testsRoot := findSourceRoot(module, "tests")
	if testsRoot == nil || testsRoot.Role != "Test Source" {
		t.Fatalf("tests source root = %+v, want Test Source role", testsRoot)
	}
	if module.FileCount != 3 {
		t.Fatalf("module.FileCount = %d, want 3 source files including test roots", module.FileCount)
	}
}

func TestNodeDetectorSupportsPlannedJSTSExtensions(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"multi-ext","scripts":{"dev":"node src/index.mjs"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export const js = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "component.jsx"), "export const jsx = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "loader.cjs"), "module.exports = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "entry.mjs"), "export const mjs = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "types.ts"), "export type Thing = string\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "config.cts"), "export const cts = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "runtime.mts"), "export const mts = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "component.tsx"), "export const tsx = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if module.FileCount != 8 {
		t.Fatalf("module.FileCount = %d, want 8 supported JS/TS extension files", module.FileCount)
	}
	pkg := findPackage(module, "src")
	if pkg == nil {
		t.Fatalf("missing src package")
	}
	if pkg.FileCount != 8 {
		t.Fatalf("pkg.FileCount = %d, want 8 supported JS/TS extension files", pkg.FileCount)
	}
	if len(pkg.TopFiles) != 3 {
		t.Fatalf("len(pkg.TopFiles) = %d, want capped top file set of 3", len(pkg.TopFiles))
	}
	for _, path := range []string{
		filepath.Join("src", "component.tsx"),
		filepath.Join("src", "component.jsx"),
		filepath.Join("src", "types.ts"),
	} {
		if !hasTopFile(pkg.TopFiles, path) {
			t.Fatalf("pkg.TopFiles = %+v, missing expected top file %s", pkg.TopFiles, path)
		}
	}
}

func TestNodeDetectorIgnoresExcludedDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"root-app","scripts":{"start":"node server.js"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "node_modules", "left-pad", "package.json"), `{"name":"left-pad","main":"index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "build", "generated", "package.json"), `{"name":"build-output","main":"index.js"}`)
	writeTestFile(t, filepath.Join(tmpDir, "dist", "bundle", "package.json"), `{"name":"dist-output","main":"index.js"}`)

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1 after excluding generated directories", len(modules))
	}
	if modules[0].Name != "root-app" {
		t.Fatalf("module.Name = %q, want root-app", modules[0].Name)
	}
}

func TestNodeDetectorAvoidsFrameworkSpecificRootContracts(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{"name":"frameworkish","dependencies":{"next":"^15.0.0"}}`)
	writeTestFile(t, filepath.Join(tmpDir, "pages", "index.ts"), "export default function Page() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "components", "Button.ts"), "export const Button = () => null\n")
	writeTestFile(t, filepath.Join(tmpDir, "hooks", "useThing.ts"), "export const useThing = () => true\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "web", "index.ts"), "export const app = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "libs", "shared", "index.ts"), "export const shared = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "pages", "home.ts"), "export const home = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	if !hasSourceRoot(module, "src") {
		t.Fatalf("module.SourceRoots = %+v, missing src root", module.SourceRoots)
	}
	for _, forbidden := range []string{"pages", "components", "hooks", "apps", "libs"} {
		if hasSourceRoot(module, forbidden) {
			t.Fatalf("module.SourceRoots = %+v, should not include framework-specific root %s", module.SourceRoots, forbidden)
		}
	}
	if module.FileCount != 1 {
		t.Fatalf("module.FileCount = %d, want only files under broad supported roots", module.FileCount)
	}
	if !hasPackage(module, filepath.Join("src", "pages")) {
		t.Fatalf("module packages = %+v, want src/pages package from supported src root", module.SourceRoots)
	}
}

func TestNodeDetectorSurfacesPackageJSONInRootPackageSummary(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), "{\n  \"name\": \"web-app\",\n  \"scripts\": {\"dev\": \"node server.js\"}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "tsconfig.json"), "{\n  \"compilerOptions\": {\"target\": \"ES2022\"}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.ts"), "export const ready = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	module := modules[0]
	rootPkg := findPackage(module, "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", module.SourceRoots)
	}
	if rootPkg.FileCount != 2 {
		t.Fatalf("rootPkg.FileCount = %d, want 2", rootPkg.FileCount)
	}
	for _, path := range []string{"package.json", "tsconfig.json"} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
}

func TestNodeDetectorSurfacesRootConfigAndEntryFilesInRootPackageSummary(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), "{\n  \"name\": \"service-app\",\n  \"scripts\": {\"dev\": \"node server.js\"}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "jsconfig.json"), "{\n  \"compilerOptions\": {\"checkJs\": true}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "server.js"), "export const boot = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "index.js"), "export const entry = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "index.js"), "export const src = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	for _, path := range []string{"package.json", "jsconfig.json", "server.js", "index.js"} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
	if hasTopFile(rootPkg.TopFiles, filepath.Join("src", "index.js")) {
		t.Fatalf("rootPkg.TopFiles = %+v, should not include nested source-root entries", rootPkg.TopFiles)
	}
}

func TestNodeDetectorSurfacesPackageJSONEntryTargetsInRootPackageSummary(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"pkg-signals",
  "main":"dist/main.js",
  "module":"esm/index.mjs",
  "types":"types/index.d.ts",
  "bin":{"badger-node":"bin/cli.js"},
  "scripts":{
    "dev":"node server/dev.js",
    "test":"vitest run",
    "bad":"node ../outside.js"
  }
}`)
	writeTestFile(t, filepath.Join(tmpDir, "dist", "main.js"), "module.exports = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "esm", "index.mjs"), "export const esm = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "types", "index.d.ts"), "export type Thing = string\n")
	writeTestFile(t, filepath.Join(tmpDir, "bin", "cli.js"), "console.log('cli')\n")
	writeTestFile(t, filepath.Join(tmpDir, "server", "dev.js"), "export const dev = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	if len(modules) != 1 {
		t.Fatalf("len(modules) = %d, want 1", len(modules))
	}

	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	for _, path := range []string{
		"package.json",
		filepath.Join("dist", "main.js"),
		filepath.Join("esm", "index.mjs"),
		filepath.Join("bin", "cli.js"),
		filepath.Join("server", "dev.js"),
	} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
	if hasTopFile(rootPkg.TopFiles, filepath.Join("..", "outside.js")) {
		t.Fatalf("rootPkg.TopFiles = %+v, should not include escaping paths", rootPkg.TopFiles)
	}
}

func TestNodeDetectorRootOverviewUsesDetectorPriorityBeforeFileSize(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), "{\n  \"name\": \"ranked-app\"\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "tsconfig.json"), "{\n  \"compilerOptions\": {\"strict\": true}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "server.js"), strings.Repeat("s", 32)+"\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.js"), strings.Repeat("m", 256)+"\n")
	writeTestFile(t, filepath.Join(tmpDir, "index.js"), strings.Repeat("i", 512)+"\n")
	writeTestFile(t, filepath.Join(tmpDir, "misc.js"), strings.Repeat("x", 4096)+"\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	got := []string{}
	for _, file := range rootPkg.TopFiles {
		got = append(got, file.Path)
	}
	wantPrefix := []string{"package.json", "tsconfig.json", "server.js", "main.js", "index.js"}
	if len(got) < len(wantPrefix) {
		t.Fatalf("rootPkg.TopFiles = %+v, want at least %v", rootPkg.TopFiles, wantPrefix)
	}
	for i, want := range wantPrefix {
		if got[i] != want {
			t.Fatalf("rootPkg.TopFiles[%d] = %q, want %q; full order = %v", i, got[i], want, got)
		}
	}
}

func TestNodeDetectorRootOverviewIsCappedAndDeterministic(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"bounded-app",
  "main":"dist/main.js",
  "module":"esm/index.mjs",
  "types":"types/index.d.ts",
  "bin":{"tool":"bin/cli.js"},
  "scripts":{"dev":"node server/dev.js"}
}`)
	writeTestFile(t, filepath.Join(tmpDir, "tsconfig.json"), "{\n  \"compilerOptions\": {\"strict\": true}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "jsconfig.json"), "{\n  \"compilerOptions\": {\"checkJs\": true}\n}\n")
	writeTestFile(t, filepath.Join(tmpDir, "server.js"), "export const server = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.js"), "export const main = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "index.js"), "export const index = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "server", "dev.js"), "export const dev = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "bin", "cli.js"), "console.log('cli')\n")
	writeTestFile(t, filepath.Join(tmpDir, "dist", "main.js"), "module.exports = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "esm", "index.mjs"), "export const esm = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "types", "index.d.ts"), "export type Thing = string\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	if len(rootPkg.TopFiles) != maxNodeRootOverviewFiles {
		t.Fatalf("len(rootPkg.TopFiles) = %d, want %d", len(rootPkg.TopFiles), maxNodeRootOverviewFiles)
	}
	if len(modules[0].TopFiles) != maxRootPackageTopFiles {
		t.Fatalf("len(module.TopFiles) = %d, want %d", len(modules[0].TopFiles), maxRootPackageTopFiles)
	}

	got := []string{}
	for _, file := range rootPkg.TopFiles {
		got = append(got, file.Path)
	}
	want := []string{
		"package.json",
		"jsconfig.json",
		"tsconfig.json",
		"server.js",
		"main.js",
		"index.js",
	}
	for i, path := range want {
		if got[i] != path {
			t.Fatalf("rootPkg.TopFiles[%d] = %q, want %q; full order = %v", i, got[i], path, got)
		}
	}
}

func TestNodeDetectorSurfacesReactAndNextOverviewCandidates(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"web-ui",
  "dependencies":{"react":"^18.0.0","next":"^15.0.0"}
}`)
	writeTestFile(t, filepath.Join(tmpDir, "next.config.js"), "module.exports = {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "App.tsx"), "export const App = () => null\n")
	writeTestFile(t, filepath.Join(tmpDir, "components", "Button.tsx"), "export const Button = () => null\n")
	writeTestFile(t, filepath.Join(tmpDir, "hooks", "useThing.ts"), "export const useThing = () => true\n")
	writeTestFile(t, filepath.Join(tmpDir, "pages", "index.tsx"), "export default function Page() { return null }\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	for _, path := range []string{
		"package.json",
		"next.config.js",
		filepath.Join("src", "App.tsx"),
		filepath.Join("components", "Button.tsx"),
		filepath.Join("pages", "index.tsx"),
	} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
}

func TestNodeDetectorSurfacesNestAndViteOverviewCandidates(t *testing.T) {
	tmpDir := t.TempDir()

	writeTestFile(t, filepath.Join(tmpDir, "package.json"), `{
  "name":"mono-app",
  "dependencies":{"@nestjs/core":"^11.0.0"},
  "devDependencies":{"vite":"^5.0.0"}
}`)
	writeTestFile(t, filepath.Join(tmpDir, "nest-cli.json"), "{}\n")
	writeTestFile(t, filepath.Join(tmpDir, "vite.config.ts"), "export default {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "src", "main.ts"), "export const main = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "apps", "api", "src", "main.ts"), "export const apiMain = true\n")
	writeTestFile(t, filepath.Join(tmpDir, "libs", "shared", "src", "index.ts"), "export const shared = true\n")

	modules, err := NewNodeDetector().Detect(tmpDir)
	if err != nil {
		t.Fatalf("Detect() error = %v", err)
	}
	rootPkg := findPackage(modules[0], "")
	if rootPkg == nil {
		t.Fatalf("module packages = %+v, missing root package summary", modules[0].SourceRoots)
	}

	for _, path := range []string{
		"package.json",
		"nest-cli.json",
		"vite.config.ts",
		filepath.Join("src", "main.ts"),
		filepath.Join("apps", "api", "src", "main.ts"),
		filepath.Join("libs", "shared", "src", "index.ts"),
	} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
}

func findSourceRoot(module model.Module, path string) *model.SourceRoot {
	for _, sr := range module.SourceRoots {
		if sr.Path == path {
			sr := sr
			return &sr
		}
	}
	return nil
}
