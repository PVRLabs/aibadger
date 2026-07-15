package model

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestFileKindConstants(t *testing.T) {
	if FileKindSource != "source" {
		t.Errorf("FileKindSource = %q, want %q", FileKindSource, "source")
	}
	if FileKindAsset != "asset" {
		t.Errorf("FileKindAsset = %q, want %q", FileKindAsset, "asset")
	}
	if FileKindBinary != "binary" {
		t.Errorf("FileKindBinary = %q, want %q", FileKindBinary, "binary")
	}
}

func TestProjectTopologyJSONRoundTrip(t *testing.T) {
	in := ProjectTopology{
		Name:            "myapp",
		Languages:       []string{"Go", "JavaScript"},
		PrimaryLanguage: "Go",
		Stack:           []string{"Go Modules"},
		Structure:       "Single Module",
		ScanTime:        342 * time.Millisecond,
		ProjectRoot:     "/home/user/myapp",
		Modules: []Module{
			{
				Name:       "core",
				Path:       ".",
				FileCount:  42,
				TotalBytes: 123456,
				Heaviest: HeaviestFile{
					Name: "main.go",
					Path: "main.go",
					Size: 8192,
					Kind: "source",
				},
				TopFiles: []FileSummary{
					{Name: "main.go", Path: "main.go", Size: 8192, Kind: "source"},
					{Name: "util.go", Path: "pkg/util.go", Size: 4096, Kind: "source"},
				},
				AuxFiles: []FileSummary{
					{Name: "config.json", Path: "config.json", Size: 512, Kind: "asset"},
				},
				SourceRoots: []SourceRoot{
					{
						Path:      ".",
						Role:      "Main Source",
						FileCount: 42,
						Packages: []Package{
							{
								Name:      "main",
								Path:      ".",
								FileCount: 2,
								Heaviest:  HeaviestFile{Name: "main.go", Path: "main.go", Size: 8192},
								TopFiles: []FileSummary{
									{Name: "main.go", Path: "main.go", Size: 8192},
								},
							},
						},
					},
				},
				Language: "Go",
			},
		},
		ExternalContext: []ExternalContext{
			{
				Path: "/home/user/external-lib",
				// AbsPath intentionally zero to avoid locking privacy-sensitive
				// serialization — removing it later is a fix, not a regression.
				Top: []ExternalContextItem{
					{Name: "README.md"},
					{Name: "src", IsDir: true},
				},
			},
		},
	}

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}

	var out ProjectTopology
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip mismatch:\n  in:  %#v\n  out: %#v", in, out)
	}
}

func TestJSONOmitEmptyFields(t *testing.T) {
	// abs_path is not tested here — removing it later is a privacy fix,
	// not a contract regression.

	tests := []struct {
		name string
		key  string
		val  any
	}{
		{
			name: "external_context zero",
			key:  "external_context",
			val:  ProjectTopology{Name: "app", ProjectRoot: "/root"},
		},
		{
			name: "top zero",
			key:  "top",
			val:  ExternalContext{Path: "/p", AbsPath: "/p"},
		},
		{
			name: "aux_files zero",
			key:  "aux_files",
			val:  Module{Name: "m", Path: ".", Heaviest: HeaviestFile{Name: "f", Path: "f"}, TopFiles: []FileSummary{{Name: "f", Path: "f"}}, SourceRoots: []SourceRoot{{}}},
		},
		{
			name: "kind zero",
			key:  "kind",
			val:  HeaviestFile{Name: "f", Path: "f", Size: 100},
		},
		{
			name: "is_dir false",
			key:  "is_dir",
			val:  ExternalContextItem{Name: "readme.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := marshalMap(t, tt.val)
			if _, ok := m[tt.key]; ok {
				t.Errorf("key %q should be absent, but got present in map %v", tt.key, m)
			}
		})
	}

	// positive cases: kind and is_dir should appear when non-empty
	t.Run("kind present", func(t *testing.T) {
		m := marshalMap(t, HeaviestFile{Name: "f", Path: "f", Size: 100, Kind: "source"})
		if v, ok := m["kind"]; !ok {
			t.Errorf("key %q should be present", "kind")
		} else if v != "source" {
			t.Errorf("kind = %v, want %q", v, "source")
		}
	})

	t.Run("is_dir present", func(t *testing.T) {
		m := marshalMap(t, ExternalContextItem{Name: "src", IsDir: true})
		if v, ok := m["is_dir"]; !ok {
			t.Errorf("key %q should be present", "is_dir")
		} else if v != true {
			t.Errorf("is_dir = %v, want true", v)
		}
	})
}

func TestProjectTopologyJSONFieldNames(t *testing.T) {
	// NOTE: abs_path is intentionally not asserted here — it serializes
	// unconditionally today (no omitempty) but that is a privacy concern
	// (leaks local filesystem paths).  Removing it later should be treated
	// as a privacy fix, not a contract regression.

	top := ProjectTopology{
		Name:            "app",
		PrimaryLanguage: "Go",
		ExternalContext: []ExternalContext{
			{Path: "/ext", AbsPath: "/ext"},
		},
		ScanTime:    100 * time.Millisecond,
		ProjectRoot: "/app",
		Modules: []Module{
			{
				Name:       "m",
				Path:       ".",
				FileCount:  5,
				TotalBytes: 1000,
				Heaviest:   HeaviestFile{Name: "f", Path: "f", Size: 500},
				TopFiles:   []FileSummary{{Name: "f", Path: "f", Size: 500}},
				AuxFiles:   []FileSummary{{Name: "a", Path: "a", Size: 100}},
				SourceRoots: []SourceRoot{
					{Path: ".", Role: "Main Source", FileCount: 5, Packages: []Package{{Name: "main", Path: ".", FileCount: 5, Heaviest: HeaviestFile{Name: "f", Path: "f", Size: 500}, TopFiles: []FileSummary{{Name: "f", Path: "f", Size: 500}}}}},
				},
				Language: "Go",
			},
		},
	}

	m := marshalMap(t, top)

	// top-level keys
	topKeys := []string{"primary_language", "external_context", "scan_time", "project_root"}
	for _, k := range topKeys {
		if _, ok := m[k]; !ok {
			t.Errorf("top-level key %q is absent from JSON output", k)
		}
	}

	// keys nested inside modules[0]
	modules, _ := m["modules"].([]any)
	if len(modules) == 0 {
		t.Fatal("expected at least one module")
	}
	mod0, _ := modules[0].(map[string]any)
	modKeys := []string{"file_count", "total_bytes", "source_roots", "top_files", "aux_files"}
	for _, k := range modKeys {
		if _, ok := mod0[k]; !ok {
			t.Errorf("key %q is absent from modules[0]", k)
		}
	}

	// external_context is present (privacy-sensitive abs_path is not checked)
}

func marshalMap(t *testing.T, value any) map[string]any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}
