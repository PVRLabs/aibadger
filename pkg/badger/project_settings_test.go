package badger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/settings"
)

func TestAPIProjectSettingsBeforeScan(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".badger"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"go.mod":                "module example.com/test\n\ngo 1.24\n",
		".badger/settings.json": `{"limits":{"max_top_files_per_package":8}}`,
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 9; i++ {
		name := string(rune('a'+i)) + ".go"
		if err := os.WriteFile(filepath.Join(root, "internal", name), []byte("package internal\n"+strings.Repeat("// comment\n", i)), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.SettingsPath = filepath.Join(t.TempDir(), "missing")
	var output, warningOutput bytes.Buffer
	if err := RunAPI(cfg, APIOptions{Operation: "topology", Stdout: &output, Stderr: &warningOutput}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "b.go") || strings.Contains(output.String(), "a.go") || warningOutput.Len() != 0 {
		t.Fatalf("effective API topology: %s warnings %s", output.String(), warningOutput.String())
	}
	effective, warnings := settings.LoadProject(cfg.SettingsPath, cfg.Root)
	if len(warnings) != 0 {
		t.Fatal(warnings)
	}
	cfg = applyProjectLimits(cfg.withDefaults(), effective.Limits)
	eng, err := scanProjectWithLimit(&bytes.Buffer{}, cfg.Root, scanOutputSilent, cfg.MaxFilesPerDirectory, cfg.MaxTopFilesPerPackage, effective.DirectoryPolicy())
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range eng.Topology.Modules {
		for _, sr := range m.SourceRoots {
			for _, p := range sr.Packages {
				if p.Path == "internal" {
					if len(p.TopFiles) != 8 {
						t.Fatalf("got %d files", len(p.TopFiles))
					}
					return
				}
			}
		}
	}
	t.Fatal("src package missing")
}

func TestProjectSettingsPreserveExplicitAPIConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxTopFilesPerPackage = 4
	cfg.MaxContextFileBytes = -1
	got := applyProjectLimits(cfg, &settings.SettingsLimits{MaxTopFilesPerPackage: 8, MaxContextFileBytes: 131072})
	if got.MaxTopFilesPerPackage != 4 || got.MaxContextFileBytes != -1 {
		t.Fatalf("explicit config overwritten: %+v", got)
	}
}
