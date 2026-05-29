package filegroups

import (
	"path/filepath"
	"testing"
)

func TestFileGroupMembership(t *testing.T) {
	tests := []struct {
		name string
		fn   func(string) bool
		in   []string
		out  []string
	}{
		{
			name: "critical guidance docs",
			fn:   IsCriticalGuidanceDoc,
			in:   []string{"agents.md", "readme.md", "contributing.md", "claude.md", "gemini.md", "codex.md"},
			out:  []string{"security.md", "package.json", "dockerfile"},
		},
		{
			name: "identity manifests",
			fn:   IsIdentityManifest,
			in:   []string{"package.json", "go.mod", "pom.xml", "build.gradle", "build.gradle.kts", "pyproject.toml", "cargo.toml"},
			out:  []string{"go.sum", "makefile", "readme.md"},
		},
		{
			name: "operational config",
			fn:   IsOperationalConfigFile,
			in:   []string{"tsconfig.json", "vite.config.ts", "next.config.mjs", "dockerfile", "docker-compose.yml", "makefile", "taskfile.yaml", "justfile", "go.sum", "requirements.txt", "cmakelists.txt"},
			out:  []string{"package.json", "go.mod", "readme.md"},
		},
		{
			name: "architecture docs",
			fn:   IsArchitectureLikeDoc,
			in:   []string{"spec.md", "architecture-overview.md", "design-notes.md", "ui-spec.md"},
			out:  []string{"api.md", "setup.md", "plan.md"},
		},
		{
			name: "planning artifacts",
			fn:   IsPlanningArtifactDoc,
			in:   []string{"plan.md", "devlog.md", "work-journal.md"},
			out:  []string{"spec.md", "api.md", "readme.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range tt.in {
				if !tt.fn(name) {
					t.Fatalf("%s should match", name)
				}
			}
			for _, name := range tt.out {
				if tt.fn(name) {
					t.Fatalf("%s should not match", name)
				}
			}
		})
	}
}

func TestPathGroupMembership(t *testing.T) {
	for _, path := range []string{
		"readme.md",
		filepath.Join("docs", "spec.md"),
		filepath.Join("doc", "api.md"),
	} {
		if !IsShallowDocumentationPath(path) {
			t.Fatalf("%s should be shallow documentation", path)
		}
	}
	for _, path := range []string{
		filepath.Join("docs", "deep", "nested.md"),
		filepath.Join("doc", "archive", "old.md"),
		filepath.Join("src", "readme.md"),
	} {
		if IsShallowDocumentationPath(path) {
			t.Fatalf("%s should not be shallow documentation", path)
		}
	}

	for _, path := range []string{
		filepath.Join("public", "app.js"),
		filepath.Join("static", "style.css"),
		filepath.Join("assets", "logo.png"),
		filepath.Join("src", "main", "resources", "static", "app.js"),
	} {
		if !IsKnownStaticWebPath(path) {
			t.Fatalf("%s should be static web path", path)
		}
	}
	for _, path := range []string{
		filepath.Join("src", "app.js"),
		filepath.Join("internal", "assets.go"),
	} {
		if IsKnownStaticWebPath(path) {
			t.Fatalf("%s should not be static web path", path)
		}
	}
}

func TestOpsTopLevelDirNames(t *testing.T) {
	for _, name := range []string{
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
		"DEPLOY",
	} {
		if !IsOpsTopLevelDirName(name) {
			t.Fatalf("%s should be an ops top-level directory name", name)
		}
	}
	for _, name := range []string{"scripts", "bin", "src", "internal", "cmd", "pkg", "node_modules", ".github"} {
		if IsOpsTopLevelDirName(name) {
			t.Fatalf("%s should not be an ops top-level directory name", name)
		}
	}
}

func TestOpsDirectoryPaths(t *testing.T) {
	for _, path := range []string{
		"deploy",
		filepath.Join("terraform"),
		filepath.Join(".github", "workflows"),
		filepath.Join(".GITHUB", "WORKFLOWS"),
		filepath.Join(".gitlab"),
		filepath.Join(".circleci"),
		filepath.Join(".buildkite"),
	} {
		if !IsOpsDirectoryPath(path) {
			t.Fatalf("%s should be an ops directory path", path)
		}
	}
	for _, path := range []string{
		"",
		".",
		filepath.Join(".github"),
		filepath.Join(".github", "actions"),
		filepath.Join("deploy", "db"),
		filepath.Join("scripts"),
		filepath.Join("bin"),
		filepath.Join("src", "deploy"),
	} {
		if IsOpsDirectoryPath(path) {
			t.Fatalf("%s should not be an ops directory path", path)
		}
	}
}

func TestRootOpsFileNames(t *testing.T) {
	for _, name := range []string{
		"run-tests.sh",
		"deploy.bash",
		"setup.zsh",
		"bootstrap.fish",
		"provision.ps1",
		"deploy.bat",
		"deploy.cmd",
		"Makefile",
		"Dockerfile",
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
		"Taskfile.yml",
		"Taskfile.yaml",
		"justfile",
		".env.example",
		".env.template",
		".env.sample",
		".gitlab-ci.yml",
		".gitlab-ci.yaml",
	} {
		if !IsRootOpsFileName(name) {
			t.Fatalf("%s should be a root ops file name", name)
		}
	}
	for _, name := range []string{
		".env",
		"package.json",
		"go.mod",
		"main.py",
		"app.js",
		"README.md",
		"notes.txt",
	} {
		if IsRootOpsFileName(name) {
			t.Fatalf("%s should not be a root ops file name", name)
		}
	}
}

func TestOpsEnvExampleNames(t *testing.T) {
	for _, name := range []string{".env.example", ".env.template", ".env.sample", ".ENV.EXAMPLE"} {
		if !IsOpsEnvExampleName(name) {
			t.Fatalf("%s should be an ops env example", name)
		}
	}
	for _, name := range []string{".env", ".env.local", ".env.production", "env.example"} {
		if IsOpsEnvExampleName(name) {
			t.Fatalf("%s should not be an ops env example", name)
		}
	}
}

func TestOpsContextFileNames(t *testing.T) {
	for _, name := range []string{
		"README.md",
		"manual-vm-deploy.md",
		"runbook.md",
		"provision_vm.sh",
		"setup.bash",
		"install.zsh",
		"bootstrap.fish",
		"deploy.ps1",
		"release.bat",
		"sync.cmd",
		"diagnose.py",
		"health.rb",
		"status.pl",
		"copy.js",
		"publish.ts",
		"copy_to_vps.mjs",
		"release.cjs",
		"schema.sql",
		"app.service",
		"backup.timer",
		"ci.yml",
		"release.yaml",
		"task.json",
		"terraform.tfvars.json",
		"config.toml",
		"main.tf",
		"prod.tfvars",
		"backend.hcl",
		"app.ini",
		"nginx.conf",
		"application.properties",
		".env.example",
		"Dockerfile",
		"docker-compose.yml",
		"compose.yaml",
		"Makefile",
		"Taskfile.yaml",
		"justfile",
		"flake.nix",
		"shell.nix",
	} {
		if !IsOpsContextFileName(name) {
			t.Fatalf("%s should be an ops context file name", name)
		}
	}
	for _, name := range []string{
		".env",
		"app.go",
		"main.java",
		"component.jsx",
		"random.md",
		"image.png",
		"archive.zip",
	} {
		if IsOpsContextFileName(name) {
			t.Fatalf("%s should not be an ops context file name", name)
		}
	}
}

func TestOpsFileRank(t *testing.T) {
	ordered := []string{
		"README.md",
		".env.example",
		"Dockerfile",
		"manual-vm-deploy.md",
		"healthcheck.sh",
		"ci.yml",
		"random.txt",
	}
	for idx := 1; idx < len(ordered); idx++ {
		prev := ordered[idx-1]
		current := ordered[idx]
		if OpsFileRank(prev) >= OpsFileRank(current) {
			t.Fatalf("expected %s to rank before %s", prev, current)
		}
	}
}
