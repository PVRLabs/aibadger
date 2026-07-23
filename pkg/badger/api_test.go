package badger

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
)

func TestRunAPIReadsInputWithoutModifyingIt(t *testing.T) {
	root := t.TempDir()
	inputPath := filepath.Join(root, "goal.txt")
	input := []byte("inspect the API boundary\n")
	if err := os.WriteFile(inputPath, input, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "goal", InputPath: inputPath, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}
	if got, err := os.ReadFile(inputPath); err != nil || !bytes.Equal(got, input) {
		t.Fatalf("goal input after RunAPI() = %q, %v; want unchanged %q", got, err, input)
	}
	if got := stdout.String(); got != "Dev goal: inspect the API boundary\n\n" {
		t.Fatalf("stdout = %q", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAPIRejectsInvalidInputsBeforeWritingOutput(t *testing.T) {
	root := t.TempDir()
	nonUTF8 := filepath.Join(root, "bad-input.txt")
	if err := os.WriteFile(nonUTF8, []byte{0xff}, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	for _, tt := range []struct {
		name string
		cfg  Config
		opts APIOptions
		want string
	}{
		{name: "missing root", opts: APIOptions{Operation: "scan"}, want: "requires --root"},
		{name: "file root", cfg: Config{Root: nonUTF8}, opts: APIOptions{Operation: "scan"}, want: "not a directory"},
		{name: "missing input", cfg: Config{Root: root}, opts: APIOptions{Operation: "goal"}, want: "requires --input"},
		{name: "unreadable input", cfg: Config{Root: root}, opts: APIOptions{Operation: "goal", InputPath: filepath.Join(root, "missing.txt")}, want: "reading api input file"},
		{name: "non UTF8 input", cfg: Config{Root: root}, opts: APIOptions{Operation: "goal", InputPath: nonUTF8}, want: "invalid UTF-8"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			tt.opts.Stdout = &stdout
			tt.opts.Stderr = &stderr
			err := RunAPI(tt.cfg, tt.opts)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunAPI() error = %v, want %q", err, tt.want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("output = stdout %q stderr %q, want empty", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunAPIReportsDisabledProjectOnStderr(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, engine.DisableFileName), nil, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "topology", Stdout: &stdout, Stderr: &stderr})
	if !errors.Is(err, engine.ErrProjectDisabled) {
		t.Fatalf("RunAPI() error = %v, want ErrProjectDisabled", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), ".badger-disable") {
		t.Fatalf("stderr = %q, want disable diagnostic", stderr.String())
	}
}

func TestRunAPITopologyMatchesEngineFormatter(t *testing.T) {
	root := writeAPITestProject(t)
	cfg := Config{Root: root}

	var stdout, stderr bytes.Buffer
	if err := RunAPI(cfg, APIOptions{Operation: "topology", Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}

	fullCfg := cfg.withDefaults()
	eng, err := engine.New(root, fullCfg.MaxFilesPerDirectory)
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	workflow.ConfigureEngine(eng, headlessEngineOptions(fullCfg, HeadlessOptions{}))
	if got, want := stdout.String(), eng.GenerateTopology(); got != want {
		t.Fatalf("topology stdout differs from engine formatter\nstdout:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("topology stdout contains absolute root %q:\n%s", root, stdout.String())
	}
	if strings.Contains(stdout.String(), "[TASK]") {
		t.Fatalf("standalone topology contains Prompt 1 task:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAPIPromptMatchesSchemaAAndSeparatesWarnings(t *testing.T) {
	root := writeAPITestProject(t)
	externalRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(externalRoot, "spec.md"), []byte("# External spec\n"), 0644); err != nil {
		t.Fatalf("WriteFile(external spec) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte(externalRoot+"\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.badger-context) error = %v", err)
	}
	goalPath := filepath.Join(t.TempDir(), "goal.txt")
	goal := []byte("Design the API around @main.go and @missing.go")
	if err := os.WriteFile(goalPath, goal, 0644); err != nil {
		t.Fatalf("WriteFile(goal) error = %v", err)
	}

	cfg := Config{Root: root}
	var stdout, stderr bytes.Buffer
	err := RunAPI(cfg, APIOptions{
		Operation: "prompt",
		InputPath: goalPath,
		Focus:     protocol.FocusDesign,
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}

	fullCfg := cfg.withDefaults()
	eng, err := engine.New(root, fullCfg.MaxFilesPerDirectory)
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	workflow.ConfigureEngine(eng, headlessEngineOptions(fullCfg, HeadlessOptions{Focus: protocol.FocusDesign}))
	want, warnings := eng.GenerateMapDetailed(string(goal))
	if got := stdout.String(); got != want {
		t.Fatalf("prompt stdout differs from engine Schema A\nstdout:\n%s\nwant:\n%s", got, want)
	}
	if len(warnings) != 1 {
		t.Fatalf("engine warnings = %v, want one tagged-file warning", warnings)
	}
	for _, wantText := range []string{
		"[EXTERNAL CONTEXT]",
		"[USER TAGGED FILES]",
		"FILE:main.go",
		"[TASK]\n" + string(goal),
		"Do not implement the design yet.",
	} {
		if !strings.Contains(stdout.String(), wantText) {
			t.Fatalf("prompt stdout missing %q:\n%s", wantText, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "Tagged file warnings") || !strings.Contains(stderr.String(), "Tagged file warnings") {
		t.Fatalf("warning stream separation failed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if got, err := os.ReadFile(goalPath); err != nil || !bytes.Equal(got, goal) {
		t.Fatalf("goal after RunAPI() = %q, %v; want unchanged %q", got, err, goal)
	}
}

func TestRunAPIPromptRejectsEmptyGoalAndUnsupportedFocus(t *testing.T) {
	root := writeAPITestProject(t)
	emptyPath := filepath.Join(t.TempDir(), "goal.txt")
	if err := os.WriteFile(emptyPath, []byte(" \n\t"), 0644); err != nil {
		t.Fatalf("WriteFile(empty goal) error = %v", err)
	}

	for _, tt := range []struct {
		name  string
		focus protocol.Focus
		want  string
	}{
		{name: "empty", focus: protocol.FocusCode, want: "input file is empty"},
		{name: "review", focus: protocol.FocusReview, want: "requires --focus <code|design>"},
		{name: "followup", focus: protocol.FocusFollowup, want: "requires --focus <code|design>"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunAPI(Config{Root: root}, APIOptions{
				Operation: "prompt",
				InputPath: emptyPath,
				Focus:     tt.focus,
				Stdout:    &stdout,
				Stderr:    &stderr,
			})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("RunAPI() error = %v, want %q", err, tt.want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("output = stdout %q stderr %q, want empty", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunAPIPromptSupportsCodeFocus(t *testing.T) {
	root := writeAPITestProject(t)
	goalPath := filepath.Join(t.TempDir(), "goal.txt")
	if err := os.WriteFile(goalPath, []byte("Implement the API"), 0644); err != nil {
		t.Fatalf("WriteFile(goal) error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation: "prompt",
		InputPath: goalPath,
		Focus:     protocol.FocusCode,
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "Do not solve this yet.") {
		t.Fatalf("code prompt missing code-focus constraint:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "Do not implement the design yet.") {
		t.Fatalf("code prompt contains design-focus constraint:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func writeAPITestProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/api\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatalf("WriteFile(go.mod) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	return root
}
