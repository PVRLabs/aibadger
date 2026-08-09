package badger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/writer"
)

func TestScanProjectStableSuppressesTimingOutput(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/quiet\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal"), 0755); err != nil {
		t.Fatalf("MkdirAll(internal) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "app.go"), []byte("package internal\n"), 0644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}

	var output bytes.Buffer
	eng, err := scanProject(&output, tmpDir, scanOutputStable, 0)
	if err != nil {
		t.Fatalf("scanProject() error = %v", err)
	}
	if eng == nil || eng.Topology == nil {
		t.Fatal("scanProject() returned nil engine or topology")
	}

	for _, want := range []string{
		"Scanning project... Done",
		"(Go)",
		"Found 1 modules",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("stable scanProject() output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "Done in") {
		t.Fatalf("stable scanProject() output included timing:\n%s", output.String())
	}
}

func TestScanProjectSilentSuppressesWrapperOutput(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/silent\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "internal"), 0755); err != nil {
		t.Fatalf("MkdirAll(internal) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "internal", "app.go"), []byte("package internal\n"), 0644); err != nil {
		t.Fatalf("WriteFile(app.go) error = %v", err)
	}

	var output bytes.Buffer
	eng, err := scanProject(&output, tmpDir, scanOutputSilent, 0)
	if err != nil {
		t.Fatalf("scanProject() error = %v", err)
	}
	if eng == nil || eng.Topology == nil {
		t.Fatal("scanProject() returned nil engine or topology")
	}
	if output.String() != "" {
		t.Fatalf("silent scanProject() output = %q, want empty", output.String())
	}
}

func TestPrintAPIResponsePlanWithUpdates(t *testing.T) {
	result := writer.ParseResult{
		Updates: []writer.FileUpdate{
			{Path: "internal/foo.go", Content: "package internal\n", Kind: writer.UpdateKindWrite},
			{Path: "internal/old.go", Kind: writer.UpdateKindDelete},
		},
		Text: "Notes",
	}

	var output bytes.Buffer
	printAPIResponsePlan(&output, "ignored raw response", result)

	for _, want := range []string{
		"[WRITE PLAN]",
		"updates=2",
		"plaintext_response_bytes=5",
		"WRITE path=internal/foo.go bytes=17",
		"DELETE path=internal/old.go bytes=0",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printAPIResponsePlan() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPrintAPIExtractionPlan(t *testing.T) {
	commands := []extractor.Command{
		{Type: "FILE", Path: "cmd/badger/main.go"},
		{Type: "NEAR", Path: "internal/tui/tui.go", Pattern: "stateHome"},
		{Type: "PREFIX", Path: "pkg/badger/api.go", Pattern: "func RunAPI"},
	}

	var output bytes.Buffer
	printAPIExtractionPlan(&output, "FILE:cmd/badger/main.go\nNEAR:internal/tui/tui.go#stateHome\n", commands)

	for _, want := range []string{
		"[EXTRACTION PLAN]",
		"commands=3",
		"FILE path=cmd/badger/main.go",
		"NEAR path=internal/tui/tui.go#stateHome",
		"PREFIX path=pkg/badger/api.go#func RunAPI",
		"plaintext_input_bytes=58",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printAPIExtractionPlan() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPrintAPIResponsePlanWithTextResponse(t *testing.T) {
	response := "No code changes needed.\n"

	var output bytes.Buffer
	printAPIResponsePlan(&output, response, writer.ParseResult{})

	for _, want := range []string{
		"[TEXT RESPONSE]",
		"updates=0",
		"plaintext_response_bytes=23",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printAPIResponsePlan() output missing %q:\n%s", want, output.String())
		}
	}
}
