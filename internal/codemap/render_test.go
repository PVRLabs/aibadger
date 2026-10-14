package codemap

import (
	"encoding/json"
	"strings"
	"testing"
)

func reportFixture() *directory {
	return &directory{Name: "project", Path: ".", Lines: 12, Files: 4, DirectLines: 2, DirectFiles: 1,
		Languages:    map[string]languageStats{"Go": {Lines: 12, Files: 4}},
		DirectByLang: map[string]languageStats{"Go": {Lines: 2, Files: 1}},
		FileList:     []sourceFile{{Name: "main.go", Lines: 2}},
		Children: []*directory{{Name: "src", Path: "src", Lines: 10, Files: 3, DirectLines: 10, DirectFiles: 2,
			Languages:    map[string]languageStats{"Go": {Lines: 10, Files: 3}},
			DirectByLang: map[string]languageStats{"Go": {Lines: 10, Files: 2}}}}}
}

func TestRenderMetadataAndEscaping(t *testing.T) {
	root := reportFixture()
	dangerous := "</script><script>alert('x')</script>&\"\u2028\u2029日本語"
	root.Name = dangerous
	root.Children[0].Path = dangerous
	root.FileList[0].Name = dangerous
	root.DirectByLang["<unknown>"] = languageStats{Files: 1}
	page, err := renderHTML(root)
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(page), `<script id="map-data" type="application/json">`) + len(`<script id="map-data" type="application/json">`)
	end := strings.Index(string(page[start:]), "</script>") + start
	data := page[start:end]
	if strings.Contains(string(data), "<") || strings.Contains(string(data), "&") {
		t.Fatalf("unsafe inline JSON: %s", data)
	}
	var decoded directory
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Name != dangerous || decoded.FileList[0].Name != dangerous || decoded.Children[0].Path != dangerous {
		t.Fatal("metadata changed during escaping")
	}
	if decoded.Children[0].DirectByLang["Go"].Files != 2 || decoded.DirectByLang["Go"].Files != 1 {
		t.Fatal("lost explicit direct-language accounting")
	}
	if !strings.Contains(string(data), `"fileList":[]`) || !strings.Contains(string(data), `"children":[]`) {
		t.Fatal("empty lists must be arrays for page consumers")
	}
	second, err := renderHTML(root)
	if err != nil || string(page) != string(second) {
		t.Fatal("rendering must be deterministic")
	}
}

func TestRenderCompleteHTMLBudget(t *testing.T) {
	root := reportFixture()
	base, err := renderHTML(root)
	if err != nil {
		t.Fatal(err)
	}
	root.Name += strings.Repeat("x", maxHTMLBytes-len(base))
	page, err := renderHTML(root)
	if err != nil || len(page) != maxHTMLBytes {
		t.Fatalf("exact production boundary: length=%d err=%v", len(page), err)
	}
	root.Name += "x"
	page, err = renderHTML(root)
	if page != nil || err == nil || !strings.Contains(err.Error(), "HTML size limit") {
		t.Fatalf("over-budget output: length=%d err=%v", len(page), err)
	}
	// Escaping, not raw input length, determines the page budget.
	root = reportFixture()
	root.Name = strings.Repeat("<", 2048)
	page, err = renderHTMLTemplate(root, dataMarker, 4096)
	if page != nil || err == nil {
		t.Fatal("escaped data must consume the budget")
	}
	for _, limit := range []int{-1, 0, 1, len(base) - 1} {
		if page, err := renderHTMLTemplate(reportFixture(), pageTemplate, limit); page != nil || err == nil {
			t.Fatalf("limit %d did not reject complete page", limit)
		}
	}
}

func TestRenderTemplateAndDepthErrors(t *testing.T) {
	if _, err := renderHTMLTemplate(reportFixture(), "no marker", maxHTMLBytes); err == nil || !strings.Contains(err.Error(), "data marker") {
		t.Fatalf("missing marker: %v", err)
	}
	if _, err := renderHTML(nil); err == nil {
		t.Fatal("nil root accepted")
	}
	root := reportFixture()
	node := root
	root.Children = nil
	for depth := 0; depth < maxDirectoryDepth; depth++ {
		child := &directory{Name: "nested", Path: "nested"}
		node.Children = []*directory{child}
		node = child
	}
	if _, err := renderHTML(root); err != nil {
		t.Fatalf("exact depth limit rejected: %v", err)
	}
	node.Children = []*directory{{Name: "too deep"}}
	if page, err := renderHTML(root); page != nil || err == nil || !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("over-depth: %v", err)
	}
	node.Children = []*directory{nil}
	if _, err := renderHTML(root); err == nil {
		t.Fatal("nil child accepted")
	}
}

func TestRenderStringChunks(t *testing.T) {
	for _, text := range []string{strings.Repeat("x", 1023) + "日本語\u2028<", strings.Repeat("é", 1030), "invalid\xff\xfe", strings.Repeat("\x80", 2050)} {
		w := &htmlWriter{limit: 20000}
		w.value(text)
		want, _ := json.Marshal(text)
		if w.err != nil || w.String() != string(want) {
			t.Fatalf("chunk escaping differs from JSON encoder: %v", w.err)
		}
	}
}

func TestPageStandaloneAssets(t *testing.T) {
	for _, forbidden := range []string{"src=", "@import", "url(", "fetch(", "XMLHttpRequest", "https://", "http://", "__BADGER_MAP_DATA____BADGER_MAP_DATA__"} {
		if strings.Contains(pageTemplate, forbidden) {
			t.Errorf("unexpected runtime/network dependency %q", forbidden)
		}
	}
	if strings.Count(pageTemplate, dataMarker) != 1 {
		t.Fatal("expected one metadata marker")
	}
	for _, fragment := range []string{`value="2" selected`, "prefers-reduced-motion", "No subfolders with nonblank source lines", "Generated locally · Nothing uploaded"} {
		if !strings.Contains(pageTemplate, fragment) {
			t.Errorf("missing page contract %q", fragment)
		}
	}
}
