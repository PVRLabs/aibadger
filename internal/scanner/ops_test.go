package scanner

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestScanOpsResourcesIncludesRootOpsFiles(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "Makefile"), "test:\n\tgo test ./...\n")
	writeTestFile(t, filepath.Join(tmpDir, "Dockerfile"), "FROM scratch\n")
	writeTestFile(t, filepath.Join(tmpDir, "docker-compose.yml"), "services: {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "run-tests.sh"), "go test ./...\n")
	writeTestFile(t, filepath.Join(tmpDir, ".env.example"), "APP_ENV=test\n")
	writeTestFile(t, filepath.Join(tmpDir, ".env"), "SECRET=hidden\n")
	writeTestFile(t, filepath.Join(tmpDir, "package.json"), "{}\n")

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}

	rootPkg := findOpsPackage(sourceRoots, "")
	if rootPkg == nil {
		t.Fatalf("root ops package missing from sourceRoots: %+v", sourceRoots)
	}
	for _, path := range []string{"Makefile", "Dockerfile", "docker-compose.yml", "run-tests.sh", ".env.example"} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
	for _, path := range []string{".env", "package.json"} {
		if hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, should not include %s", rootPkg.TopFiles, path)
		}
	}
}

func TestScanOpsResourcesIncludesShallowOpsDirs(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "readme.md"), "# Deploy\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "manual-vm-deploy.md"), "# Manual\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "provision_vm.sh"), "provision\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "diagnose_vm.sh"), "diagnose\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "copy_to_vps", "app.sh"), "copy\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "copy_to_vps", "provision_app.sh"), "provision\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "db", "schema.sql"), "create table x(id int);\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "db", "schema-aux.sql"), "create table y(id int);\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "db", "deep", "ignored.sql"), "select 1;\n")

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}

	deployPkg := findOpsPackage(sourceRoots, "deploy")
	if deployPkg == nil {
		t.Fatalf("deploy package missing from sourceRoots: %+v", sourceRoots)
	}
	for _, path := range []string{
		filepath.Join("deploy", "readme.md"),
		filepath.Join("deploy", "manual-vm-deploy.md"),
		filepath.Join("deploy", "provision_vm.sh"),
		filepath.Join("deploy", "diagnose_vm.sh"),
	} {
		if !hasTopFile(deployPkg.TopFiles, path) {
			t.Fatalf("deployPkg.TopFiles = %+v, missing %s", deployPkg.TopFiles, path)
		}
	}

	copyPkg := findOpsPackage(sourceRoots, filepath.Join("deploy", "copy_to_vps"))
	if copyPkg == nil {
		t.Fatalf("nested copy_to_vps package missing from sourceRoots: %+v", sourceRoots)
	}
	if !hasTopFile(copyPkg.TopFiles, filepath.Join("deploy", "copy_to_vps", "app.sh")) {
		t.Fatalf("copyPkg.TopFiles = %+v, missing app.sh", copyPkg.TopFiles)
	}

	dbPkg := findOpsPackage(sourceRoots, filepath.Join("deploy", "db"))
	if dbPkg == nil {
		t.Fatalf("nested db package missing from sourceRoots: %+v", sourceRoots)
	}
	if !hasTopFile(dbPkg.TopFiles, filepath.Join("deploy", "db", "schema.sql")) {
		t.Fatalf("dbPkg.TopFiles = %+v, missing schema.sql", dbPkg.TopFiles)
	}
	if findOpsPackage(sourceRoots, filepath.Join("deploy", "db", "deep")) != nil {
		t.Fatalf("sourceRoots = %+v, should not recurse beyond one nested level", sourceRoots)
	}
}

func TestScanOpsResourcesIncludesGitHubWorkflows(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"), "name: ci\n")
	writeTestFile(t, filepath.Join(tmpDir, ".github", "workflows", "release.yaml"), "name: release\n")
	writeTestFile(t, filepath.Join(tmpDir, ".github", "actions", "setup.yml"), "name: setup\n")

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}

	workflowPath := filepath.Join(".github", "workflows")
	workflowPkg := findOpsPackage(sourceRoots, workflowPath)
	if workflowPkg == nil {
		t.Fatalf(".github/workflows package missing from sourceRoots: %+v", sourceRoots)
	}
	for _, path := range []string{
		filepath.Join(workflowPath, "ci.yml"),
		filepath.Join(workflowPath, "release.yaml"),
	} {
		if !hasTopFile(workflowPkg.TopFiles, path) {
			t.Fatalf("workflowPkg.TopFiles = %+v, missing %s", workflowPkg.TopFiles, path)
		}
	}
	if findOpsPackage(sourceRoots, filepath.Join(".github", "actions")) != nil {
		t.Fatalf("sourceRoots = %+v, should not include .github/actions", sourceRoots)
	}
}

func TestScanOpsResourcesRecognizesOpsFileTypesAndDirs(t *testing.T) {
	tmpDir := t.TempDir()
	files := []string{
		filepath.Join("infra", "main.tf"),
		filepath.Join("k8s", "deployment.yaml"),
		filepath.Join("helm", "values.yml"),
		filepath.Join("ansible", "playbook.yml"),
		filepath.Join("systemd", "app.service"),
		filepath.Join("systemd", "backup.timer"),
		filepath.Join("release", "publish.ts"),
		filepath.Join("provision", "setup.rb"),
		filepath.Join("ops", "status.pl"),
		filepath.Join("ci", "check.ps1"),
	}
	for _, relPath := range files {
		writeTestFile(t, filepath.Join(tmpDir, relPath), "ops\n")
	}

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}
	for _, relPath := range files {
		pkg := findOpsPackage(sourceRoots, filepath.Dir(relPath))
		if pkg == nil {
			t.Fatalf("sourceRoots = %+v, missing package for %s", sourceRoots, relPath)
		}
		if !hasTopFile(pkg.TopFiles, relPath) {
			t.Fatalf("%s TopFiles = %+v, missing %s", pkg.Path, pkg.TopFiles, relPath)
		}
	}
}

func TestScanOpsResourcesExcludesBroadScriptsBinAndNoise(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "scripts", "deploy.sh"), "deploy\n")
	writeTestFile(t, filepath.Join(tmpDir, "bin", "deploy.sh"), "deploy\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", ".DS_Store"), "junk\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "archive.zip"), "zip\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "diagram.png"), "png\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "schema.generated.sql"), "select 1;\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "run.sh"), "run\n")

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}

	if findOpsPackage(sourceRoots, "scripts") != nil {
		t.Fatalf("sourceRoots = %+v, should not include broad scripts dir", sourceRoots)
	}
	if findOpsPackage(sourceRoots, "bin") != nil {
		t.Fatalf("sourceRoots = %+v, should not include broad bin dir", sourceRoots)
	}

	deployPkg := findOpsPackage(sourceRoots, "deploy")
	if deployPkg == nil {
		t.Fatalf("deploy package missing from sourceRoots: %+v", sourceRoots)
	}
	if !hasTopFile(deployPkg.TopFiles, filepath.Join("deploy", "run.sh")) {
		t.Fatalf("deployPkg.TopFiles = %+v, missing run.sh", deployPkg.TopFiles)
	}
	for _, path := range []string{
		filepath.Join("deploy", ".DS_Store"),
		filepath.Join("deploy", "archive.zip"),
		filepath.Join("deploy", "diagram.png"),
		filepath.Join("deploy", "schema.generated.sql"),
	} {
		if hasTopFile(deployPkg.TopFiles, path) || hasAuxFile(deployPkg.AuxFiles, path) {
			t.Fatalf("deploy package surfaced omitted file %s: top=%+v aux=%+v", path, deployPkg.TopFiles, deployPkg.AuxFiles)
		}
	}
}

func TestScanOpsResourcesCapsAndOrdersDeterministically(t *testing.T) {
	tmpDir := t.TempDir()
	for _, name := range []string{
		"zzz.sh",
		"aaa.sh",
		"manual-deploy.md",
		"runbook.md",
		"backup.sh",
		"status.sh",
		"deploy.sh",
		"release.sh",
	} {
		writeTestFile(t, filepath.Join(tmpDir, "deploy", name), "ops\n")
	}

	sourceRoots, err := scanOpsResources(tmpDir)
	if err != nil {
		t.Fatalf("scanOpsResources() error = %v", err)
	}
	deployPkg := findOpsPackage(sourceRoots, "deploy")
	if deployPkg == nil {
		t.Fatalf("deploy package missing from sourceRoots: %+v", sourceRoots)
	}
	if len(deployPkg.TopFiles) != maxOpsPackageFiles {
		t.Fatalf("len(deployPkg.TopFiles) = %d, want %d: %+v", len(deployPkg.TopFiles), maxOpsPackageFiles, deployPkg.TopFiles)
	}
	if deployPkg.TopFiles[0].Path != filepath.Join("deploy", "manual-deploy.md") {
		t.Fatalf("first deploy top file = %q, want manual-deploy.md: %+v", deployPkg.TopFiles[0].Path, deployPkg.TopFiles)
	}
}

func TestScanAttachesOpsResourcesToLanguageTopology(t *testing.T) {
	tmpDir := t.TempDir()
	writeTestFile(t, filepath.Join(tmpDir, "go.mod"), "module example.com/ops\n")
	writeTestFile(t, filepath.Join(tmpDir, "main.go"), "package main\n\nfunc main() {}\n")
	writeTestFile(t, filepath.Join(tmpDir, "Makefile"), "test:\n\tgo test ./...\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "provision.sh"), "provision\n")
	writeTestFile(t, filepath.Join(tmpDir, "deploy", "db", "schema.sql"), "create table x(id int);\n")
	writeTestFile(t, filepath.Join(tmpDir, ".github", "workflows", "ci.yml"), "name: ci\n")

	topology, err := NewScanner(tmpDir).Scan()
	if err != nil {
		t.Fatalf("Scan() error = %v", err)
	}
	if !reflect.DeepEqual(topology.Languages, []string{"Go"}) {
		t.Fatalf("Languages = %v, want [Go]", topology.Languages)
	}
	if topology.PrimaryLanguage != "Go" {
		t.Fatalf("PrimaryLanguage = %q, want Go", topology.PrimaryLanguage)
	}
	if len(topology.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(topology.Modules))
	}

	module := topology.Modules[0]
	rootPkg := findPackage(module, "")
	if rootPkg == nil {
		t.Fatalf("root package missing from source roots: %+v", module.SourceRoots)
	}
	if countPackages(module, "") != 1 {
		t.Fatalf("root package should not be duplicated: %+v", module.SourceRoots)
	}
	for _, path := range []string{"main.go", "Makefile"} {
		if !hasTopFile(rootPkg.TopFiles, path) {
			t.Fatalf("rootPkg.TopFiles = %+v, missing %s", rootPkg.TopFiles, path)
		}
	}
	for _, path := range []string{
		filepath.Join("deploy", "provision.sh"),
		filepath.Join("deploy", "db", "schema.sql"),
		filepath.Join(".github", "workflows", "ci.yml"),
	} {
		if !moduleHasPackageTopFile(module, path) {
			t.Fatalf("module packages = %+v, missing ops file %s", module.SourceRoots, path)
		}
	}
}

func TestScanKeepsOpsScriptsLanguageNeutralAcrossDetectors(t *testing.T) {
	tests := []struct {
		name              string
		files             map[string]string
		expectedLanguages []string
		expectedPrimary   string
		expectedStack     []string
	}{
		{
			name: "java",
			files: map[string]string{
				"pom.xml": "<project><groupId>com.example</groupId><artifactId>app</artifactId></project>",
				filepath.Join("src", "main", "java", "App.java"): "class App {}\n",
			},
			expectedLanguages: []string{"Java"},
			expectedPrimary:   "Java",
			expectedStack:     []string{"Maven"},
		},
		{
			name: "go",
			files: map[string]string{
				"go.mod":  "module example.com/app\n",
				"main.go": "package main\n\nfunc main() {}\n",
			},
			expectedLanguages: []string{"Go"},
			expectedPrimary:   "Go",
			expectedStack:     []string{"Go Modules"},
		},
		{
			name: "node",
			files: map[string]string{
				"package.json":                   `{"name":"web","main":"src/index.js"}`,
				filepath.Join("src", "index.js"): "export const app = true\n",
			},
			expectedLanguages: []string{"JavaScript"},
			expectedPrimary:   "JavaScript",
			expectedStack:     []string{"Node.js"},
		},
		{
			name: "python",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = \"app\"\n",
				filepath.Join("src", "app", "__init__.py"): "",
				filepath.Join("src", "app", "main.py"):     "def main(): return True\n",
			},
			expectedLanguages: []string{"Python"},
			expectedPrimary:   "Python",
			expectedStack:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			for relPath, contents := range tt.files {
				writeTestFile(t, filepath.Join(tmpDir, relPath), contents)
			}
			writeTestFile(t, filepath.Join(tmpDir, "deploy", "status.py"), "print('ok')\n")
			writeTestFile(t, filepath.Join(tmpDir, "deploy", "release.ts"), "console.log('release')\n")
			writeTestFile(t, filepath.Join(tmpDir, "deploy", "package.json"), `{"name":"deploy-tools","scripts":{"release":"node release.js"}}`)
			writeTestFile(t, filepath.Join(tmpDir, "deploy", "release.js"), "console.log('release')\n")
			writeTestFile(t, filepath.Join(tmpDir, "Makefile"), "deploy:\n\ttrue\n")

			topology, err := NewScanner(tmpDir).Scan()
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if !reflect.DeepEqual(topology.Languages, tt.expectedLanguages) {
				t.Fatalf("Languages = %v, want %v", topology.Languages, tt.expectedLanguages)
			}
			if topology.PrimaryLanguage != tt.expectedPrimary {
				t.Fatalf("PrimaryLanguage = %q, want %s", topology.PrimaryLanguage, tt.expectedPrimary)
			}
			if !reflect.DeepEqual(topology.Stack, tt.expectedStack) {
				t.Fatalf("Stack = %v, want %v", topology.Stack, tt.expectedStack)
			}
			if len(topology.Modules) != 1 {
				t.Fatalf("len(Modules) = %d, want 1; modules=%+v", len(topology.Modules), topology.Modules)
			}
			if !moduleHasPackageTopFile(topology.Modules[0], filepath.Join("deploy", "status.py")) {
				t.Fatalf("ops Python script missing from supplemental package: %+v", topology.Modules[0].SourceRoots)
			}
		})
	}
}

func findOpsPackage(sourceRoots []model.SourceRoot, path string) *model.Package {
	for sourceRootIdx := range sourceRoots {
		for packageIdx := range sourceRoots[sourceRootIdx].Packages {
			if sourceRoots[sourceRootIdx].Packages[packageIdx].Path == path {
				return &sourceRoots[sourceRootIdx].Packages[packageIdx]
			}
		}
	}
	return nil
}

func countPackages(module model.Module, path string) int {
	count := 0
	for _, sourceRoot := range module.SourceRoots {
		for _, pkg := range sourceRoot.Packages {
			if pkg.Path == path {
				count++
			}
		}
	}
	return count
}
