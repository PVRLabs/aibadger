package badger

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
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

func TestRunAPIExtractMatchesEngineSchemaB(t *testing.T) {
	root := writeAPIExtractionProject(t)
	selectorsPath := filepath.Join(t.TempDir(), "selectors.txt")
	selectors := []byte(strings.Join([]string{
		"FILE:go.mod",
		"PREFIX:main.go#func alpha",
		"NEAR:main.go#func beta",
		"FILE:go.mod",
		"FILE:preview.png",
	}, "\n"))
	if err := os.WriteFile(selectorsPath, selectors, 0644); err != nil {
		t.Fatalf("WriteFile(selectors) error = %v", err)
	}
	goalPath := filepath.Join(t.TempDir(), "goal.txt")
	goal := []byte("Explain the extraction paths")
	if err := os.WriteFile(goalPath, goal, 0644); err != nil {
		t.Fatalf("WriteFile(goal) error = %v", err)
	}

	cfg := Config{Root: root}
	var stdout, stderr bytes.Buffer
	err := RunAPI(cfg, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}

	fullCfg := cfg.withDefaults()
	eng, err := engine.New(root, fullCfg.MaxFilesPerDirectory)
	if err != nil {
		t.Fatalf("engine.New() error = %v", err)
	}
	workflow.ConfigureEngine(eng, headlessEngineOptions(fullCfg, HeadlessOptions{}))
	session := workflow.NewSession(eng, writer.WhitespaceMode(fullCfg.WhitespaceMode))
	parsed := session.ParseExtractionInputDetailed(string(selectors))
	want, _, _, _, _, err := session.GenerateContextDetailed(string(goal), parsed.Commands)
	if err != nil {
		t.Fatalf("GenerateContextDetailed() error = %v", err)
	}
	if got := stdout.String(); got != want {
		t.Fatalf("extract stdout differs from engine Schema B\nstdout:\n%s\nwant:\n%s", got, want)
	}
	for _, wantText := range []string{
		"[PROJECT TOPOLOGY]",
		"[TASK]\n" + string(goal),
		"[OUTPUT CONSTRAINT]",
		"[CONTEXT]",
		"--- File: go.mod (Full File) ---",
		"--- File: main.go (Extracted Span) ---",
		"--- File: preview.png (Binary Summary) ---",
	} {
		if !strings.Contains(stdout.String(), wantText) {
			t.Fatalf("extract stdout missing %q:\n%s", wantText, stdout.String())
		}
	}
	if strings.Count(stdout.String(), "--- File: go.mod") != 1 {
		t.Fatalf("duplicate FILE selector was not deduplicated:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for path, want := range map[string][]byte{selectorsPath: selectors, goalPath: goal} {
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, want) {
			t.Fatalf("input %s after RunAPI() = %q, %v; want unchanged %q", path, got, err, want)
		}
	}
}

func TestRunAPIExtractFocusDefaultsToCodeAndSupportsDesign(t *testing.T) {
	root := writeAPIExtractionProject(t)
	selectorsPath := writeAPITestInput(t, "selectors.txt", "FILE:main.go")
	goalPath := writeAPITestInput(t, "goal.txt", "Design the extraction API")

	for _, tt := range []struct {
		name  string
		focus protocol.Focus
		want  string
		avoid string
	}{
		{name: "omitted defaults to code", want: "This is the final-answer step.", avoid: "This is the final-answer step for a design task."},
		{name: "design", focus: protocol.FocusDesign, want: "This is the final-answer step for a design task.", avoid: "full updated file contents"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunAPI(Config{Root: root}, APIOptions{
				Operation:    "extract",
				InputPath:    selectorsPath,
				GoalFilePath: goalPath,
				Focus:        tt.focus,
				Stdout:       &stdout,
				Stderr:       &stderr,
			})
			if err != nil {
				t.Fatalf("RunAPI() error = %v", err)
			}
			if !strings.Contains(stdout.String(), tt.want) || strings.Contains(stdout.String(), tt.avoid) {
				t.Fatalf("extract focus output = %q, want %q and no %q", stdout.String(), tt.want, tt.avoid)
			}
		})
	}
}

func TestRunAPIExtractEmitsPartialPromptAndWarnings(t *testing.T) {
	root := writeAPIExtractionProject(t)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "compiled.jar"), []byte{0, 1, 2}, 0644); err != nil {
		t.Fatalf("WriteFile(compiled.jar) error = %v", err)
	}
	selectorsPath := writeAPITestInput(t, "selectors.txt", strings.Join([]string{
		"FILE:main.go",
		"UNKNOWN:ignored.go",
		"FILE:missing.go",
		"FILE:.env",
		"FILE:compiled.jar",
	}, "\n"))
	goalPath := writeAPITestInput(t, "goal.txt", "Inspect partial extraction")

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() partial error = %v, want usable Prompt 2", err)
	}
	if !strings.Contains(stdout.String(), "--- File: main.go (Full File) ---") {
		t.Fatalf("stdout missing usable extraction:\n%s", stdout.String())
	}
	for _, forbidden := range []string{"TOKEN=secret", "compiled.jar (Full File)", "Extracted 1 file with warnings"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("stdout contains diagnostic or excluded content %q:\n%s", forbidden, stdout.String())
		}
	}
	for _, want := range []string{
		"Extracted 1 file with warnings",
		"invalid or unsupported selector",
		"missing.go",
		".env: excluded from Prompt 2",
		"compiled.jar: excluded from Prompt 2",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunAPIExtractFailsWithoutUsableContext(t *testing.T) {
	root := writeAPIExtractionProject(t)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret\n"), 0644); err != nil {
		t.Fatalf("WriteFile(.env) error = %v", err)
	}
	selectorsPath := writeAPITestInput(t, "selectors.txt", "FILE:.env")
	goalPath := writeAPITestInput(t, "goal.txt", "Inspect exclusions")

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if !errors.Is(err, extractor.ErrNoSafePrompt2Files) {
		t.Fatalf("RunAPI() error = %v, want ErrNoSafePrompt2Files", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunAPIExtractReportsMalformedOnlyInput(t *testing.T) {
	root := writeAPIExtractionProject(t)
	selectorsPath := writeAPITestInput(t, "selectors.txt", "PREFIX:main.go\nUNKNOWN:file.go")
	goalPath := writeAPITestInput(t, "goal.txt", "Inspect malformed selectors")

	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err == nil || !strings.Contains(err.Error(), "no valid extraction selectors") {
		t.Fatalf("RunAPI() error = %v, want no valid selectors", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	for _, want := range []string{"PREFIX requires path#pattern", "invalid or unsupported selector"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
}

func TestRunAPIExtractRejectsInvalidCallerFiles(t *testing.T) {
	root := writeAPIExtractionProject(t)
	validSelectors := writeAPITestInput(t, "selectors.txt", "FILE:main.go")
	validGoal := writeAPITestInput(t, "goal.txt", "Inspect input validation")
	emptySelectors := writeAPITestInput(t, "empty-selectors.txt", " \n")
	emptyGoal := writeAPITestInput(t, "empty-goal.txt", "\t")
	invalidGoal := filepath.Join(t.TempDir(), "invalid-goal.txt")
	if err := os.WriteFile(invalidGoal, []byte{0xff}, 0644); err != nil {
		t.Fatalf("WriteFile(invalid goal) error = %v", err)
	}

	for _, tt := range []struct {
		name      string
		selectors string
		goal      string
		want      string
	}{
		{name: "missing selector file", selectors: filepath.Join(root, "missing.txt"), goal: validGoal, want: "reading api input file"},
		{name: "missing goal file", selectors: validSelectors, goal: filepath.Join(root, "missing-goal.txt"), want: "reading api goal file"},
		{name: "empty selectors", selectors: emptySelectors, goal: validGoal, want: "api extract input file is empty"},
		{name: "empty goal", selectors: validSelectors, goal: emptyGoal, want: "api extract goal file is empty"},
		{name: "invalid UTF-8 goal", selectors: validSelectors, goal: invalidGoal, want: "invalid UTF-8"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunAPI(Config{Root: root}, APIOptions{
				Operation:    "extract",
				InputPath:    tt.selectors,
				GoalFilePath: tt.goal,
				Stdout:       &stdout,
				Stderr:       &stderr,
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

func TestRunAPIExtractPreservesLimitsAndExternalContext(t *testing.T) {
	root := writeAPIExtractionProject(t)
	externalOne := t.TempDir()
	externalTwo := t.TempDir()
	externalContent := []byte(strings.Repeat("external context ", 20))
	uniquePath := filepath.Join(externalOne, "unique.md")
	if err := os.WriteFile(uniquePath, externalContent, 0644); err != nil {
		t.Fatalf("WriteFile(unique) error = %v", err)
	}
	for _, externalRoot := range []string{externalOne, externalTwo} {
		if err := os.WriteFile(filepath.Join(externalRoot, "duplicate.md"), []byte("ambiguous\n"), 0644); err != nil {
			t.Fatalf("WriteFile(duplicate) error = %v", err)
		}
	}
	contextConfig := externalOne + "\n" + externalTwo + "\n"
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte(contextConfig), 0644); err != nil {
		t.Fatalf("WriteFile(.badger-context) error = %v", err)
	}
	selectorsPath := writeAPITestInput(t, "selectors.txt", "FILE:unique.md\nFILE:duplicate.md\nFILE:main.go")
	goalPath := writeAPITestInput(t, "goal.txt", "Inspect external context")
	cfg := Config{Root: root, MaxContextFileBytes: 40, MaxTotalContextBytes: 1100}

	var stdout, stderr bytes.Buffer
	err := RunAPI(cfg, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}
	if !strings.Contains(stdout.String(), "unique.md") || !strings.Contains(stdout.String(), "Truncated") {
		t.Fatalf("stdout missing external truncated context:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Ambiguous file reference: duplicate.md") {
		t.Fatalf("stderr missing external ambiguity:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "TRUNCATED") {
		t.Fatalf("stderr missing size-limit metadata:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), "DROPPED - EXCEEDS TOTAL LIMIT") {
		t.Fatalf("stderr missing total-limit metadata:\n%s", stderr.String())
	}
	if got, err := os.ReadFile(uniquePath); err != nil || !bytes.Equal(got, externalContent) {
		t.Fatalf("external context after RunAPI() = %q, %v; want unchanged", got, err)
	}
}

func writeAPIExtractionProject(t *testing.T) string {
	t.Helper()
	root := writeAPITestProject(t)
	mainSource := `package main

func alpha() {
	println("alpha")
}

func beta() {
	println("beta")
}
`
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainSource), 0644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.png"), []byte("not a decoded image"), 0644); err != nil {
		t.Fatalf("WriteFile(preview.png) error = %v", err)
	}
	return root
}

func writeAPITestInput(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
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
