package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/PVRLabs/aibadger/internal/dirpolicy"
)

func TestProjectSettingsUseSelectedRootAndKeepUserState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := SaveSettings(path, Settings{FirstRunOnboardingCompleted: true, Limits: &SettingsLimits{MaxTopFilesPerPackage: 5, MaxTopologyPromptBytes: 1048576}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".badger"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".badger", "settings.json"), []byte(`{"limits":{"max_top_files_per_package":8},"exclude_generated_directories":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.SettingsPath = path
	m := NewModel(root, cfg)
	if m.cfg.MaxTopFilesPerPackage != 8 || m.cfg.MaxTopologyPromptBytes != 1048576 || m.cfg.DirectoryPolicy.Skip("build") {
		t.Fatalf("effective config %+v", m.cfg)
	}
	if m.settings.Limits.MaxTopFilesPerPackage != 5 {
		t.Fatal("project override leaked into saved user state")
	}
	cfg.MaxTopFilesPerPackage = 4
	if got := NewModel(root, cfg).cfg.MaxTopFilesPerPackage; got != 4 {
		t.Fatalf("explicit configuration overwritten: %d", got)
	}
}

func TestProjectReviewBudgetAndExplicitOverride(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".badger"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".badger", "settings.json"), []byte(`{"limits":{"max_topology_prompt_bytes":1048576}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name           string
		supplied, want int
	}{
		{"project", 0, 1048576},
		{"explicit", 786432, 786432},
		{"disabled", -1, -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.MaxTopologyPromptBytes = tc.supplied
			m := NewModel(root, cfg)
			if m.reviewMaxPromptBytes != tc.want || m.cfg.MaxTopologyPromptBytes != tc.want {
				t.Fatalf("review budget %d, topology budget %d; want %d", m.reviewMaxPromptBytes, m.cfg.MaxTopologyPromptBytes, tc.want)
			}
		})
	}
}

func TestDisabledSettingsPreserveDirectoryPolicy(t *testing.T) {
	for _, project := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing project", true: "limits only"}[project], func(t *testing.T) {
			root := t.TempDir()
			if project {
				if err := os.Mkdir(filepath.Join(root, ".badger"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, ".badger", "settings.json"), []byte(`{"limits":{"max_top_files_per_package":8}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			enabled := false
			cfg := DefaultConfig()
			cfg.SettingsPath = ""
			cfg.DirectoryPolicy = dirpolicy.New(&enabled, []string{"custom-output"})
			m := NewModel(root, cfg)
			if !m.cfg.DirectoryPolicy.Skip("custom-output") || m.cfg.DirectoryPolicy.Skip("build") {
				t.Fatal("caller directory policy was replaced")
			}
		})
	}
}
