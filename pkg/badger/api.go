package badger

// This file owns the non-interactive API runner used by external tools and
// certification. It deliberately does not enter the headless session: callers
// provide all input up front and own both output streams.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
)

// APIOptions describes one non-interactive API invocation. InputPath is read
// exactly once and is never modified. Topology, prompt, extract, and the review
// operations are the
// stable text-first operations; the other current operations support
// certification.
type APIOptions struct {
	Operation             string
	InputPath             string
	GoalFilePath          string
	Focus                 protocol.Focus
	ReviewMode            string
	ReviewRef             string
	PathsFilePath         string
	MaxReviewPayloadBytes int
	MaxReviewFileBytes    int
	IncludeReviewTopology bool
	MaxFilesPerDirectory  int
	Stdout                io.Writer
	Stderr                io.Writer
}

// RunAPI executes a non-interactive API operation. It never reads stdin,
// changes settings, asks for confirmation, or accesses the clipboard.
func RunAPI(cfg Config, opts APIOptions) error {
	return runAPIWithReviewBuilder(cfg, opts, reviewtask.BuildInitialReviewPayload)
}

// reviewPayloadBuilder is injected by presentation/facade tests so they can
// exercise output and error mapping without repeatedly creating Git
// repositories. The production entry point above always uses the real
// reviewtask builder.
type reviewPayloadBuilder func(string, reviewtask.Options) (reviewtask.InitialReviewResult, error)

func runAPIWithReviewBuilder(cfg Config, opts APIOptions, buildReview reviewPayloadBuilder) error {
	if err := validateAPIOperation(opts); err != nil {
		return err
	}

	root, err := normalizeAPIRoot(cfg.Root)
	if err != nil {
		return err
	}
	cfg.Root = root
	cfg = cfg.withDefaults()

	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	var input string
	if opts.Operation == "review-context" || opts.Operation == "review-continuation" {
		input, err = readAPIFileLimited(opts.InputPath, "api input file", maxReviewAPIInputBytes)
	} else {
		input, err = readAPIInput(opts.InputPath)
	}
	if err != nil {
		return err
	}
	goal, err := readAPIFile(opts.GoalFilePath, "api goal file")
	if err != nil {
		return err
	}
	if opts.Operation == "prompt" && strings.TrimSpace(input) == "" {
		return fmt.Errorf("api prompt input file is empty")
	}
	if opts.Operation == "extract" {
		if strings.TrimSpace(input) == "" {
			return fmt.Errorf("api extract input file is empty")
		}
		if strings.TrimSpace(goal) == "" {
			return fmt.Errorf("api extract goal file is empty")
		}
	}
	if opts.Operation == "review-continuation" && strings.TrimSpace(input) == "" {
		return fmt.Errorf("api review-continuation input file is empty")
	}
	if err := engine.CheckDisabled(cfg.Root); err != nil {
		if errors.Is(err, engine.ErrProjectDisabled) {
			fmt.Fprintln(stderr, "project explicitly disabled via .badger-disable")
		}
		return err
	}
	if opts.Operation == "review-context" {
		opts.MaxFilesPerDirectory = cfg.MaxFilesPerDirectory
		return runReviewContextAPIWithBuilder(cfg.Root, input, opts, stdout, buildReview)
	}
	if opts.Operation == "review-continuation" {
		return runReviewContinuationAPI(cfg, input, opts, stdout, stderr)
	}

	scanOutput := scanOutputSilent
	if opts.Operation == "scan" {
		fmt.Fprintf(stdout, "%s — Headless\n", Name)
		fmt.Fprintln(stdout, "----------------------")
		scanOutput = scanOutputStable
	}
	eng, err := scanProject(stdout, cfg.Root, scanOutput, cfg.MaxFilesPerDirectory)
	if err != nil {
		return fmt.Errorf("scanning: %w", err)
	}
	if opts.Operation == "scan" {
		return nil
	}

	workflow.ConfigureEngine(eng, headlessEngineOptions(cfg, HeadlessOptions{Focus: opts.Focus}))
	session := workflow.NewSession(eng, writer.WhitespaceMode(cfg.WhitespaceMode))
	switch opts.Operation {
	case "topology":
		fmt.Fprint(stdout, eng.GenerateTopology())
	case "prompt":
		schemaA, warnings := session.GenerateMapDetailed(input)
		printTaggedFileWarnings(stderr, warnings)
		fmt.Fprint(stdout, schemaA)
	case "extract":
		parsed := session.ParseExtractionInputDetailed(input)
		if parsed.Empty {
			printExtractionWarnings(stderr, 0, parsed.Failures, nil)
			return fmt.Errorf("no valid extraction selectors")
		}
		schemaB, metadata, extractedCount, failedCommands, safetyExclusions, err := session.GenerateContextDetailed(goal, parsed.Commands)
		if err != nil {
			return err
		}
		failedCommands = append(parsed.Failures, failedCommands...)
		printExtractionWarnings(stderr, extractedCount, failedCommands, safetyExclusions)
		printExtractionMetadata(stderr, metadata)
		fmt.Fprint(stdout, schemaB)
	case "goal":
		fmt.Fprintf(stdout, "Dev goal: %s\n", input)
	case "extraction":
		printHeadlessExtractionPlan(stdout, input, session.ParseExtractionInput(input).Commands)
	case "write-plan":
		printHeadlessResponsePlan(stdout, input, session.ParseWritePlan(input))
	}
	return nil
}

func validateAPIOperation(opts APIOptions) error {
	operation, inputPath, goalFilePath, focus := opts.Operation, opts.InputPath, opts.GoalFilePath, opts.Focus
	if operation != "review-context" && operation != "review-continuation" && (opts.ReviewMode != "" || opts.ReviewRef != "" || opts.PathsFilePath != "" || opts.MaxReviewPayloadBytes != 0 || opts.MaxReviewFileBytes != 0 || opts.IncludeReviewTopology) {
		return fmt.Errorf("api %s does not accept review-context options", operation)
	}
	switch operation {
	case "scan", "topology":
		if inputPath != "" {
			return fmt.Errorf("api %s does not accept --input", operation)
		}
		if focus != "" {
			return fmt.Errorf("api %s does not accept --focus", operation)
		}
		if goalFilePath != "" {
			return fmt.Errorf("api %s does not accept --goal-file", operation)
		}
	case "prompt":
		if inputPath == "" {
			return fmt.Errorf("api prompt requires --input <file>")
		}
		if focus != protocol.FocusCode && focus != protocol.FocusDesign {
			return fmt.Errorf("api prompt requires --focus <code|design>")
		}
		if goalFilePath != "" {
			return fmt.Errorf("api prompt does not accept --goal-file")
		}
	case "extract":
		if inputPath == "" {
			return fmt.Errorf("api extract requires --input <file>")
		}
		if goalFilePath == "" {
			return fmt.Errorf("api extract requires --goal-file <file>")
		}
		// Extract predates focus-aware Prompt 2 generation. Keep callers that
		// omit the flag on the code-focused contract, while allowing Prompt 1
		// and Prompt 2 to use the same explicit focus.
		if focus != "" && focus != protocol.FocusCode && focus != protocol.FocusDesign {
			return fmt.Errorf("api extract supports --focus <code|design>")
		}
	case "review-context":
		if goalFilePath != "" || focus != "" {
			return errors.New("api review-context does not accept --focus or --goal-file")
		}
		if opts.MaxReviewPayloadBytes < 0 || opts.MaxReviewFileBytes < 0 {
			return errors.New("api review-context byte limits cannot be negative")
		}
		mode := opts.ReviewMode
		if mode == "" {
			mode = "default"
		}
		switch mode {
		case "default", "staged":
			if strings.TrimSpace(opts.ReviewRef) != "" {
				return fmt.Errorf("api review-context mode %s does not accept --ref", mode)
			}
		case "branch", "commit":
			if strings.TrimSpace(opts.ReviewRef) == "" {
				return fmt.Errorf("api review-context mode %s requires --ref <revision>", mode)
			}
		default:
			return fmt.Errorf("api review-context supports --mode <default|staged|branch|commit>; got %q", mode)
		}
		if opts.PathsFilePath != "" && mode != "default" {
			return fmt.Errorf("api review-context mode %s does not accept --paths-file", mode)
		}
	case "review-continuation":
		if inputPath == "" {
			return errors.New("api review-continuation requires --input <file>")
		}
		if goalFilePath != "" || focus != "" || opts.ReviewMode != "" || opts.ReviewRef != "" || opts.PathsFilePath != "" || opts.IncludeReviewTopology {
			return errors.New("api review-continuation accepts only --root, --input, --max-payload-bytes, and --max-file-bytes")
		}
		if opts.MaxReviewPayloadBytes < 0 || opts.MaxReviewFileBytes < 0 {
			return errors.New("api review-continuation byte limits cannot be negative")
		}
	case "goal", "extraction", "write-plan":
		if inputPath == "" {
			return fmt.Errorf("api %s requires --input <file>", operation)
		}
		if focus != "" {
			return fmt.Errorf("api %s does not accept --focus", operation)
		}
		if goalFilePath != "" {
			return fmt.Errorf("api %s does not accept --goal-file", operation)
		}
	default:
		return fmt.Errorf("unknown api operation: %s", operation)
	}
	return nil
}

func runReviewContinuationAPI(cfg Config, input string, api APIOptions, stdout, stderr io.Writer) error {
	eng := engine.FromTopology(cfg.Root, nil)
	maxPayload := cfg.MaxPromptTwoBytes
	if api.MaxReviewPayloadBytes > 0 {
		maxPayload = api.MaxReviewPayloadBytes
	}
	maxFile := cfg.MaxContextFileBytes
	if api.MaxReviewFileBytes > 0 {
		maxFile = api.MaxReviewFileBytes
	}
	workflow.ConfigureEngine(eng, workflow.EngineOptions{MaxContextFileBytes: maxFile, MaxPromptTwoBytes: maxPayload, Focus: protocol.FocusReview})
	session := workflow.NewSession(eng, writer.WhitespaceMode(cfg.WhitespaceMode))
	parsed := session.ParseStrictExtractionInputDetailed(input)
	if len(parsed.Failures) > 0 {
		if parsed.Count > 0 {
			return errors.New("api review-continuation response mixes selectors with findings or invalid text")
		}
		return errors.New("api review-continuation response contains no selectors; final findings require no continuation")
	}
	prompt, metadata, extractedCount, failed, excluded, err := session.GenerateReviewContinuation(parsed.Commands)
	if err != nil {
		return err
	}
	usable := 0
	for _, item := range metadata {
		if !item.Dropped {
			usable++
		}
	}
	if usable == 0 {
		return errors.New("api review-continuation payload limit leaves no usable supplemental context")
	}
	printExtractionWarnings(stderr, extractedCount, failed, excluded)
	printExtractionMetadata(stderr, metadata)
	if _, err := fmt.Fprint(stdout, prompt); err != nil {
		return fmt.Errorf("writing review continuation: %w", err)
	}
	return nil
}

const maxReviewAPIInputBytes = 1024 * 1024

func runReviewContextAPI(root, guidance string, api APIOptions, stdout io.Writer) error {
	return runReviewContextAPIWithBuilder(root, guidance, api, stdout, reviewtask.BuildInitialReviewPayload)
}

func runReviewContextAPIWithBuilder(root, guidance string, api APIOptions, stdout io.Writer, buildReview reviewPayloadBuilder) error {
	mode := reviewtask.ModeDefault
	switch api.ReviewMode {
	case "", "default":
	case "staged":
		mode = reviewtask.ModeStaged
	case "branch":
		mode = reviewtask.ModeBranch
	case "commit":
		mode = reviewtask.ModeCommit
	}
	paths, err := readReviewPaths(api.PathsFilePath)
	if err != nil {
		return err
	}
	result, err := buildReview(root, reviewtask.Options{
		Mode: mode, Ref: api.ReviewRef, ExtraFocus: guidance, SelectedPaths: paths,
		MaxPayloadBytes: api.MaxReviewPayloadBytes, MaxFileBytes: api.MaxReviewFileBytes,
		IncludeTopology:      api.IncludeReviewTopology,
		MaxFilesPerDirectory: api.MaxFilesPerDirectory,
	})
	if err != nil {
		return fmt.Errorf("preparing review context: %w", sanitizeReviewPreparationError(root, err))
	}
	switch result.Failure {
	case reviewtask.PayloadFailureNoChanges:
		return errors.New("api review-context found no reviewable changes")
	case reviewtask.PayloadFailureMandatoryOverflow:
		return errors.New("api review-context mandatory content exceeds the payload byte limit")
	case reviewtask.PayloadFailureTopologyScan:
		return errors.New("api review-context could not scan project topology")
	}
	if _, err := fmt.Fprint(stdout, result.Payload.Prompt); err != nil {
		return fmt.Errorf("writing review context: %w", err)
	}
	return nil
}

type sanitizedReviewPreparationError struct {
	message string
	err     error
}

func (e sanitizedReviewPreparationError) Error() string { return e.message }
func (e sanitizedReviewPreparationError) Unwrap() error { return e.err }

func sanitizeReviewPreparationError(root string, err error) error {
	message := err.Error()
	candidates := []string{root}
	if root != "" {
		if cleaned := filepath.Clean(root); cleaned != root {
			candidates = append(candidates, cleaned)
		}
		if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil && resolved != root {
			candidates = append(candidates, resolved)
		}
	}
	for _, candidate := range candidates {
		if candidate != "" {
			message = replacePathFold(message, candidate, "<repository>")
		}
	}
	return sanitizedReviewPreparationError{message: message, err: err}
}

func replacePathFold(message, path, replacement string) string {
	for {
		index := strings.Index(strings.ToLower(message), strings.ToLower(path))
		if index < 0 {
			return message
		}
		message = message[:index] + replacement + message[index+len(path):]
	}
}

func readReviewPaths(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("reading api paths file: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxReviewAPIInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading api paths file: %w", err)
	}
	if len(data) > maxReviewAPIInputBytes {
		return nil, fmt.Errorf("reading api paths file: exceeds %d bytes", maxReviewAPIInputBytes)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("reading api paths file: invalid UTF-8")
	}
	var paths []string
	if err := json.Unmarshal(data, &paths); err != nil {
		return nil, fmt.Errorf("reading api paths file: expected a JSON array of strings: %w", err)
	}
	if len(paths) == 0 {
		return nil, errors.New("reading api paths file: JSON array must contain at least one path")
	}
	return paths, nil
}

func normalizeAPIRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("api operation requires --root <project>")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("normalizing api root: %w", err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return "", fmt.Errorf("validating api root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validating api root: not a directory: %s", root)
	}
	return absRoot, nil
}

func readAPIInput(path string) (string, error) {
	return readAPIFile(path, "api input file")
}

func readAPIFile(path, label string) (string, error) {
	if path == "" {
		return "", nil
	}
	input, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	if !utf8.Valid(input) {
		return "", fmt.Errorf("reading %s: invalid UTF-8: %s", label, path)
	}
	return string(input), nil
}

func readAPIFileLimited(path, label string, maxBytes int) (string, error) {
	if path == "" {
		return "", nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", label, err)
	}
	if len(input) > maxBytes {
		return "", fmt.Errorf("reading %s: exceeds %d bytes", label, maxBytes)
	}
	if !utf8.Valid(input) {
		return "", fmt.Errorf("reading %s: invalid UTF-8: %s", label, path)
	}
	return string(input), nil
}
