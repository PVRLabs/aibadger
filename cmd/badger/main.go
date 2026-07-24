package main

import (
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/reviewtask"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

type appConfig struct {
	headless         bool
	showBadge        bool
	focus            protocol.Focus
	reviewMode       reviewtask.Mode
	reviewRef        string
	reviewExtraFocus string
	showHelp         bool
	showVersion      bool
	cpuprofile       string // Profile mode: CPU profile output path
	parseErr         error
}

var devOnlyFlags = []string{
	"headless",
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "api" {
		if err := runAPI(os.Args[2:], os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	cfg := loadConfig(os.Args[1:])
	if cfg.showHelp {
		printUsage(cfg)
		return
	}
	if cfg.showVersion {
		printVersion(os.Stdout)
		return
	}

	if releaseBuild {
		devFlags := usedDevOnlyFlags(os.Args[1:])
		if len(devFlags) > 0 {
			fmt.Fprintf(os.Stderr, "Error: the following flags are only available in development builds: %s\n", strings.Join(devFlags, ", "))
			fmt.Fprintf(os.Stderr, "Use the development build (default `go build`) or remove these flags.\n")
			os.Exit(1)
		}
	}

	if cfg.parseErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", cfg.parseErr)
		os.Exit(1)
	}
	if err := validateHeadlessMode(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
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
	badgerCfg.BuildInfo = buildInfoLine()
	badgerCfg.Focus = protocol.NormalizeFocus(cfg.focus)
	if cfg.showBadge {
		if err := applyBadgeStartup(&cfg, &badgerCfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.focus == protocol.FocusReview && cfg.headless {
		if err := runHeadlessReview(badgerCfg, cfg, os.Stdout, os.Stderr); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if cfg.focus == protocol.FocusReview {
		if err := applyReviewStartup(&badgerCfg, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if cfg.focus == protocol.FocusDesign {
		applyDesignStartup(&badgerCfg, cfg)
	}
	if cfg.focus == protocol.FocusFollowup {
		applyFollowupStartup(&badgerCfg, cfg)
	}
	if err := badger.Run(badgerCfg); err != nil {
		fmt.Printf("TUI error: %v\n", err)
	}
}

type apiConfig struct {
	operation    string
	root         string
	inputPath    string
	goalFilePath string
	focus        protocol.Focus
}

func runAPI(args []string, stdout, stderr io.Writer) error {
	api, err := parseAPIConfig(args)
	if err != nil {
		return err
	}

	cfg := badger.DefaultConfig()
	cfg.Root = api.root
	return badger.RunAPI(cfg, badger.APIOptions{
		Operation:    api.operation,
		InputPath:    api.inputPath,
		GoalFilePath: api.goalFilePath,
		Focus:        api.focus,
		Stdout:       stdout,
		Stderr:       stderr,
	})
}

func parseAPIConfig(args []string) (apiConfig, error) {
	if len(args) == 0 {
		return apiConfig{}, fmt.Errorf("api operation is required")
	}

	cfg := apiConfig{operation: args[0]}
	switch cfg.operation {
	case "topology", "prompt", "extract", "scan", "goal", "extraction", "write-plan":
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
		default:
			return apiConfig{}, fmt.Errorf("unknown api flag: %s", arg)
		}
	}

	if cfg.root == "" {
		return apiConfig{}, fmt.Errorf("api %s requires --root <project>", cfg.operation)
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

func loadConfig(args []string) appConfig {
	cfg := appConfig{}

	args = stripFocusCommand(args, &cfg)
	cfg.parseErr = parseArgs(args, &cfg)
	return cfg
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
		case string(protocol.FocusCode), string(protocol.FocusReview), string(protocol.FocusDesign), string(protocol.FocusFollowup):
			cfg.focus = protocol.Focus(arg)
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
			case arg == "--headless":
				cfg.headless = true
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

	return nil
}

func runHeadlessReview(cfg badger.Config, app appConfig, stdout, _ io.Writer) error {
	reviewTask, err := reviewtask.Build(cfg.Root, reviewtask.Options{
		Mode:       app.reviewMode,
		Ref:        app.reviewRef,
		ExtraFocus: app.reviewExtraFocus,
	})
	if err != nil {
		return err
	}
	goal, err := reviewTask.HeadlessGoal()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, strings.TrimRight(goal, "\n"))
	return err
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

func validateHeadlessMode(cfg appConfig) error {
	if cfg.headless && cfg.focus != protocol.FocusReview {
		return fmt.Errorf("--headless is only valid with badger review")
	}
	return nil
}

func applyDesignStartup(cfg *badger.Config, app appConfig) {
	cfg.SkipOnboarding = true
	cfg.Startup = badger.StartupContext{
		Goal: protocol.DefaultDesignPrompt,
		Status: badger.StartupStatus{
			Text:     "Focus set to Design. Edit the goal before submitting.",
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
	reviewTask, err := reviewtask.Build(cfg.Root, reviewtask.Options{
		Mode:       app.reviewMode,
		Ref:        app.reviewRef,
		ExtraFocus: app.reviewExtraFocus,
	})
	if err != nil {
		return err
	}

	if app.headless {
		goal, err := reviewTask.HeadlessGoal()
		if err != nil {
			return err
		}
		cfg.Startup = badger.StartupContext{Goal: goal}
		return nil
	}

	cfg.SkipOnboarding = true
	cfg.Startup = reviewTask.StartupContext()
	return nil
}

func usedDevOnlyFlags(args []string) []string {
	var used []string
	seen := make(map[string]bool, len(devOnlyFlags))

	for _, arg := range args {
		if arg == "--" {
			break
		}
		for _, flagName := range devOnlyFlags {
			if isFlagArg(arg, flagName) {
				displayName := "--" + flagName
				if !seen[flagName] {
					used = append(used, displayName)
					seen[flagName] = true
				}
				break
			}
		}
	}

	return used
}

func isFlagArg(arg, name string) bool {
	return arg == "-"+name ||
		arg == "--"+name ||
		strings.HasPrefix(arg, "-"+name+"=") ||
		strings.HasPrefix(arg, "--"+name+"=")
}

func printUsage(cfg appConfig) {
	fmt.Printf("%s - local context bridge\n%s\n\nUsage:\n  badger [code|review|design|followup] [--help]\n  badger [code|review|design|followup] [--version]\n  badger badge                        Launch the TUI with /badge preloaded\n  badger review [--staged | --branch <ref> | --commit <sha>] [extra focus text]\n  badger api topology --root <project>\n  badger api prompt --root <project> --focus <code|design> --input <goal-file>\n  badger api extract --root <project> [--focus <code|design>] --input <selector-file> --goal-file <goal-file>\n  badger version\n\nOptions:\n  --help, -h        Print this help and exit.\n  --version         Print version and exit.\n\nThe api commands are non-interactive and write directly usable prompt text to stdout.\nStandard runs start the interactive BYOL workflow for the current directory.\nThe default focus is Code; use badger review, badger design, or badger followup to start in a different focus.\n`badger review` preloads an editable review prompt from the current Git working tree. Default mode includes staged and unstaged tracked changes plus up to 25 relevant Git-untracked paths in a separate section; it never includes untracked file contents, and untracked paths alone are valid review context. `--staged`, `--branch <ref>`, and `--commit <sha>` exclude working-tree untracked files. If no reviewable changes are available or the repo is not git-backed, Badger leaves a manual fallback prompt in the editor.\n", badger.Name, buildInfoLine())

	// Show note about dev flags in release builds
	if releaseBuild {
		fmt.Print(`
Note: This is a release build. The development-only review --headless mode
is not available. Use the development build (default 'go build') for that mode.
`)
		return
	}

	fmt.Print(`
Developer testing flags:
  review --headless
                  Prepare a review goal from Git state and exit.
`)

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
	if !releaseBuild {
		info += " · Dev flags: enabled"
	}
	if profileBuild {
		info += " · Profiling: enabled"
	}
	return info
}
