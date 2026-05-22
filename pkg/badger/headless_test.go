package badger

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/model"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

func TestConfirm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "yes short", input: "y\n", expected: true},
		{name: "yes long", input: "yes\n", expected: true},
		{name: "no", input: "n\n", expected: false},
		{name: "blank", input: "\n", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bufio.NewReader(strings.NewReader(tt.input))
			if got := confirm(reader); got != tt.expected {
				t.Fatalf("confirm() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReadFromStopsAtDone(t *testing.T) {
	input := "first line\nsecond line\nDONE\nignored\n"
	got := readFrom(strings.NewReader(input))
	want := "first line\nsecond line"

	if got != want {
		t.Fatalf("readFrom() = %q, want %q", got, want)
	}
}

func TestHandleExtractionCommandsDevStepReadsInputFile(t *testing.T) {
	inputFile := writeTempInput(t, "FILE:cmd/badger/main.go\nPREFIX:cmd/badger/main_test.go#func TestConfirm\n")
	opts := HeadlessOptions{
		Step:      "extraction",
		InputPath: inputFile,
	}
	eng := engine.FromTopology("", nil)
	session := workflow.NewSession(eng, writer.DefaultWhitespaceMode)

	var output bytes.Buffer
	var commands []extractor.Command
	var goal string
	var stop bool
	commands, goal, stop = handleExtractionCommands(&output, "keep this goal", session, opts)

	if !stop {
		t.Fatal("handleExtractionCommands() did not stop after dev extraction step")
	}
	if commands != nil {
		t.Fatalf("handleExtractionCommands() commands = %+v, want nil for dev extraction step", commands)
	}
	if goal != "keep this goal" {
		t.Fatalf("handleExtractionCommands() goal = %q, want %q", goal, "keep this goal")
	}
	for _, want := range []string{
		"[✓] Parsed 2 commands.",
		"cmd: FILE cmd/badger/main.go",
		"cmd: PREFIX cmd/badger/main_test.go",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("handleExtractionCommands() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestHandleContextCopyUsesRespondLabelForDesignFocus(t *testing.T) {
	var output bytes.Buffer
	reader := bufio.NewReader(strings.NewReader("n\n"))

	if handleContextCopy(&output, reader, "schema b", HeadlessOptions{Focus: protocol.FocusDesign}) {
		t.Fatal("handleContextCopy() returned true, want false")
	}
	for _, want := range []string{
		"Ready to copy Prompt 2: Respond to clipboard.",
		"Copy? (y/N):",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("handleContextCopy() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestHandleFinalResponseUsesRespondLabelForDesignFocus(t *testing.T) {
	eng := engine.FromTopology("", nil)
	session := workflow.NewSession(eng, writer.DefaultWhitespaceMode)
	var output bytes.Buffer

	if handleFinalResponse(&output, bufio.NewReader(strings.NewReader("n\n")), session, HeadlessOptions{
		Focus: protocol.FocusDesign,
		Stdin: strings.NewReader("A textual response only.\nDONE\n"),
	}) {
		t.Fatal("handleFinalResponse() returned true, want false")
	}
	if !strings.Contains(output.String(), "1. Paste Prompt 2: Respond into your AI chat.") {
		t.Fatalf("handleFinalResponse() output missing focus-aware next step:\n%s", output.String())
	}
}

func TestRunHeadlessContextStepIncludesExplicitGoal(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/context\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}

	inputFile := writeTempInput(t, "FILE:main.go\n")
	cfg := DefaultConfig()
	cfg.Root = tmpDir

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:      "context",
		InputPath: inputFile,
		Goal:      "add structured logging to main",
		Stdout:    &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	for _, want := range []string{
		"[CONTEXT]",
		"--- File: main.go (Full File) ---",
		"[TASK]\nadd structured logging to main",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("RunHeadless() context output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunHeadlessContextStepWarnsAndContinuesWithUsablePartialExtraction(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/context\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=1\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}

	inputFile := writeTempInput(t, strings.Join([]string{
		"FILE:main.go",
		"FILE:missing.go",
		"FILE:.env",
	}, "\n"))
	cfg := DefaultConfig()
	cfg.Root = tmpDir

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:      "context",
		InputPath: inputFile,
		Goal:      "review partial extraction",
		Stdout:    &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	for _, want := range []string{
		"[!] Extracted 1 file with warnings.",
		"Failed requests:",
		"missing.go: file not found: missing.go",
		"Excluded by Prompt 2 safety rules:",
		".env: excluded from Prompt 2",
		"[CONTEXT]",
		"--- File: main.go (Full File) ---",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("RunHeadless() partial extraction output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "SECRET=1") {
		t.Fatalf("RunHeadless() leaked excluded file content:\n%s", output.String())
	}
}

func TestRunHeadlessContextStepRejectsOnlyExcludedFiles(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/context\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, ".env"), []byte("SECRET=1\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "keys"), 0755); err != nil {
		t.Fatalf("MkdirAll(keys) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "keys", "id_rsa"), []byte("-----BEGIN OPENSSH PRIVATE KEY-----\n"), 0644); err != nil {
		t.Fatalf("WriteFile(id_rsa) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "binary"), []byte{0x00, 0x01, 0x02}, 0644); err != nil {
		t.Fatalf("WriteFile(binary) error = %v", err)
	}

	inputFile := writeTempInput(t, strings.Join([]string{
		"FILE:.env",
		"FILE:keys/id_rsa",
		"FILE:binary",
	}, "\n"))
	cfg := DefaultConfig()
	cfg.Root = tmpDir

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:      "context",
		InputPath: inputFile,
		Goal:      "review the project",
		Stdout:    &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	if !strings.Contains(output.String(), "Extraction error: no safe files available for Prompt 2 after excluding binary and sensitive files") {
		t.Fatalf("RunHeadless() did not surface the safe-file exclusion error:\n%s", output.String())
	}
	if strings.Contains(output.String(), "[CONTEXT]") {
		t.Fatalf("RunHeadless() printed Prompt 2 output despite excluding every file:\n%s", output.String())
	}
}

func TestRunHeadlessTopologyStepDoesNotPromptForGoal(t *testing.T) {
	inputFile := writeTempInput(t, "add logging to handler")
	eng := engine.FromTopology("", &model.ProjectTopology{
		Languages:       []string{"Go"},
		PrimaryLanguage: "Go",
	})
	session := workflow.NewSession(eng, writer.DefaultWhitespaceMode)
	opts := HeadlessOptions{
		Step:      "topology",
		InputPath: inputFile,
	}

	var output bytes.Buffer
	runHeadlessStep(&output, session, opts)

	if strings.Contains(output.String(), "Step 1:") {
		t.Fatalf("runHeadlessStep() prompted for goal:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "[PROJECT TOPOLOGY]") {
		t.Fatalf("runHeadlessStep() did not print topology:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "add logging to handler") {
		t.Fatalf("runHeadlessStep() did not include topology query:\n%s", output.String())
	}
}

func TestRunHeadlessLargeProjectDoesNotShowTruncationDialog(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/large\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "pkg"), 0755); err != nil {
		t.Fatalf("MkdirAll(pkg) error = %v", err)
	}
	for _, name := range []string{"one.go", "two.go"} {
		if err := os.WriteFile(filepath.Join(tmpDir, "pkg", name), []byte("package pkg\n"), 0644); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}

	cfg := DefaultConfig()
	cfg.Root = tmpDir
	cfg.LargeProjectFileThreshold = 1

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Stdin:  strings.NewReader("/exit\n"),
		Stdout: &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	for _, unwanted := range []string{
		"WARNING: Project is large",
		"Choice (c/t/e):",
		"Truncate Prompt 1: Topology",
		"Truncation enabled.",
		"Continuing as is.",
	} {
		if strings.Contains(output.String(), unwanted) {
			t.Fatalf("RunHeadless() output included truncation dialog text %q:\n%s", unwanted, output.String())
		}
	}
	if !strings.Contains(output.String(), "Step 1: What is your coding goal?") {
		t.Fatalf("RunHeadless() did not continue to the normal headless session:\n%s", output.String())
	}
}

func TestRunHeadlessTopologyStepCanTruncateTopology(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/truncate\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	for _, dir := range []string{"pkg/one", "pkg/two"} {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatalf("MkdirAll(%s) error = %v", dir, err)
		}
		name := filepath.Base(dir)
		if err := os.WriteFile(filepath.Join(tmpDir, dir, name+".go"), []byte("package "+name+"\n"), 0644); err != nil {
			t.Fatalf("WriteFile(%s.go) error = %v", name, err)
		}
	}
	inputFile := writeTempInput(t, "inspect packages")

	cfg := DefaultConfig()
	cfg.Root = tmpDir
	cfg.TruncatedMaxPackages = 1

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:             "topology",
		InputPath:        inputFile,
		TruncateTopology: true,
		Stdout:           &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	if !strings.Contains(output.String(), "... [Truncated due to size limit] ...") {
		t.Fatalf("RunHeadless() topology output missing truncation marker:\n%s", output.String())
	}
	if strings.Contains(output.String(), "Choice (c/t/e):") {
		t.Fatalf("RunHeadless() topology output included interactive prompt:\n%s", output.String())
	}
}

func TestScanProjectQuietSuppressesTimingOutput(t *testing.T) {
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
	eng, err := scanProject(&output, tmpDir, scanOutputStable)
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
			t.Fatalf("quiet scanProject() output missing %q:\n%s", want, output.String())
		}
	}
	if strings.Contains(output.String(), "Done in") {
		t.Fatalf("quiet scanProject() output included timing:\n%s", output.String())
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
	eng, err := scanProject(&output, tmpDir, scanOutputSilent)
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

func TestRunHeadlessDoesNotCreateSettings(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/headless\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}

	settingsPath := filepath.Join(t.TempDir(), ".badger", "settings.json")
	cfg := DefaultConfig()
	cfg.Root = tmpDir
	cfg.SettingsPath = settingsPath

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:   "scan",
		Stdout: &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v", err)
	}

	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("settings file exists after headless run, stat error = %v", err)
	}
}

func TestHeadlessStepAliases(t *testing.T) {
	topologyAliases := []string{"topology", "map", "prompt1"}
	for _, step := range topologyAliases {
		if !isTopologyStep(step) {
			t.Fatalf("isTopologyStep(%q) = false, want true", step)
		}
	}

	codeContextAliases := []string{"context", "prompt2"}
	for _, step := range codeContextAliases {
		if !isCodeContextStep(step) {
			t.Fatalf("isCodeContextStep(%q) = false, want true", step)
		}
	}

	writePlanAliases := []string{"write-plan", "write"}
	for _, step := range writePlanAliases {
		if !isWritePlanStep(step) {
			t.Fatalf("isWritePlanStep(%q) = false, want true", step)
		}
	}
}

func TestHeadlessStepAliasesRejectOtherSteps(t *testing.T) {
	otherSteps := []string{"", "scan", "goal", "extraction"}
	for _, step := range otherSteps {
		if isTopologyStep(step) {
			t.Fatalf("isTopologyStep(%q) = true, want false", step)
		}
		if isCodeContextStep(step) {
			t.Fatalf("isCodeContextStep(%q) = true, want false", step)
		}
		if isWritePlanStep(step) {
			t.Fatalf("isWritePlanStep(%q) = true, want false", step)
		}
	}
}

func TestPrintHeadlessResponsePlanWithUpdates(t *testing.T) {
	result := writer.ParseResult{
		Updates: []writer.FileUpdate{
			{Path: "internal/foo.go", Content: "package internal\n", Kind: writer.UpdateKindWrite},
			{Path: "internal/old.go", Kind: writer.UpdateKindDelete},
		},
		Text: "Notes",
	}

	var output bytes.Buffer
	printHeadlessResponsePlan(&output, "ignored raw response", result)

	for _, want := range []string{
		"[WRITE PLAN]",
		"updates=2",
		"plaintext_response_bytes=5",
		"WRITE path=internal/foo.go bytes=17",
		"DELETE path=internal/old.go bytes=0",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printHeadlessResponsePlan() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPrintHeadlessResponsePlanWithTextResponse(t *testing.T) {
	response := "No code changes needed.\n"

	var output bytes.Buffer
	printHeadlessResponsePlan(&output, response, writer.ParseResult{})

	for _, want := range []string{
		"[TEXT RESPONSE]",
		"updates=0",
		"plaintext_response_bytes=23",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printHeadlessResponsePlan() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestPrintMixedResponseNotes(t *testing.T) {
	var output bytes.Buffer

	printMixedResponseNotes(&output, "Deepseek notes\n@@")

	for _, want := range []string{
		"[i] AI included notes alongside file updates.",
		"Deepseek notes",
		"@@",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("printMixedResponseNotes() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunHeadlessExitsGracefullyWhenDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".badger-disable"), []byte("opt-out\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.badger-disable) error = %v", err)
	}

	cfg := DefaultConfig()
	cfg.Root = tmpDir

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:   "scan",
		Stdout: &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v, want nil", err)
	}

	for _, want := range []string{
		"⚠️  Project explicitly opted out.",
		"A .badger-disable file exists in this project's root.",
		"Badger will not scan, summarize, copy, or generate prompts.",
		"To proceed, remove .badger-disable from the project root.",
	} {
		if !strings.Contains(output.String(), want) {
			t.Fatalf("RunHeadless() output missing %q:\n%s", want, output.String())
		}
	}
}

func TestRunHeadlessExitsGracefullyWhenDisabledTopologyStep(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, ".badger-disable"), []byte("opt-out\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.badger-disable) error = %v", err)
	}

	inputFile := writeTempInput(t, "add logging")

	cfg := DefaultConfig()
	cfg.Root = tmpDir

	var output bytes.Buffer
	if err := RunHeadless(cfg, HeadlessOptions{
		Step:      "topology",
		InputPath: inputFile,
		Stdout:    &output,
	}); err != nil {
		t.Fatalf("RunHeadless() error = %v, want nil", err)
	}

	if !strings.Contains(output.String(), "Project explicitly opted out.") {
		t.Fatalf("RunHeadless() output missing disabled message:\n%s", output.String())
	}
}

func writeTempInput(t *testing.T, content string) string {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "badger-input-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp() error = %v", err)
	}
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return file.Name()
}

var _ io.Reader = (*strings.Reader)(nil)
