package scanner

import (
	"path/filepath"
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
