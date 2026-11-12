package badger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/reviewtask"
)

func TestAPIReceivesGlobalDirectorySettings(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"tool.py", "build/visible.html", "custom-output/hidden.html", "nested/CUSTOM-OUTPUT/hidden.html", ".git/hidden.html"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"exclude_generated_directories":false,"additional_excluded_directories":["custom-output"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output, warnings bytes.Buffer
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.SettingsPath = path
	if err := RunAPI(cfg, APIOptions{Operation: "topology", Stdout: &output, Stderr: &warnings}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "visible.html") || strings.Contains(output.String(), "hidden.html") || warnings.Len() != 0 {
		t.Fatalf("API policy: %s warnings=%s", output.String(), warnings.String())
	}
	// Deliberate extraction remains available even when discovery excludes it.
	input := filepath.Join(t.TempDir(), "input")
	goal := filepath.Join(t.TempDir(), "goal")
	if err := os.WriteFile(input, []byte("FILE:custom-output/hidden.html"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goal, []byte("read the selected file"), 0600); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := RunAPI(cfg, APIOptions{Operation: "extract", InputPath: input, GoalFilePath: goal, Stdout: &output}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "custom-output/hidden.html") {
		t.Fatal("explicit request excluded by discovery settings")
	}
	// Review topology gets the same per-invocation policy.
	reviewInput := filepath.Join(t.TempDir(), "review-input")
	if err := os.WriteFile(reviewInput, []byte("focus"), 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	_ = runAPIWithReviewBuilder(cfg, APIOptions{Operation: "review-context", InputPath: reviewInput, IncludeReviewTopology: true}, func(_ string, opts reviewtask.Options) (reviewtask.InitialReviewResult, error) {
		called = true
		if opts.DirectoryPolicy == nil || opts.DirectoryPolicy.Skip("build") || !opts.DirectoryPolicy.Skip("custom-output") {
			t.Fatal("review policy missing")
		}
		return reviewtask.InitialReviewResult{}, nil
	})
	if !called {
		t.Fatal("review builder not called")
	}
}
