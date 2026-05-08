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
