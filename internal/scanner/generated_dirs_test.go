package scanner

import (
	"fmt"
	"github.com/PVRLabs/aibadger/internal/dirpolicy"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestGeneratedDirectoriesExcludedAcrossTopology(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".playwright-cli", ".cache", ".next", ".pytest_cache", "test-results", ".gradle"} {
		for _, prefix := range []string{"", "src", "public"} {
			base := filepath.Join(prefix, dir)
			for name, content := range map[string]string{"noise.js": "console.log(1)", "noise.py": "print(1)", "noise.html": "<html/>", "noise.json": "{}", "noise.md": "# Generated", "package.json": `{"name":"generated","dependencies":{"express":"1"}}`} {
				htmlFixture(t, root, filepath.Join(base, name), content)
			}
		}
	}
	htmlFixture(t, root, "tool.py", "print(1)")
	htmlFixture(t, root, "src/kept.js", "console.log(1)")
	htmlFixture(t, root, "public/kept.html", "<html/>")
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, module := range topology.Modules {
		if isUnderIgnoredDir(root, filepath.Join(root, module.Path, "probe"), nil) {
			t.Fatalf("generated module: %s", module.Path)
		}
		for _, sr := range module.SourceRoots {
			for _, pkg := range sr.Packages {
				if isUnderIgnoredDir(root, filepath.Join(root, pkg.Path, "probe"), nil) {
					t.Fatalf("generated package: %s", pkg.Path)
				}
			}
		}
	}
	output := protocol.NewFormatter().GenerateSchemaA(topology, "")
	if strings.Contains(output, "noise.") || !strings.Contains(output, "kept.js") || !strings.Contains(output, "kept.html") {
		t.Fatalf("generated output/source selection: %s", output)
	}
}

func TestJavaSourceWalksSkipGeneratedDirectories(t *testing.T) {
	root := t.TempDir()
	htmlFixture(t, root, "pom.xml", "<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1</version></project>")
	htmlFixture(t, root, "src/main/java/Keep.java", "class Keep {}")
	htmlFixture(t, root, "src/main/java/.playwright-cli/Noise.java", "class Noise {}")
	htmlFixture(t, root, "src/main/java/.cache/noise.json", "{}")
	modules, err := NewJavaDetector().Detect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(modules) == 0 {
		t.Fatal("missing Java module")
	}
	for _, m := range modules {
		for _, file := range m.TopFiles {
			if strings.Contains(file.Path, "Noise") {
				t.Fatal("Java top files include generated source")
			}
		}
		for _, sr := range m.SourceRoots {
			for _, pkg := range sr.Packages {
				if strings.Contains(pkg.Path, ".cache") || strings.Contains(pkg.Path, ".playwright-cli") {
					t.Fatalf("Java generated package: %s", pkg.Path)
				}
			}
		}
	}
}

func TestConfiguredDirectoryPolicy(t *testing.T) {
	for _, specialized := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			name := fmt.Sprintf("specialized=%v/generated=%v", specialized, enabled)
			t.Run(name, func(t *testing.T) {
				root := t.TempDir()
				if specialized {
					htmlFixture(t, root, "tool.py", "print(1)")
				}
				for _, path := range []string{"build/keep.html", "nested/.cache/keep.html", "htmlcov/keep.html", ".my-cache/hidden.html", "nested/.MY-CACHE/hidden.html", "custom-output/hidden.js", "nested/custom-output/hidden.html", ".git/hidden.html", "docs/visible.html"} {
					htmlFixture(t, root, path, "x")
				}
				policy := dirpolicy.New(&enabled, []string{".my-cache", "custom-output"})
				topology, err := NewScanner(root, policy).Scan()
				if err != nil {
					t.Fatal(err)
				}
				output := protocol.NewFormatter().GenerateSchemaA(topology, "")
				if strings.Contains(output, "hidden") || strings.Contains(output, ".git") || !strings.Contains(output, "visible.html") {
					t.Fatalf("custom/mandatory filtering: %s", output)
				}
				if strings.Contains(output, "keep.html") == enabled {
					t.Fatalf("generated toggle not applied before scanning: %s", output)
				}
			})
		}
	}
}

func TestAdditionalExclusionsCoverDirectSourceRootProbes(t *testing.T) {
	for _, tc := range []struct{ name, marker, content, outside, inside, excluded string }{
		{"Go", "go.mod", "module example.test/fixture\n", "main.go", "internal/hidden.go", "internal"},
		{"Java", "pom.xml", "<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1</version></project>", "Main.java", "src/main/java/Hidden.java", "src"},
		{"Python", "pyproject.toml", "[project]\nname='fixture'\nversion='1'", "tool.py", "src/hidden.py", "src"},
		{"Node", "package.json", `{"name":"fixture","dependencies":{"express":"1"}}`, "server.js", "src/hidden.js", "src"},
		{"CSharp", "App.csproj", "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><TargetFramework>net8.0</TargetFramework></PropertyGroup></Project>", "Program.cs", "src/Hidden.cs", "src"},
		{"Cpp", "CMakeLists.txt", "project(fixture)", "main.cpp", "src/hidden.cpp", "src"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			htmlFixture(t, root, tc.marker, tc.content)
			htmlFixture(t, root, tc.outside, "source")
			htmlFixture(t, root, tc.inside, "source")
			enabled := false
			policy := dirpolicy.New(&enabled, []string{tc.excluded})
			topology, err := NewScanner(root, policy).Scan()
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range topology.Modules {
				for _, sr := range m.SourceRoots {
					for _, pkg := range sr.Packages {
						if pkg.Path == tc.excluded || strings.HasPrefix(filepath.ToSlash(pkg.Path), tc.excluded+"/") {
							t.Fatalf("excluded source root package survived: %+v", pkg)
						}
					}
				}
			}
			if strings.Contains(strings.ToLower(protocol.NewFormatter().GenerateSchemaA(topology, "")), "hidden.") {
				t.Fatal("excluded source root file survived")
			}
		})
	}
}
