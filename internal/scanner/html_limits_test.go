package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestHTMLPackageTopFileOverrides(t *testing.T) {
	for _, specialized := range []bool{false, true} {
		t.Run(fmt.Sprintf("specialized=%t", specialized), func(t *testing.T) {
			root := t.TempDir()
			write := func(name, body string) {
				t.Helper()
				path := filepath.Join(root, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if specialized {
				write("tools/chart.py", "print(\"chart\")\n")
			}
			for i := 0; i < 25; i++ {
				write(fmt.Sprintf("articles/page%02d.html", i), "<html>"+strings.Repeat("x", i+1)+"</html>")
			}
			write("articles/index.html", "<html>index</html>")
			for i := 0; i < 15; i++ {
				write(fmt.Sprintf("root%02d.html", i), "<html>root</html>")
			}
			scan := func(limit int) ([]string, *model.ProjectTopology) {
				s := NewScanner(root)
				s.MaxTopFilesPerPackage = limit
				topology, err := s.Scan()
				if err != nil {
					t.Fatal(err)
				}
				var names []string
				for _, m := range topology.Modules {
					for _, sr := range m.SourceRoots {
						for _, p := range sr.Packages {
							if p.Path == "articles" {
								for _, f := range p.TopFiles {
									names = append(names, f.Name)
								}
							}
							if p.Path == "" && len(p.TopFiles) > 10 {
								t.Fatal("root budget exceeded")
							}
						}
					}
				}
				return names, topology
			}
			baseline, _ := scan(0)
			wantDefault := 10
			if specialized {
				wantDefault = 3
			}
			if len(baseline) != wantDefault {
				t.Fatalf("default files %d; want %d", len(baseline), wantDefault)
			}
			for _, limit := range []int{3, 8, 20, 30, 50} {
				names, topology := scan(limit)
				want := limit
				if !specialized && want < 10 {
					want = 10
				}
				if want > 26 {
					want = 26
				}
				if len(names) != want {
					t.Fatalf("limit %d returned %d files: %v", limit, len(names), names)
				}
				again, _ := scan(limit)
				if !reflect.DeepEqual(names, again) {
					t.Fatal("nondeterministic selection")
				}
				if specialized && names[0] != "index.html" {
					t.Fatal("HTML index priority lost")
				}
				for _, m := range topology.Modules {
					if len(m.TopFiles) > 10 {
						t.Fatalf("module overview grew to %d", len(m.TopFiles))
					}
				}
			}
		})
	}
}

func TestMixedMarkdownPackageKeepsExpandedBudget(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "App.csproj"), "")
	writeTestFile(t, filepath.Join(root, "Program.cs"), "class Program {}")
	writeTestFile(t, filepath.Join(root, "wwwroot", "README.md"), "# Web resources")
	for i := 0; i < 29; i++ {
		writeTestFile(t, filepath.Join(root, "wwwroot", fmt.Sprintf("page%02d.html", i)), "<html>page</html>")
	}
	for i := 0; i < 12; i++ {
		writeTestFile(t, filepath.Join(root, "docs", fmt.Sprintf("guide%02d.md", i)), "# Guide")
	}
	var previous []string
	for run := 0; run < 16; run++ {
		s := NewScanner(root)
		s.MaxTopFilesPerPackage = 30
		topology, err := s.Scan()
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, m := range topology.Modules {
			for _, sr := range m.SourceRoots {
				for _, p := range sr.Packages {
					if p.Path == "wwwroot" {
						for _, f := range p.TopFiles {
							names = append(names, f.Name)
						}
					}
					if sr.Role == "Documentation" && len(p.TopFiles) > 5 {
						t.Fatalf("documentation budget expanded: %d", len(p.TopFiles))
					}
				}
			}
		}
		if len(names) != 30 {
			t.Fatalf("run %d: mixed package truncated to %d: %v", run, len(names), names)
		}
		foundMarkdown := false
		for _, name := range names {
			if name == "README.md" {
				foundMarkdown = true
			}
		}
		if !foundMarkdown {
			t.Fatal("test did not retain Markdown alongside HTML")
		}
		if previous != nil && !reflect.DeepEqual(previous, names) {
			t.Fatalf("selection changed: %v vs %v", previous, names)
		}
		previous = names
	}
}

func TestGenericFallbackInferredLanguageKeepsPackageBudget(t *testing.T) {
	root := t.TempDir()
	// No project markers: the fallback infers JavaScript from this utility.
	writeTestFile(t, filepath.Join(root, "tools", "chart.js"), "console.log('chart');")
	for i := 0; i < 15; i++ {
		writeTestFile(t, filepath.Join(root, "articles", fmt.Sprintf("page%02d.html", i)), "<html>"+strings.Repeat("x", i+1)+"</html>")
	}
	var baseline []string
	for _, tc := range []struct{ limit, want int }{{0, 10}, {3, 10}, {8, 10}, {30, 15}} {
		t.Run(fmt.Sprintf("limit=%d", tc.limit), func(t *testing.T) {
			s := NewScanner(root)
			s.MaxTopFilesPerPackage = tc.limit
			topology, err := s.Scan()
			if err != nil {
				t.Fatal(err)
			}
			if !s.genericFallback {
				t.Fatal("fixture did not use Generic fallback")
			}
			if len(topology.Modules) != 1 || topology.Modules[0].Language != "JavaScript" {
				t.Fatalf("expected inferred JavaScript fallback: %+v", topology.Modules)
			}
			var names []string
			for _, sr := range topology.Modules[0].SourceRoots {
				for _, p := range sr.Packages {
					if p.Path == "articles" {
						for _, f := range p.TopFiles {
							names = append(names, f.Name)
						}
					}
				}
			}
			if len(names) != tc.want {
				t.Fatalf("got %d files; want %d: %v", len(names), tc.want, names)
			}
			if tc.limit == 0 {
				baseline = names
			} else if !reflect.DeepEqual(baseline, names[:10]) {
				t.Fatalf("default ranking changed: %v vs %v", baseline, names)
			}
		})
	}
}
