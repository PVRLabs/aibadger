package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strconv"
	"strings"

	"github.com/PVRLabs/aibadger/internal/handoff"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/internal/startup"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

type appConfig struct {
	showBadge        bool
	focus            protocol.Focus
	focusExplicit    bool
	reviewMode       reviewtask.Mode
	reviewRef        string
	reviewExtraFocus string
	startupGoal      string
	literalStartup   bool
	continueCommand  bool
	handoffContinue  bool
	showHelp         bool
	showVersion      bool
	cpuprofile       string // Profile mode: CPU profile output path
	parseErr         error
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "api" {
		if err := runAPI(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "skills" {
		if err := runSkillsCommand(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			os.Exit(1)
		}
		return
	}

	cfg := loadConfig(os.Args[1:])
	if cfg.showHelp {
		printUsage()
		return
	}
	if cfg.showVersion {
		printVersion(os.Stdout)
		return
	}

	if cfg.parseErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cfg.parseErr)
		os.Exit(1)
	}
	var invocationRoot string
	if cfg.continueCommand {
		var err error
		invocationRoot, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: resolving invocation directory: %v\n", err)
			os.Exit(1)
		}
		content, err := handoff.Consume(invocationRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := applyContinueContent(&cfg, content); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	// Profile mode: start CPU profiling if --cpuprofile flag provided
	var cpuprofile *os.File
	if profileBuild && cfg.cpuprofile != "" {
		var err error
		cpuprofile, err = os.Create(cfg.cpuprofile)
		if err != nil {
			fmt.Printf("Error creating CPU profile: %v\n", err)
			os.Exit(1)
		}
		if err := pprof.StartCPUProfile(cpuprofile); err != nil {
			fmt.Printf("Error starting CPU profile: %v\n", err)
			os.Exit(1)
		}
		defer cpuprofile.Close()
		defer pprof.StopCPUProfile()
	}

	badgerCfg := badger.DefaultConfig()
	if invocationRoot != "" {
		badgerCfg.Root = invocationRoot
	}
	badgerCfg.BuildInfo = buildInfoLine()
	applyInteractiveFocus(&badgerCfg, &cfg)
	if cfg.showBadge {
		if err := applyBadgeStartup(&badgerCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.focus == protocol.FocusReview {
		if err := applyReviewStartup(&badgerCfg, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.focus == protocol.FocusFollowup {
		applyFollowupStartup(&badgerCfg, cfg)
	}
	if cfg.handoffContinue {
		applyHandoffStartup(&badgerCfg, cfg.startupGoal)
	}
	if err := badger.Run(badgerCfg); err != nil {
		fmt.Printf("TUI error: %v\n", err)
	}
}

type apiConfig struct {
	operation             string
	root                  string
	inputPath             string
	goalFilePath          string
	focus                 protocol.Focus
	reviewMode            string
	reviewRef             string
	pathsFilePath         string
	maxReviewPayloadBytes int
	maxReviewFileBytes    int
	includeReviewTopology bool
}

func runAPI(args []string, stdout, stderr io.Writer) error {
	if operation, ok := apiHelpRequest(args); ok {
		printAPIHelp(stdout, operation)
		return nil
	}

	api, err := parseAPIConfig(args)
	if err != nil {
		return err
	}

	cfg := badger.DefaultConfig()
	cfg.Root = api.root
	return badger.RunAPI(cfg, badger.APIOptions{
		Operation:             api.operation,
		InputPath:             api.inputPath,
		GoalFilePath:          api.goalFilePath,
		Focus:                 api.focus,
		ReviewMode:            api.reviewMode,
		ReviewRef:             api.reviewRef,
		PathsFilePath:         api.pathsFilePath,
		MaxReviewPayloadBytes: api.maxReviewPayloadBytes,
		MaxReviewFileBytes:    api.maxReviewFileBytes,
		IncludeReviewTopology: api.includeReviewTopology,
		Stdout:                stdout,
		Stderr:                stderr,
	})
}

func parseAPIConfig(args []string) (apiConfig, error) {
	if len(args) == 0 {
		return apiConfig{}, fmt.Errorf("api operation is required")
	}

	cfg := apiConfig{operation: args[0]}
	switch cfg.operation {
	case "topology", "prompt", "extract", "review-context", "review-continuation", "scan", "goal", "extraction", "write-plan":
	default:
		return apiConfig{}, fmt.Errorf("unknown api operation: %s", cfg.operation)
	}

	for i := 1; i < len(args); i++ {
		arg := args[i]
		nextValue := func(flagName string) (string, error) {
			if value, ok := flagValue(arg, flagName); ok {
				return value, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("flag needs an argument: %s", arg)
			}
			i++
			return args[i], nil
		}

		switch {
		case arg == "--root" || strings.HasPrefix(arg, "--root="):
			value, err := nextValue("root")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.root = value
		case arg == "--input" || strings.HasPrefix(arg, "--input="):
			value, err := nextValue("input")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.inputPath = value
		case arg == "--focus" || strings.HasPrefix(arg, "--focus="):
			value, err := nextValue("focus")
			if err != nil {
				return apiConfig{}, err
			}
			switch protocol.Focus(value) {
			case protocol.FocusCode, protocol.FocusDesign:
				cfg.focus = protocol.Focus(value)
			default:
				return apiConfig{}, fmt.Errorf("api %s supports --focus <code|design>; got %q", cfg.operation, value)
			}
		case arg == "--goal-file" || strings.HasPrefix(arg, "--goal-file="):
			value, err := nextValue("goal-file")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.goalFilePath = value
		case arg == "--mode" || strings.HasPrefix(arg, "--mode="):
			value, err := nextValue("mode")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.reviewMode = value
		case arg == "--ref" || strings.HasPrefix(arg, "--ref="):
			value, err := nextValue("ref")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.reviewRef = value
		case arg == "--paths-file" || strings.HasPrefix(arg, "--paths-file="):
			value, err := nextValue("paths-file")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.pathsFilePath = value
		case arg == "--max-payload-bytes" || strings.HasPrefix(arg, "--max-payload-bytes="):
			value, err := nextValue("max-payload-bytes")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.maxReviewPayloadBytes, err = parsePositiveAPIBytes("max-payload-bytes", value)
			if err != nil {
				return apiConfig{}, err
			}
		case arg == "--max-file-bytes" || strings.HasPrefix(arg, "--max-file-bytes="):
			value, err := nextValue("max-file-bytes")
			if err != nil {
				return apiConfig{}, err
			}
			cfg.maxReviewFileBytes, err = parsePositiveAPIBytes("max-file-bytes", value)
			if err != nil {
				return apiConfig{}, err
			}
		case arg == "--include-topology":
			cfg.includeReviewTopology = true
		default:
			return apiConfig{}, fmt.Errorf("unknown api flag: %s", arg)
		}
	}

	if cfg.root == "" {
		return apiConfig{}, fmt.Errorf("api %s requires --root <project>", cfg.operation)
	}
	if cfg.operation == "review-context" {
		if cfg.focus != "" || cfg.goalFilePath != "" {
			return apiConfig{}, fmt.Errorf("api review-context does not accept --focus or --goal-file")
		}
		if cfg.reviewMode == "" {
			cfg.reviewMode = "default"
		}
		switch cfg.reviewMode {
		case "default", "staged":
			if cfg.reviewRef != "" {
				return apiConfig{}, fmt.Errorf("api review-context mode %s does not accept --ref", cfg.reviewMode)
			}
		case "branch", "commit":
			if cfg.reviewRef == "" {
				return apiConfig{}, fmt.Errorf("api review-context mode %s requires --ref <revision>", cfg.reviewMode)
			}
		default:
			return apiConfig{}, fmt.Errorf("api review-context supports --mode <default|staged|branch|commit>; got %q", cfg.reviewMode)
		}
		if cfg.pathsFilePath != "" && cfg.reviewMode != "default" {
			return apiConfig{}, fmt.Errorf("api review-context mode %s does not accept --paths-file", cfg.reviewMode)
		}
		return cfg, nil
	}
	if cfg.operation == "review-continuation" {
		if cfg.inputPath == "" {
			return apiConfig{}, fmt.Errorf("api review-continuation requires --input <file>")
		}
		if cfg.focus != "" || cfg.goalFilePath != "" || cfg.reviewMode != "" || cfg.reviewRef != "" || cfg.pathsFilePath != "" || cfg.includeReviewTopology {
			return apiConfig{}, fmt.Errorf("api review-continuation accepts only --root, --input, --max-payload-bytes, and --max-file-bytes")
		}
		return cfg, nil
	}
	if cfg.reviewMode != "" || cfg.reviewRef != "" || cfg.pathsFilePath != "" || cfg.maxReviewPayloadBytes != 0 || cfg.maxReviewFileBytes != 0 || cfg.includeReviewTopology {
		return apiConfig{}, fmt.Errorf("api %s does not accept review-context options", cfg.operation)
	}
	if cfg.operation == "scan" || cfg.operation == "topology" {
		if cfg.inputPath != "" {
			return apiConfig{}, fmt.Errorf("api %s does not accept --input", cfg.operation)
		}
		if cfg.focus != "" {
			return apiConfig{}, fmt.Errorf("api %s does not accept --focus", cfg.operation)
		}
		if cfg.goalFilePath != "" {
			return apiConfig{}, fmt.Errorf("api %s does not accept --goal-file", cfg.operation)
		}
		return cfg, nil
	}
	if cfg.inputPath == "" {
		return apiConfig{}, fmt.Errorf("api %s requires --input <file>", cfg.operation)
	}
	if cfg.operation == "prompt" {
		if cfg.goalFilePath != "" {
			return apiConfig{}, fmt.Errorf("api prompt does not accept --goal-file")
		}
		if cfg.focus == "" {
			return apiConfig{}, fmt.Errorf("api prompt requires --focus <code|design>")
		}
		return cfg, nil
	}
	if cfg.operation == "extract" {
		if cfg.goalFilePath == "" {
			return apiConfig{}, fmt.Errorf("api extract requires --goal-file <file>")
		}
		return cfg, nil
	}
	if cfg.focus != "" {
		return apiConfig{}, fmt.Errorf("api %s does not accept --focus", cfg.operation)
	}
	if cfg.goalFilePath != "" {
		return apiConfig{}, fmt.Errorf("api %s does not accept --goal-file", cfg.operation)
	}
	return cfg, nil
}

func parsePositiveAPIBytes(flag, value string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("api --%s requires a positive integer", flag)
	}
	return n, nil
}

func loadConfig(args []string) appConfig {
	cfg := appConfig{}

	args = stripFocusCommand(args, &cfg)
	cfg.parseErr = parseArgs(args, &cfg)
	return cfg
}

func interactiveFocus(focus protocol.Focus) protocol.Focus {
	if focus == "" {
		return protocol.FocusDesign
	}
	return focus
}

func applyContinueContent(cfg *appConfig, content handoff.Content) error {
	cfg.focusExplicit = true
	switch content.Mode {
	case handoff.ModeReview:
		cfg.focus = protocol.FocusReview
		cfg.reviewMode = reviewtask.ModeDefault
		cfg.reviewExtraFocus = content.Body
	case handoff.ModeDesign:
		cfg.focus = protocol.FocusDesign
		cfg.startupGoal = content.Body
		cfg.literalStartup = true
	case handoff.ModeHandoff:
		cfg.focus = protocol.FocusDesign
		cfg.startupGoal = content.Body
		cfg.literalStartup = true
		cfg.handoffContinue = true
	default:
		return fmt.Errorf("unsupported handoff mode %q", content.Mode)
	}
	return nil
}

func applyInteractiveFocus(cfg *badger.Config, app *appConfig) {
	app.focus = interactiveFocus(app.focus)
	cfg.Focus = protocol.NormalizeFocus(app.focus)
	if app.focus == protocol.FocusDesign && app.focusExplicit {
		applyDesignStartup(cfg, *app)
	}
}

func stripFocusCommand(args []string, cfg *appConfig) []string {
	for i, arg := range args {
		if arg == "--" {
			return args
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		switch arg {
		case "continue":
			cfg.continueCommand = true
			return append(append([]string(nil), args[:i]...), args[i+1:]...)
		case string(protocol.FocusCode), string(protocol.FocusReview), string(protocol.FocusDesign), string(protocol.FocusFollowup):
			cfg.focus = protocol.Focus(arg)
			cfg.focusExplicit = true
			return append(append([]string(nil), args[:i]...), args[i+1:]...)
		case "badge":
			cfg.showBadge = true
			return append(append([]string(nil), args[:i]...), args[i+1:]...)
		}
		return args
	}
	return args
}

func parseArgs(args []string, cfg *appConfig) error {
	if cfg.continueCommand {
		if len(args) > 0 {
			return fmt.Errorf("continue command does not accept flags or arguments")
		}
		return nil
	}
	if len(args) > 0 && args[0] == "version" {
		cfg.showVersion = true
		return nil
	}

	var positional []string
	var parsingFlags = true
	var reviewModeSet bool

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if parsingFlags && arg == "--" {
			parsingFlags = false
			continue
		}
		if arg == "--help" || arg == "-h" {
			cfg.showHelp = true
			return nil
		}
		if arg == "--version" {
			cfg.showVersion = true
			return nil
		}
		if parsingFlags && strings.HasPrefix(arg, "-") {
			nextValue := func(flagName string) (string, error) {
				if value, ok := flagValue(arg, flagName); ok {
					return value, nil
				}
				if i+1 >= len(args) {
					return "", fmt.Errorf("flag needs an argument: %s", arg)
				}
				i++
				return args[i], nil
			}

			switch {
			case profileBuild && arg == "--cpuprofile":
				value, err := nextValue("cpuprofile")
				if err != nil {
					return err
				}
				cfg.cpuprofile = value
			case arg == "--staged":
				if reviewModeSet {
					return fmt.Errorf("review flags --staged, --branch, and --commit are mutually exclusive")
				}
				cfg.reviewMode = reviewtask.ModeStaged
				reviewModeSet = true
			case arg == "--branch":
				if reviewModeSet {
					return fmt.Errorf("review flags --staged, --branch, and --commit are mutually exclusive")
				}
				value, err := nextValue("branch")
				if err != nil {
					return err
				}
				cfg.reviewMode = reviewtask.ModeBranch
				cfg.reviewRef = value
				reviewModeSet = true
			case strings.HasPrefix(arg, "--branch="):
				if reviewModeSet {
					return fmt.Errorf("review flags --staged, --branch, and --commit are mutually exclusive")
				}
				cfg.reviewMode = reviewtask.ModeBranch
				cfg.reviewRef = strings.TrimPrefix(arg, "--branch=")
				reviewModeSet = true
			case arg == "--commit":
				if reviewModeSet {
					return fmt.Errorf("review flags --staged, --branch, and --commit are mutually exclusive")
				}
				value, err := nextValue("commit")
				if err != nil {
					return err
				}
				cfg.reviewMode = reviewtask.ModeCommit
				cfg.reviewRef = value
				reviewModeSet = true
			case strings.HasPrefix(arg, "--commit="):
				if reviewModeSet {
					return fmt.Errorf("review flags --staged, --branch, and --commit are mutually exclusive")
				}
				cfg.reviewMode = reviewtask.ModeCommit
				cfg.reviewRef = strings.TrimPrefix(arg, "--commit=")
				reviewModeSet = true
			default:
				return fmt.Errorf("unknown flag: %s", arg)
			}
			continue
		}

		positional = append(positional, arg)
	}

	if cfg.focus == protocol.FocusReview && len(positional) > 0 {
		cfg.reviewExtraFocus = strings.TrimSpace(strings.Join(positional, " "))
	}

	if cfg.focus == protocol.FocusReview {
		if err := validateReviewOptions(cfg.reviewMode, cfg.reviewRef); err != nil {
			return err
		}
	}
	if cfg.showBadge && len(positional) > 0 {
		return fmt.Errorf("badge command does not accept arguments")
	}
	if cfg.focus != protocol.FocusReview && len(positional) > 0 {
		return fmt.Errorf("unknown command: %s", positional[0])
	}

	return nil
}

func flagValue(arg, name string) (string, bool) {
	prefix := "--" + name + "="
	if strings.HasPrefix(arg, prefix) {
		return strings.TrimPrefix(arg, prefix), true
	}
	prefix = "-" + name + "="
	if strings.HasPrefix(arg, prefix) {
		return strings.TrimPrefix(arg, prefix), true
	}
	return "", false
}

func validateReviewOptions(mode reviewtask.Mode, ref string) error {
	switch mode {
	case reviewtask.ModeDefault, reviewtask.ModeStaged:
		if strings.TrimSpace(ref) != "" {
			return fmt.Errorf("review mode %s does not accept a ref", mode)
		}
	case reviewtask.ModeBranch, reviewtask.ModeCommit:
		if strings.TrimSpace(ref) == "" {
			return fmt.Errorf("review mode %s requires a ref", mode)
		}
	default:
		return fmt.Errorf("unknown review mode %d", mode)
	}
	return nil
}

func applyDesignStartup(cfg *badger.Config, app appConfig) {
	cfg.SkipOnboarding = true
	cfg.Startup = badger.StartupContext{
		Goal:        app.startupGoal,
		LiteralGoal: app.literalStartup,
		Status: badger.StartupStatus{
			Text:     "Focus set to Design.",
			Severity: "success",
		},
	}
}

func applyFollowupStartup(cfg *badger.Config, app appConfig) {
	cfg.SkipOnboarding = true
	cfg.Startup = badger.StartupContext{
		Goal: protocol.DefaultFollowupPrompt,
		Status: badger.StartupStatus{
			Text:     "Focus set to Follow-up. Edit the goal before submitting.",
			Severity: "success",
		},
	}
}

func applyReviewStartup(cfg *badger.Config, app appConfig) error {
	return applyReviewStartupWithBuilder(cfg, app, reviewtask.BuildInteractiveContext)
}

type reviewContextBuilder func(string, reviewtask.Options) (startup.Context, error)

func applyHandoffStartup(cfg *badger.Config, body string) {
	applyHandoffStartupWithBuilder(cfg, body, reviewtask.BuildInteractiveContext)
}

func applyHandoffStartupWithBuilder(cfg *badger.Config, body string, buildReview reviewContextBuilder) {
	cfg.SkipOnboarding = true
	cfg.Startup = badger.StartupContext{
		Goal:        body,
		LiteralGoal: true,
		Status: badger.StartupStatus{
			Text:     "Handoff loaded. Edit the goal before submitting.",
			Severity: "success",
		},
	}
	ctx, err := buildReview(cfg.Root, reviewtask.Options{
		Mode:           reviewtask.ModeDefault,
		MaxPromptBytes: cfg.MaxTopologyPromptBytes,
	})
	if err != nil {
		cfg.Startup.Status = badger.StartupStatus{
			Text:     "Handoff loaded, but current Git context could not be prepared. Edit the goal and continue.",
			Severity: "warning",
		}
		return
	}
	cfg.Startup.Attachments = ctx.Attachments
}

func applyReviewStartupWithBuilder(cfg *badger.Config, app appConfig, buildReview reviewContextBuilder) error {
	ctx, err := buildReview(cfg.Root, reviewtask.Options{
		Mode:           app.reviewMode,
		Ref:            app.reviewRef,
		ExtraFocus:     app.reviewExtraFocus,
		MaxPromptBytes: cfg.MaxTopologyPromptBytes,
	})
	if err != nil {
		return err
	}

	cfg.SkipOnboarding = true
	cfg.Startup = ctx
	return nil
}

func printUsage() {
	fmt.Printf(`%s - local context bridge
%s

Usage:
  badger [design|code|review|followup] [options]
  badger continue
  badger badge
  badger skills install
  badger api <command> --help
  badger version

Interactive focuses:
  design      Explore or design changes (default).
  code        Prepare context for implementation work.
  review      Review Git changes with optional supporting context.
  followup    Continue an existing AI conversation.
  continue    Consume .badger-handoff from the invocation directory.
  badge       Launch the TUI with /badge preloaded.

Workspace handoff:
  badger continue reads one explicit, ephemeral session handoff. The file
  supports mode: review, mode: design, or mode: handoff and accepts no flags
  or arguments. See docs/usage.md for the format and mode behavior.

Agent Skills:
  badger skills install installs bundled official skills to ~/.agents/skills/.

Review options:
  badger review [--staged | --branch <ref> | --commit <sha>]
                [extra guidance]

Stable API:
  badger api topology --root <project>
  badger api prompt --root <project> --focus <code|design>
                    --input <goal-file>
  badger api extract --root <project> [--focus <code|design>]
                     --input <selector-file> --goal-file <goal-file>
  badger api review-context --root <repository>
                            [--mode <default|staged|branch|commit>]
  badger api review-continuation --root <repository>
                                 --input <selector-file>

Options:
  -h, --help       Print help and exit.
  --version        Print version and exit.

API commands are non-interactive and write usable prompt text to stdout.
Run badger api --help or add --help to a command for complete options.
`, badger.Name, buildInfoLine())

	// Profile mode: show profiler-specific help
	if profileBuild {
		fmt.Print(`
Profiler flags:
  --cpuprofile <file>  Write CPU profile to file (pprof format).
`)
	}
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "badger %s\n", badger.Version)
}

func buildInfoLine() string {
	build := "release"
	if profileBuild {
		build = "profile"
	} else if !releaseBuild {
		build = "development"
	}

	info := fmt.Sprintf("Version: %s · Build: %s", badger.Version, build)
	if profileBuild {
		info += " · Profiling: enabled"
	}
	return info
}
