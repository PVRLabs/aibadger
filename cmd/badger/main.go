package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/pprof"
	"strings"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/pkg/badger"
)

type appConfig struct {
	headless         bool
	focus            protocol.Focus
	stepFlag         string
	inputFlag        string
	truncateTopology bool
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
	if !cfg.headless {
		if err := badger.Run(badgerCfg); err != nil {
			fmt.Printf("TUI error: %v\n", err)
		}
		return
	}

	if err := badger.RunHeadless(badgerCfg, badger.HeadlessOptions{
		Step:             cfg.stepFlag,
		InputPath:        cfg.inputFlag,
		TruncateTopology: cfg.truncateTopology,
	}); err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}

func loadConfig(args []string) appConfig {
	cfg := appConfig{}

	args = stripFocusCommand(args, &cfg)

	// First pass: check for help/version flags
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			cfg.showHelp = true
			return cfg
		}
		if arg == "--version" {
			cfg.showVersion = true
			return cfg
		}
	}
	if len(args) > 0 && args[0] == "version" {
		cfg.showVersion = true
		return cfg
	}

	// Parse all flags unconditionally
	fs := flag.NewFlagSet("badger", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	// Dev-only flags (always defined, but validated later)
	fs.BoolVar(&cfg.headless, "headless", false, "Dev only: run the non-interactive automation path")
	fs.StringVar(&cfg.stepFlag, "step", "", "Dev only: specific step to run ("+badger.StepNames+")")
	fs.StringVar(&cfg.inputFlag, "input", "", "Dev only: input file for the step")
	fs.BoolVar(&cfg.truncateTopology, "truncate-topology", false, "Dev only: truncate Prompt 1: Topology in headless mode")

	// Profile mode: add --cpuprofile flag
	if profileBuild {
		fs.StringVar(&cfg.cpuprofile, "cpuprofile", "", "Profile only: write CPU profile to file")
	}

	cfg.parseErr = fs.Parse(args)

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
	fmt.Printf(`%s - local context bridge
%s

Usage:
  badger [code|review|design] [--help]
  badger [code|review|design] [--version]
  badger version

Options:
  --help, -h        Print this help and exit.
  --version         Print version and exit.

Standard runs start the interactive BYOL workflow for the current directory.
The default focus is Code; use badger review or badger design to start in a different focus.
`, badger.Name, buildInfoLine())

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
