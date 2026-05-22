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
	focus            protocol.Focus
	stepFlag         string
	inputFlag        string
	truncateTopology bool
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
	"step",
	"input",
	"truncate-topology",
}

func main() {
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
	if hasHeadlessOnlyFlagsWithoutHeadless(cfg) {
		fmt.Fprintln(os.Stderr, "Error: --step, --input, and --truncate-topology require --headless.")
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
	var headlessReviewGoal string
	if cfg.focus == protocol.FocusReview {
		var err error
		headlessReviewGoal, err = applyReviewStartup(&badgerCfg, cfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
	if !cfg.headless {
		if err := badger.Run(badgerCfg); err != nil {
			fmt.Printf("TUI error: %v\n", err)
		}
		return
	}

	if err := badger.RunHeadless(badgerCfg, badger.HeadlessOptions{
		Step:             cfg.stepFlag,
		InputPath:        cfg.inputFlag,
		Goal:             headlessReviewGoal,
		TruncateTopology: cfg.truncateTopology,
	}); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
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
		case string(protocol.FocusCode), string(protocol.FocusReview), string(protocol.FocusDesign):
			cfg.focus = protocol.Focus(arg)
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
			case arg == "--step":
				value, err := nextValue("step")
				if err != nil {
					return err
				}
				cfg.stepFlag = value
			case arg == "--input":
				value, err := nextValue("input")
				if err != nil {
					return err
				}
				cfg.inputFlag = value
			case arg == "--truncate-topology":
				cfg.truncateTopology = true
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

func applyReviewStartup(cfg *badger.Config, app appConfig) (string, error) {
	reviewTask, err := reviewtask.Build(cfg.Root, reviewtask.Options{
		Mode:       app.reviewMode,
		Ref:        app.reviewRef,
		ExtraFocus: app.reviewExtraFocus,
	})
	if err != nil {
		return "", err
	}

	if app.headless {
		return reviewTask.HeadlessGoal()
	}

	cfg.SkipOnboarding = true
	cfg.StartupGoal = reviewTask.StartupPrompt()
	cfg.StartupStatus, cfg.StartupStatusSeverity = reviewTask.StartupStatus()
	return "", nil
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

func hasHeadlessOnlyFlagsWithoutHeadless(cfg appConfig) bool {
	return !cfg.headless && (cfg.stepFlag != "" || cfg.inputFlag != "" || cfg.truncateTopology)
}

func printUsage(cfg appConfig) {
	fmt.Printf("%s - local context bridge\n%s\n\nUsage:\n  badger [code|review|design] [--help]\n  badger [code|review|design] [--version]\n  badger review [--staged | --branch <ref> | --commit <sha>] [extra focus text]\n  badger version\n\nOptions:\n  --help, -h        Print this help and exit.\n  --version         Print version and exit.\n\nStandard runs start the interactive BYOL workflow for the current directory.\nThe default focus is Code; use badger review or badger design to start in a different focus.\n`badger review` preloads an editable review prompt from the current git diff. Use `--staged`, `--branch <ref>`, or `--commit <sha>` to change the diff source. If no diff is available or the repo is not git-backed, Badger leaves a manual fallback prompt in the editor.\n", badger.Name, buildInfoLine())

	// Show note about dev flags in release builds
	if releaseBuild {
		fmt.Printf(`
Note: This is a release build. Development flags (--headless, --step, --input, --truncate-topology)
are not available. Use the development build (default 'go build') or profile build for those features.
`)
		return
	}

	fmt.Printf(`
Developer testing flags:
  --headless        Run the non-interactive automation path.
  --step <name>     With --headless, run one step and exit: %s.
  --input <file>   With --headless, read step input from a file.
  --truncate-topology
                  With --headless, cap Prompt 1: Topology package output.
`, badger.StepNames)

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
