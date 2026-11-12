package scanner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

func htmlFixture(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestSupplementalHTMLFinalTopology(t *testing.T) {
	for _, node := range []bool{false, true} {
		t.Run(map[bool]string{false: "python", true: "node"}[node], func(t *testing.T) {
			root := t.TempDir()
			if node {
				htmlFixture(t, root, "package.json", `{"name":"fixture","dependencies":{"express":"1"}}`)
				htmlFixture(t, root, "server.js", "console.log(1)")
			} else {
				htmlFixture(t, root, "tool.py", "print(1)")
				htmlFixture(t, root, "script.js", "console.log(1)")
			}
			before, err := NewScanner(root).Scan()
			if err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"index.html", "articles/index.htm", "articles/index.html", "articles/a.html", "articles/b.html", "articles/c.html", "other/a.html", "public/visible.html", "robots.txt"} {
				htmlFixture(t, root, path, strings.Repeat("x", 1000))
			}
			// Entrypoints are deliberately smaller than their competitors.
			htmlFixture(t, root, "articles/index.html", "x")
			htmlFixture(t, root, "articles/index.htm", "x")
			topology, err := NewScanner(root).Scan()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before.Languages, topology.Languages) || before.PrimaryLanguage != topology.PrimaryLanguage {
				t.Fatalf("languages changed: %v => %v", before.Languages, topology.Languages)
			}
			count := 0
			main, web := false, false
			for _, m := range topology.Modules {
				for _, sr := range m.SourceRoots {
					if sr.Path == "" && sr.Role == "Main Source" {
						main = true
					}
					if !sr.SupplementalHTML {
						continue
					}
					web = true
					count += sr.FileCount
					for _, p := range sr.Packages {
						if p.Path == "articles" {
							want := []string{"articles/index.htm", "articles/index.html", "articles/a.html"}
							var got []string
							for _, f := range p.TopFiles {
								got = append(got, f.Path)
							}
							if !reflect.DeepEqual(got, want) {
								t.Fatalf("HTML ranking: %v", got)
							}
						}
					}
				}
			}
			if count != 7 || !web || (!node && !main) {
				t.Fatalf("count=%d main=%v web=%v", count, main, web)
			}
			f := protocol.NewFormatter()
			f.MaxTopologyPromptBytes = 0
			output := f.GenerateSchemaA(topology, "")
			for _, path := range []string{"index.html", "articles", "index.htm", "visible.html"} {
				if !strings.Contains(output, path) {
					t.Fatalf("missing %s: %s", path, output)
				}
			}
			rootCount := 0
			for _, m := range topology.Modules {
				for _, sr := range m.SourceRoots {
					for _, p := range sr.Packages {
						for _, file := range p.TopFiles {
							if file.Path == "index.html" {
								rootCount++
							}
						}
					}
				}
			}
			if rootCount != 1 {
				t.Fatalf("root HTML appears %d times", rootCount)
			}
		})
	}
}

func TestSupplementalHTMLTraversal(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"aaa/htmlcov/one.html", "aaa/htmlcov/two.html", "aaa/htmlcov/three.html", "articles/INDEX.HTM", "docs/report.html", "examples/template.HTML"} {
		htmlFixture(t, root, path, "x")
	}
	if err := os.Symlink(filepath.Join(root, "docs/report.html"), filepath.Join(root, "linked.html")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "docs"), filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	det := NewGenericDetector()
	det.maxTotalFiles = 3
	roots, err := det.collectSupplementalHTML(root, newSemanticSourceOwnership(root, nil))
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].FileCount != 3 {
		t.Fatalf("pruning/eligibility: %+v", roots)
	}
	htmlFixture(t, root, "000.txt", "x")
	det.maxTotalFiles = 1
	roots, err = det.collectSupplementalHTML(root, newSemanticSourceOwnership(root, nil))
	if err != nil || len(roots) != 0 {
		t.Fatalf("non HTML budget: %+v %v", roots, err)
	}
}

func TestSupplementalHTMLDelegation(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"public/a.html", "module/static/a.html", "module/templates/a.html"} {
		htmlFixture(t, root, path, "x")
	}
	det := NewGenericDetector()
	ownership := newSemanticSourceOwnership(root, []model.Module{{Language: "Go", Path: "module"}})
	roots, err := det.collectSupplementalHTML(root, ownership)
	if err != nil || len(roots) != 1 || roots[0].FileCount != 1 || roots[0].Packages[0].Path != "module/templates" {
		t.Fatalf("delegation: %+v %v", roots, err)
	}
}

func TestSupplementalHTMLFallbackUnchanged(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"index.html", "articles/story.html", "public/visible.html"} {
		htmlFixture(t, root, path, "x")
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	output := protocol.NewFormatter().GenerateSchemaA(topology, "")
	for _, path := range []string{"index.html", "story.html", "visible.html"} {
		if !strings.Contains(output, path) {
			t.Fatalf("missing fallback page %s: %s", path, output)
		}
	}
	for _, m := range topology.Modules {
		for _, sr := range m.SourceRoots {
			if sr.SupplementalHTML {
				t.Fatal("supplemental traversal applied to fallback")
			}
		}
	}
}

func TestSupplementalHTMLHiddenNodeAccounting(t *testing.T) {
	root := t.TempDir()
	htmlFixture(t, root, "package.json", `{"name":"fixture"}`)
	htmlFixture(t, root, "index.html", "x")
	topology := &model.ProjectTopology{Modules: []model.Module{{Language: "JavaScript", SourceRoots: []model.SourceRoot{{Role: "Main Source", FileCount: 2, Packages: []model.Package{{Path: "", FileCount: 2, TopFiles: []model.FileSummary{{Name: "package.json", Path: "package.json"}}}}}}}}}
	transferNodeHTML(topology, root, map[string]bool{"index.html": true})
	sr := topology.Modules[0].SourceRoots[0]
	if sr.FileCount != 1 || sr.Packages[0].FileCount != 1 {
		t.Fatalf("hidden overlap remains counted: %+v", sr)
	}
}

func TestSupplementalHTMLPerDirectoryBudget(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"htmlcov/one.html", "htmlcov/two.html", "pages/a.txt", "pages/b.html", "pages/c.html"} {
		htmlFixture(t, root, path, "x")
	}
	det := NewGenericDetector()
	det.maxFilesPerDir = 2
	roots, err := det.collectSupplementalHTML(root, newSemanticSourceOwnership(root, nil))
	if err != nil || len(roots) != 1 || roots[0].FileCount != 1 || roots[0].Packages[0].TopFiles[0].Name != "b.html" {
		t.Fatalf("per directory budget: %+v %v", roots, err)
	}
}

func TestNodeHTMLTransferPreservesGenericCoverage(t *testing.T) {
	root := t.TempDir()
	htmlFixture(t, root, "package.json", `{"name":"fixture","dependencies":{"express":"1"}}`)
	htmlFixture(t, root, "index.html", "x")
	htmlFixture(t, root, "unclaimed.js", "console.log(1)")
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range topology.Modules {
		if !m.Coverage {
			continue
		}
		for _, sr := range m.SourceRoots {
			for _, pkg := range sr.Packages {
				for _, file := range pkg.TopFiles {
					if file.Path == "unclaimed.js" {
						found = true
						if pkg.FileCount < 1 || sr.FileCount < 1 {
							t.Fatal("coverage counts were decremented")
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("generic JS coverage lost")
	}
}

func TestNodeHTMLMixedCaseOverlap(t *testing.T) {
	root := t.TempDir()
	htmlFixture(t, root, "package.json", `{"name":"fixture","dependencies":{"express":"1"}}`)
	htmlFixture(t, root, "INDEX.HTML", "x")
	actual, err := os.Stat(filepath.Join(root, "INDEX.HTML"))
	if err != nil {
		t.Fatal(err)
	}
	alias, err := os.Stat(filepath.Join(root, "index.html"))
	insensitive := err == nil && os.SameFile(actual, alias)
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	count, visible := 0, 0
	for _, m := range topology.Modules {
		for _, sr := range m.SourceRoots {
			for _, pkg := range sr.Packages {
				if pkg.Path == "" {
					count += pkg.FileCount
				}
				for _, file := range pkg.TopFiles {
					if strings.EqualFold(file.Path, "index.html") {
						visible++
					}
				}
			}
		}
	}
	if count != 2 || visible != 1 {
		t.Fatalf("case insensitive=%v root count=%d visible=%d", insensitive, count, visible)
	}
	// Exercise equivalent candidate spellings on every filesystem using a hard link.
	if !insensitive {
		if err := os.Link(filepath.Join(root, "INDEX.HTML"), filepath.Join(root, "index.html")); err != nil {
			t.Fatal(err)
		}
	}
	synthetic := &model.ProjectTopology{Modules: []model.Module{{Language: "JavaScript", SourceRoots: []model.SourceRoot{{Role: "Main Source", FileCount: 2, Packages: []model.Package{{FileCount: 2, TopFiles: []model.FileSummary{{Path: "index.html", Name: "index.html"}}}}}}}}}
	transferNodeHTML(synthetic, root, map[string]bool{"INDEX.HTML": true})
	pkg := synthetic.Modules[0].SourceRoots[0].Packages[0]
	if pkg.FileCount != 1 || len(pkg.TopFiles) != 0 {
		t.Fatalf("equivalent path overlap: %+v", pkg)
	}
}

func TestNodeHTMLTransferPreservesRootWebResources(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{"package.json": `{"name":"fixture","dependencies":{"express":"1"}}`, "index.html": "x", "robots.txt": "User-agent: *"} {
		htmlFixture(t, root, path, content)
	}
	topology, err := NewScanner(root).Scan()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range topology.Modules {
		for _, sr := range m.SourceRoots {
			if sr.Role != "Web Resources" || sr.SupplementalHTML {
				continue
			}
			for _, pkg := range sr.Packages {
				for _, file := range pkg.AuxFiles {
					if file.Path == "robots.txt" {
						found = true
						if pkg.FileCount != 1 || sr.FileCount != 1 {
							t.Fatalf("resource counts: %+v %+v", sr, pkg)
						}
					}
				}
			}
		}
	}
	if !found {
		t.Fatal("robots.txt pruned")
	}
	if !strings.Contains(protocol.NewFormatter().GenerateSchemaA(topology, ""), "robots.txt") {
		t.Fatal("robots.txt absent from source tree")
	}
}

func TestSupplementalHTMLConventionalDirectoryCase(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		t.Run(map[bool]string{false: "shared", true: "delegated"}[delegated], func(t *testing.T) {
			root := t.TempDir()
			prefix := ""
			var modules []model.Module
			if delegated {
				prefix = "module"
				modules = []model.Module{{Language: "Go", Path: prefix}}
			}
			actual := filepath.Join(prefix, "Public")
			htmlFixture(t, root, filepath.Join(actual, "page.html"), "x")
			upper, err := os.Stat(filepath.Join(root, actual))
			if err != nil {
				t.Fatal(err)
			}
			lowerPath := filepath.Join(root, prefix, "public")
			lower, err := os.Stat(lowerPath)
			same := err == nil && os.SameFile(upper, lower)
			ownership := newSemanticSourceOwnership(root, modules)
			roots, err := NewGenericDetector().collectSupplementalHTML(root, ownership)
			if err != nil {
				t.Fatal(err)
			}
			if same {
				if len(roots) != 0 {
					t.Fatalf("equivalent directory scanned twice: %+v", roots)
				}
			} else {
				if len(roots) != 1 || roots[0].FileCount != 1 {
					t.Fatalf("distinct uppercase directory lost: %+v", roots)
				}
				htmlFixture(t, root, filepath.Join(prefix, "public", "other.html"), "x")
				roots, err = NewGenericDetector().collectSupplementalHTML(root, ownership)
				if err != nil || len(roots) != 1 || roots[0].FileCount != 1 {
					t.Fatalf("distinct directories conflated: %+v %v", roots, err)
				}
			}
		})
	}
}
