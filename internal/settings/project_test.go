package settings

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectLayers(t *testing.T) {
	root := t.TempDir()
	user := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(user, []byte(`{"exclude_generated_directories":true,"additional_excluded_directories":["cache"],"limits":{"max_top_files_per_package":5,"max_topology_prompt_bytes":1048576}}`), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, ".badger")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, "settings.json")
	for _, tc := range []struct {
		name, json           string
		top, bytes, warnings int
	}{
		{"missing keys", `{"exclude_generated_directories":false,"additional_excluded_directories":[],"limits":{"max_top_files_per_package":8}}`, 8, 1048576, 0},
		{"zero inherits", `{"limits":{"max_top_files_per_package":0}}`, 5, 1048576, 0},
		{"invalid retains global", `{"limits":{"max_top_files_per_package":51,"max_topology_prompt_bytes":0}}`, 5, 1048576, 1},
		{"malformed limits", `{"limits":"bad"}`, 5, 1048576, 1},
		{"malformed file", `{`, 5, 1048576, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(project, []byte(tc.json), 0600); err != nil {
				t.Fatal(err)
			}
			got, warnings := LoadProject(user, root)
			if got.Limits.MaxTopFilesPerPackage != tc.top || got.Limits.MaxTopologyPromptBytes != tc.bytes || len(warnings) != tc.warnings {
				t.Fatalf("got %+v warnings %v", got.Limits, warnings)
			}
			if tc.name == "missing keys" && (got.ExcludeGeneratedDirectories == nil || *got.ExcludeGeneratedDirectories || got.AdditionalExcludedDirectories == nil || len(*got.AdditionalExcludedDirectories) != 0) {
				t.Fatalf("lost explicit directory overrides: %+v", got)
			}
			data, _ := os.ReadFile(project)
			if string(data) != tc.json {
				t.Fatal("rewrote project file")
			}
		})
	}
}

func TestProjectExclusionOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, entries string
		want          []string
		warnings      int
	}{
		{"invalid only inherits", `["invalid/path"]`, []string{"user-cache"}, 1},
		{"explicit empty clears", `[]`, []string{}, 0},
		{"mixed applies valid entries", `["invalid/path","project-cache"]`, []string{"project-cache"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			user := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(user, []byte(`{"additional_excluded_directories":["user-cache"]}`), 0600); err != nil {
				t.Fatal(err)
			}
			project := filepath.Join(root, ".badger", "settings.json")
			if err := os.MkdirAll(filepath.Dir(project), 0700); err != nil {
				t.Fatal(err)
			}
			body := `{"additional_excluded_directories":` + tc.entries + `}`
			if err := os.WriteFile(project, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			got, warnings := LoadProject(user, root)
			if got.AdditionalExcludedDirectories == nil || !reflect.DeepEqual(*got.AdditionalExcludedDirectories, tc.want) {
				t.Fatalf("effective exclusions %+v; want %v", got.AdditionalExcludedDirectories, tc.want)
			}
			if len(warnings) != tc.warnings {
				t.Fatalf("warnings %v; want %d", warnings, tc.warnings)
			}
			for _, name := range []string{"user-cache", "project-cache"} {
				want := false
				for _, excluded := range tc.want {
					if name == excluded {
						want = true
					}
				}
				if got.DirectoryPolicy().Skip(name) != want {
					t.Fatalf("wrong effective policy for %s", name)
				}
			}
			data, err := os.ReadFile(project)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != body {
				t.Fatal("rewrote project settings")
			}
		})
	}
}
