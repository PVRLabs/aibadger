package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestConfiguredPackageTopFiles(t *testing.T) {
	for _, language := range []struct{ name, marker, content, extension string }{
		{"Go", "go.mod", "module example.com/test\n\ngo 1.24\n", "go"},
		{"Java", "pom.xml", "<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1</version></project>", "java"},
		{"CSharp", "test.csproj", `<Project Sdk="Microsoft.NET.Sdk"></Project>`, "cs"},
		{"Cpp", "CMakeLists.txt", "project(test)\n", "cpp"},
		{"Node", "package.json", `{"name":"test"}`, "js"},
		{"Python", "pyproject.toml", "[project]\nname = 'test'\nversion = '1.0'\n", "py"},
	} {
		t.Run(language.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, language.marker), []byte(language.content), 0600); err != nil {
				t.Fatal(err)
			}
			packagePath := "src"
			if language.name == "Go" {
				packagePath = "internal"
			}
			if err := os.Mkdir(filepath.Join(root, packagePath), 0700); err != nil {
				t.Fatal(err)
			}
			prefix := ""
			if language.name == "Go" {
				prefix = "package sample\n"
			}
			for i := 0; i < 12; i++ {
				if err := os.WriteFile(filepath.Join(root, packagePath, fmt.Sprintf("file%02d.%s", i, language.extension)), []byte(prefix+fmt.Sprintf("// %s\n", strings.Repeat("x", i+30))), 0600); err != nil {
					t.Fatal(err)
				}
			}
			scan := func(limit int) []string {
				s := NewScanner(root)
				s.MaxTopFilesPerPackage = limit
				topology, err := s.Scan()
				if err != nil {
					t.Fatal(err)
				}
				for _, m := range topology.Modules {
					if m.Path == "" && len(m.TopFiles) > maxRootPackageTopFiles {
						t.Fatalf("root module exceeded overview budget: %d", len(m.TopFiles))
					}
					for _, sr := range m.SourceRoots {
						for _, p := range sr.Packages {
							if p.Path == packagePath {
								var names []string
								for _, f := range p.TopFiles {
									names = append(names, f.Path)
								}
								return names
							}
						}
					}
				}
				t.Fatal("src package missing")
				return nil
			}
			original, explicit, expanded := scan(0), scan(3), scan(8)
			if len(original) != 3 || !reflect.DeepEqual(original, explicit) || len(expanded) != 8 || !reflect.DeepEqual(original, expanded[:3]) {
				t.Fatalf("default %v explicit %v expanded %v", original, explicit, expanded)
			}
		})
	}
}
