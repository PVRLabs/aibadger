package protocol

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/model"
)

func TestSupplementalHTMLPresentationBudget(t *testing.T) {
	source := model.Package{Path: "z-source", FileCount: 1, TopFiles: []model.FileSummary{{Name: "tool.py", Path: "z-source/tool.py"}}}
	conventional := model.Package{Path: "public", FileCount: 1, TopFiles: []model.FileSummary{{Name: "app.js", Path: "public/app.js"}}}
	html := model.SourceRoot{Role: "Web Resources", SupplementalHTML: true}
	for i := 0; i < 12; i++ {
		path := fmt.Sprintf("a-pages/%02d", i)
		name := "page.html"
		if i == 11 {
			name = "index.htm"
		}
		html.Packages = append(html.Packages, model.Package{Path: path, FileCount: 20, SupplementalHTML: true, TopFiles: []model.FileSummary{{Name: name, Path: path + "/" + name}}})
		html.FileCount += 20
	}
	topology := &model.ProjectTopology{Modules: []model.Module{{Language: "Python", SourceRoots: []model.SourceRoot{html, {Role: "Main Source", Packages: []model.Package{source}}, {Role: "Web Resources", Packages: []model.Package{conventional}}}}}}
	before := fmt.Sprintf("%#v", topology.Modules[0].SourceRoots)
	packages := schemaATopologyPackages(topology)
	if len(packages) != 10 || packages[0].Path != "z-source" || packages[1].Path != "public" || packages[2].Path != "a-pages/11" {
		t.Fatalf("selection: %+v", packages)
	}
	f := NewFormatter()
	f.MaxTopologyPromptBytes = 0
	full := f.generateTopology(topology, nil, 0)
	if strings.Count(full, "a-pages/") != 8 {
		t.Fatalf("cap: %s", full)
	}
	f.MaxPackages = 2
	truncated := f.generateTopology(topology, nil, 0)
	if !strings.Contains(truncated, "tool.py") || !strings.Contains(truncated, "app.js") || strings.Contains(truncated, "page.html") {
		t.Fatalf("source preservation: %s", truncated)
	}
	f.MaxPackages = 0
	f.MaxTopologyPromptBytes = len(truncated) + 30
	truncated = f.generateTopology(topology, nil, 0)
	if !strings.Contains(truncated, "tool.py") || !strings.Contains(truncated, "app.js") {
		t.Fatalf("byte preservation: %s", truncated)
	}
	if after := fmt.Sprintf("%#v", topology.Modules[0].SourceRoots); !reflect.DeepEqual(before, after) {
		t.Fatal("formatter mutated topology")
	}
}

func TestSupplementalHTMLFlattensSharedPackagePaths(t *testing.T) {
	source := model.SourceRoot{Role: "Main Source"}
	html := model.SourceRoot{Role: "Web Resources", SupplementalHTML: true}
	for _, path := range []string{"", "nested"} {
		source.Packages = append(source.Packages, model.Package{Path: path, FileCount: 1, TopFiles: []model.FileSummary{{Name: "source.py", Path: path + "/source.py"}}})
		html.Packages = append(html.Packages, model.Package{Path: path, FileCount: 2, TopFiles: []model.FileSummary{{Name: "index.html", Path: path + "/index.html"}}})
	}
	topology := &model.ProjectTopology{Modules: []model.Module{{Language: "Python", SourceRoots: []model.SourceRoot{html, source}}}}
	before := fmt.Sprintf("%#v", topology.Modules)
	packages := schemaATopologyPackages(topology)
	if len(packages) != 2 {
		t.Fatalf("duplicate package paths: %+v", packages)
	}
	for _, pkg := range packages {
		if pkg.FileCount != 3 || len(pkg.TopFiles) != 2 || pkg.TopFiles[0].Name != "source.py" || pkg.TopFiles[1].Name != "index.html" {
			t.Fatalf("merged representatives: %+v", pkg)
		}
	}
	f := NewFormatter()
	f.MaxPackages = 2
	f.MaxTopologyPromptBytes = 0
	full := f.generateTopology(topology, nil, 0)
	if strings.Count(full, "Pkg:") != 2 || strings.Count(full, "index.html") != 2 {
		t.Fatalf("logical package limit: %s", full)
	}
	base := *topology
	base.Modules = []model.Module{{Language: "Python", SourceRoots: []model.SourceRoot{source}}}
	baseline := f.generateTopology(&base, nil, 0)
	f.MaxTopologyPromptBytes = len(baseline)
	output := f.generateTopology(topology, nil, 0)
	if strings.Count(output, "source.py") != 2 || strings.Contains(output, "index.html") {
		t.Fatalf("HTML displaced source under byte budget: %s", output)
	}
	if fmt.Sprintf("%#v", topology.Modules) != before {
		t.Fatal("topology mutated")
	}
}

func TestTopologyBudgetSkipsEmptyRepresentatives(t *testing.T) {
	source := model.SourceRoot{Role: "Main Source", Packages: []model.Package{
		{Path: "empty", FileCount: 5},
		{Path: "source", FileCount: 1, TopFiles: []model.FileSummary{{Name: "tool.py", Path: "source/tool.py"}}},
	}}
	topology := &model.ProjectTopology{Modules: []model.Module{{Language: "Python", SourceRoots: []model.SourceRoot{source}}}}
	f := NewFormatter()
	f.MaxTopologyPromptBytes = 0
	full := f.generateTopology(topology, nil, 0)
	// Reserve one byte to force fallback even without supplemental HTML.
	f.MaxTopologyPromptBytes = len(full) - 1
	output := f.generateTopology(topology, nil, 0)
	if strings.Contains(output, "Pkg: empty") || strings.Contains(output, "\n\n\n") {
		t.Fatalf("empty package substituted: %q", output)
	}
	withoutEmpty := *topology
	withoutEmpty.Modules = []model.Module{{Language: "Python", SourceRoots: []model.SourceRoot{{Role: "Main Source", Packages: source.Packages[1:]}}}}
	if expected := f.generateTopology(&withoutEmpty, nil, 0); output != expected {
		t.Fatalf("empty package changed truncation: got %q want %q", output, expected)
	}
	// With an enriched line, the source-only baseline fits exactly.
	f.MaxTopologyPromptBytes = len(full)
	topology.Modules[0].SourceRoots = append(topology.Modules[0].SourceRoots, model.SourceRoot{SupplementalHTML: true, Packages: []model.Package{{Path: "source", FileCount: 1, TopFiles: []model.FileSummary{{Name: "index.html", Path: "source/index.html"}}}}})
	output = f.generateTopology(topology, nil, 0)
	if !strings.Contains(output, "tool.py") || strings.Contains(output, "index.html") || strings.Contains(output, "Pkg: empty") {
		t.Fatalf("baseline mismatch: %s", output)
	}
}
