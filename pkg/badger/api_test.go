package badger

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

func TestRunAPIReviewContextProducesStablePrompt(t *testing.T) {
	root := writeAPIReviewRepo(t)
	guidance := writeAPITestInput(t, "guidance.txt", "Check concurrency and error handling.\n")
	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation: "review-context", InputPath: guidance,
		Stdout: &stdout, Stderr: &stderr,
	})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}
	for _, want := range []string{"Review guidance:\nCheck concurrency", "Authoritative tracked Git diff:", "+const changed = true"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), root) {
		t.Fatalf("stdout leaked absolute root %q", root)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAPIReviewContextReturnsStdoutWriteFailure(t *testing.T) {
	root := writeAPIReviewRepo(t)
	stdout := &failAfterWriter{remaining: 32, err: io.ErrClosedPipe}
	err := RunAPI(Config{Root: root}, APIOptions{
		Operation: "review-context",
		Stdout:    stdout,
	})
	if err == nil {
		t.Fatal("RunAPI() error = nil, want stdout write failure")
	}
	if !errors.Is(err, io.ErrClosedPipe) || !strings.Contains(err.Error(), "writing review context") {
		t.Fatalf("RunAPI() error = %v, want wrapped closed-pipe write error", err)
	}
	if got := stdout.written; got != 32 {
		t.Fatalf("stdout bytes written = %d, want 32-byte partial prefix", got)
	}
}

type failAfterWriter struct {
	remaining int
	written   int
	err       error
}

func (w *failAfterWriter) Write(p []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, w.err
	}
	n := len(p)
	if n > w.remaining {
		n = w.remaining
	}
	w.remaining -= n
	w.written += n
	if n < len(p) {
		return n, w.err
	}
	return n, nil
}

func TestRunAPIReviewContextSelectedPathsAndFailures(t *testing.T) {
	root := writeAPIReviewRepo(t)
	if err := os.WriteFile(filepath.Join(root, "other.go"), []byte("package main\nconst other = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gone.go"), []byte("package main\nconst gone = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runAPIGit(t, root, "add", "gone.go")
	runAPIGit(t, root, "commit", "-m", "add deleted fixture")
	if err := os.Remove(filepath.Join(root, "gone.go")); err != nil {
		t.Fatal(err)
	}
	paths := writeAPITestInput(t, "paths.json", `["main.go","main.go"]`)
	var stdout, stderr bytes.Buffer
	if err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", PathsFilePath: paths, Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatalf("RunAPI(selected) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "main.go") || strings.Contains(stdout.String(), "other.go") {
		t.Fatalf("selected stdout has wrong scope:\n%s", stdout.String())
	}
	deletedPaths := writeAPITestInput(t, "deleted-paths.json", `["gone.go"]`)
	stdout.Reset()
	if err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", PathsFilePath: deletedPaths, Stdout: &stdout, Stderr: &stderr}); err != nil {
		t.Fatalf("RunAPI(deleted) error = %v", err)
	}
	if !strings.Contains(stdout.String(), "gone.go\tdiff-only-deleted") {
		t.Fatalf("deleted stdout missing disposition:\n%s", stdout.String())
	}

	badPaths := writeAPITestInput(t, "bad-paths.json", `["../escape.go"]`)
	stdout.Reset()
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", PathsFilePath: badPaths, Stdout: &stdout, Stderr: &stderr})
	if err == nil || !strings.Contains(err.Error(), "invalid selected path") || stdout.Len() != 0 {
		t.Fatalf("escaping selection result = error %v, stdout %q", err, stdout.String())
	}

	stdout.Reset()
	err = RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", MaxReviewPayloadBytes: 10, Stdout: &stdout, Stderr: &stderr})
	if err == nil || !strings.Contains(err.Error(), "mandatory content exceeds") || stdout.Len() != 0 {
		t.Fatalf("overflow result = error %v, stdout %q", err, stdout.String())
	}
}

func TestRunAPIReviewContextNoChangesAndNotGitProduceNoOutput(t *testing.T) {
	clean := writeAPIReviewRepo(t)
	runAPIGit(t, clean, "add", "main.go")
	runAPIGit(t, clean, "commit", "-m", "changed")
	for name, root := range map[string]string{"clean": clean, "not git": t.TempDir()} {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", Stdout: &stdout, Stderr: &stderr})
			if err == nil || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("RunAPI() = error %v, stdout %q, stderr %q", err, stdout.String(), stderr.String())
			}
			if strings.Contains(err.Error(), root) {
				t.Fatalf("error leaked absolute root: %v", err)
			}
		})
	}
}

func TestRunAPIReviewContextModesAndInvalidRef(t *testing.T) {
	root := writeAPIReviewRepo(t)
	runAPIGit(t, root, "add", "main.go")
	for _, tt := range []struct {
		name string
		mode string
		ref  string
	}{
		{name: "staged", mode: "staged"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", ReviewMode: tt.mode, ReviewRef: tt.ref, Stdout: &stdout}); err != nil {
				t.Fatalf("RunAPI() error = %v", err)
			}
			if !strings.Contains(stdout.String(), "+const changed = true") {
				t.Fatalf("stdout missing staged change:\n%s", stdout.String())
			}
		})
	}
	runAPIGit(t, root, "commit", "-m", "changed")
	for _, tt := range []struct{ mode, ref string }{{"commit", "HEAD"}, {"branch", "HEAD~1"}} {
		t.Run(tt.mode, func(t *testing.T) {
			var stdout bytes.Buffer
			if err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", ReviewMode: tt.mode, ReviewRef: tt.ref, Stdout: &stdout}); err != nil {
				t.Fatalf("RunAPI() error = %v", err)
			}
			if !strings.Contains(stdout.String(), "+const changed = true") {
				t.Fatalf("stdout missing %s change:\n%s", tt.mode, stdout.String())
			}
		})
	}
	var stdout bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-context", ReviewMode: "commit", ReviewRef: "missing-ref", Stdout: &stdout})
	if err == nil || stdout.Len() != 0 {
		t.Fatalf("invalid ref = error %v, stdout %q", err, stdout.String())
	}
}

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

	var repeatedStdout, repeatedStderr bytes.Buffer
	if err := RunAPI(cfg, APIOptions{Operation: "topology", Stdout: &repeatedStdout, Stderr: &repeatedStderr}); err != nil {
		t.Fatalf("repeated RunAPI() error = %v", err)
	}
	if got, want := repeatedStdout.String(), stdout.String(); got != want {
		t.Fatalf("topology changed for unchanged repository\nfirst:\n%s\nsecond:\n%s", want, got)
	}
	if repeatedStderr.Len() != 0 {
		t.Fatalf("repeated stderr = %q, want empty", repeatedStderr.String())
	}
}

func TestRunAPITopologyFailureProducesNoNormalOutput(t *testing.T) {
	missingRoot := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer

	err := RunAPI(Config{Root: missingRoot}, APIOptions{
		Operation: "topology",
		Stdout:    &stdout,
		Stderr:    &stderr,
	})
	if err == nil {
		t.Fatal("RunAPI() error = nil, want invalid-root failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want command layer to report returned error", stderr.String())
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

func TestRunAPIReviewContinuationProducesOnlySupplementalCurrentContext(t *testing.T) {
	root := writeAPIExtractionProject(t)
	selectors := writeAPITestInput(t, "review-selectors.txt", "FILE:main.go\nFILE:main.go\nNEAR:go.mod#module")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc continuationState() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-continuation", InputPath: selectors, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("RunAPI() error = %v", err)
	}
	for _, want := range []string{"[REVIEW CONTINUATION]", "filesystem at continuation time", "continuationState", "--- File: go.mod (Full File) ---"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout missing %q:\n%s", want, stdout.String())
		}
	}
	for _, forbidden := range []string{"[PROJECT TOPOLOGY]", "diff --git"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("stdout repeats initial context %q:\n%s", forbidden, stdout.String())
		}
	}
	if strings.Count(stdout.String(), "--- File: main.go") != 1 {
		t.Fatalf("duplicate selector was not deduplicated:\n%s", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunAPIReviewContinuationClassifiesResponsesWithoutParsingFindings(t *testing.T) {
	root := writeAPIExtractionProject(t)
	for _, tt := range []struct{ name, input, want string }{
		{name: "empty", input: " \n", want: "input file is empty"},
		{name: "findings", input: "High: main.go has a race", want: "contains no selectors"},
		{name: "mixed", input: "FILE:main.go\nHigh: inspect this race", want: "mixes selectors with findings or invalid text"},
		{name: "oversized mixed", input: "FILE:main.go\n" + strings.Repeat("finding prose ", 6000), want: "mixes selectors with findings or invalid text"},
		{name: "invalid", input: "PREFIX:main.go", want: "contains no selectors"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			input := writeAPITestInput(t, tt.name+".txt", tt.input)
			var stdout, stderr bytes.Buffer
			err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-continuation", InputPath: input, Stdout: &stdout, Stderr: &stderr})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("output = stdout %q stderr %q, want empty", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunAPIReviewContinuationPartialSafetyAndLimits(t *testing.T) {
	root := writeAPIExtractionProject(t)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SECRET=value\n"), 0644); err != nil {
		t.Fatal(err)
	}
	selectors := writeAPITestInput(t, "partial-review-selectors.txt", "FILE:main.go\nFILE:missing.go\nFILE:.env\nFILE:preview.png")
	var stdout, stderr bytes.Buffer
	err := RunAPI(Config{Root: root}, APIOptions{Operation: "review-continuation", InputPath: selectors, MaxReviewFileBytes: 32, Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("RunAPI() partial error = %v", err)
	}
	if !strings.Contains(stdout.String(), "main.go (Full File, Truncated)") || !strings.Contains(stdout.String(), "preview.png (Binary Summary, Truncated)") {
		t.Fatalf("stdout missing bounded usable context:\n%s", stdout.String())
	}
	if strings.Contains(stdout.String(), "SECRET=value") {
		t.Fatalf("stdout leaked sensitive content:\n%s", stdout.String())
	}
	for _, want := range []string{"missing.go", ".env: excluded", "Extracted 2 files with warnings"} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("stderr missing %q:\n%s", want, stderr.String())
		}
	}
	stdout.Reset()
	stderr.Reset()
	err = RunAPI(Config{Root: root}, APIOptions{Operation: "review-continuation", InputPath: selectors, MaxReviewPayloadBytes: 1, Stdout: &stdout, Stderr: &stderr})
	if err == nil || !strings.Contains(err.Error(), "leaves no usable supplemental context") || stdout.Len() != 0 {
		t.Fatalf("tiny limit = error %v stdout %q", err, stdout.String())
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
	const (
		perFileLimit   = 8 * 1024
		promptTwoLimit = 12 * 1024
		sourceSize     = 16 * 1024
	)
	// The aggregate limit fits one truncated file but not two.
	// The multi-KiB margin keeps the test independent of path and formatter
	// overhead.

	// Layout:
	//   <sandbox>/
	//     project/          ← root for RunAPI
	//     external-one/     ← sibling, referenced via relative .badger-context
	//     external-two/
	sandbox := t.TempDir()
	root := filepath.Join(sandbox, "project")
	ext1 := filepath.Join(sandbox, "external-one")
	ext2 := filepath.Join(sandbox, "external-two")
	for _, dir := range []string{root, ext1, ext2} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}

	// Populate project root.
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module ex\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mainBody := "package main\n" + strings.Repeat("var _ = 0\n", (sourceSize-len("package main\n"))/len("var _ = 0\n"))
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte(mainBody), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "preview.png"), []byte("not a decoded image"), 0644); err != nil {
		t.Fatal(err)
	}

	// Populate external context directories.
	extContent := bytes.Repeat([]byte("x"), sourceSize)
	uniquePath := filepath.Join(ext1, "unique.md")
	if err := os.WriteFile(uniquePath, extContent, 0644); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{ext1, ext2} {
		if err := os.WriteFile(filepath.Join(dir, "duplicate.md"), []byte("ambiguous\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// External context is configured via relative paths (siblings).
	ctxCfg := "../external-one\n../external-two\n"
	if err := os.WriteFile(filepath.Join(root, ".badger-context"), []byte(ctxCfg), 0644); err != nil {
		t.Fatal(err)
	}

	selectorsPath := writeAPITestInput(t, "selectors.txt",
		"FILE:unique.md\nFILE:duplicate.md\nFILE:main.go")
	goalPath := writeAPITestInput(t, "goal.txt", "Inspect external context")
	cfg := Config{
		Root:                root,
		MaxContextFileBytes: perFileLimit,
		MaxPromptTwoBytes:   promptTwoLimit,
	}

	var stdout, stderr bytes.Buffer
	if err := RunAPI(cfg, APIOptions{
		Operation:    "extract",
		InputPath:    selectorsPath,
		GoalFilePath: goalPath,
		Stdout:       &stdout,
		Stderr:       &stderr,
	}); err != nil {
		t.Fatalf("RunAPI: %v", err)
	}

	stdoutText := stdout.String()
	stderrText := stderr.String()

	// --- Ambiguous file diagnostics ----------------------------------------
	if !strings.Contains(stderrText, "Ambiguous file reference: duplicate.md") {
		t.Fatalf("stderr missing ambiguity:\n%s", stderrText)
	}

	// --- Per-file truncation metadata (both valid files exceed perFileLimit) -
	if !strings.Contains(stderrText, "TRUNCATED") {
		t.Fatalf("stderr missing truncation:\n%s", stderrText)
	}

	// --- Total-limit drop metadata -----------------------------------------
	if !strings.Contains(stderrText, "DROPPED - EXCEEDS TOTAL LIMIT") {
		t.Fatalf("stderr missing total-limit drop:\n%s", stderrText)
	}
	dropped := strings.Count(stderrText, "DROPPED - EXCEEDS TOTAL LIMIT")
	if dropped != 1 {
		t.Fatalf("expected 1 dropped file, got %d:\n%s", dropped, stderrText)
	}

	// The ambiguous request must not be counted as a total-limit drop.
	for _, line := range strings.Split(stderrText, "\n") {
		if strings.Contains(line, "duplicate.md") && strings.Contains(line, "DROPPED") {
			t.Fatalf("ambiguous file must not appear as dropped:\n%s", line)
		}
	}

	// --- Surviving file present in stdout ----------------------------------
	extDisplayPath := "../external-one/unique.md"
	if !strings.Contains(stdoutText, extDisplayPath) {
		t.Fatalf("stdout missing surviving file %q:\n%s", extDisplayPath, stdoutText)
	}
	if !strings.Contains(stdoutText, "Truncated") {
		t.Fatalf("stdout missing Truncated label:\n%s", stdoutText)
	}

	// --- Dropped file absent from stdout (no file block) -------------------
	if strings.Contains(stdoutText, "--- File: main.go ") {
		t.Fatalf("stdout contains dropped file section:\n%s", stdoutText)
	}

	// --- External context file not modified --------------------------------
	if got, err := os.ReadFile(uniquePath); err != nil || !bytes.Equal(got, extContent) {
		t.Fatalf("external content after RunAPI = %q, %v; want unchanged", got, err)
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

func writeAPIReviewRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runAPIGit(t, root, "init")
	runAPIGit(t, root, "config", "user.email", "test@example.com")
	runAPIGit(t, root, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nconst changed = false\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runAPIGit(t, root, "add", "main.go")
	runAPIGit(t, root, "commit", "-m", "initial")
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\nconst changed = true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runAPIGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
