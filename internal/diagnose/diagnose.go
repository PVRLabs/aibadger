package diagnose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/version"
)

const (
	probeTimeout = 3 * time.Second
	maxOutput    = 32 * 1024
)

// Options supplies the local values and execution seams used to collect a
// report. Zero-valued fields use production defaults.
type Options struct {
	Version            string
	GOOS               string
	GOARCH             string
	ClipboardAvailable func() bool
	Runner             Runner
	Environment        []string
	Timeout            time.Duration
}

// Runner locates and directly invokes local executables without a shell.
type Runner interface {
	LookPath(name string) (string, error)
	Run(ctx context.Context, path string, args []string, env []string) (string, error)
}

type systemRunner struct{}

func (systemRunner) LookPath(name string) (string, error) { return exec.LookPath(name) }

func (systemRunner) Run(ctx context.Context, path string, args []string, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Env = env
	var output limitedBuffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err := cmd.Run()
	if output.exceeded {
		return "", errors.New("probe output limit exceeded")
	}
	return output.String(), err
}

type limitedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := maxOutput - b.Len()
	if remaining <= 0 {
		b.exceeded = true
		return original, nil
	}
	if len(p) > remaining {
		b.exceeded = true
		p = p[:remaining]
	}
	_, _ = b.Buffer.Write(p)
	return original, nil
}

// Run writes a deterministic, privacy-safe snapshot of the local development
// environment.
func Run(w io.Writer, opts Options) {
	opts = defaults(opts)
	collector := collector{opts: opts}
	results := collector.collect()

	fmt.Fprintln(w, "AI Badger Diagnose")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Environment")
	for _, result := range results[:4] {
		fmt.Fprintf(w, "  %s: %s\n", result.label, result.value)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Development tools")
	for _, result := range results[4:] {
		fmt.Fprintf(w, "  %s: %s\n", result.label, result.value)
	}
}

// PrintHelp writes command-specific help without running any probes.
func PrintHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  badger diagnose

Purpose:
  Print local environment information for troubleshooting and bug reports.

The report is project-independent, designed to be shareable, and does not
perform dependency resolution, updates, builds, tests, or network requests.
`)
}

type result struct{ label, value string }

type collector struct{ opts Options }

func defaults(opts Options) Options {
	if opts.Version == "" {
		opts.Version = version.Version
	}
	if opts.GOOS == "" {
		opts.GOOS = runtime.GOOS
	}
	if opts.GOARCH == "" {
		opts.GOARCH = runtime.GOARCH
	}
	if opts.ClipboardAvailable == nil {
		opts.ClipboardAvailable = clipboard.Available
	}
	if opts.Runner == nil {
		opts.Runner = systemRunner{}
	}
	if opts.Environment == nil {
		opts.Environment = os.Environ()
	}
	if opts.Timeout <= 0 {
		opts.Timeout = probeTimeout
	}
	return opts
}

func (c collector) collect() []result {
	available := "unavailable"
	if c.opts.ClipboardAvailable() {
		available = "available"
	}
	results := []result{
		{"Badger", strings.TrimPrefix(c.opts.Version, "v")},
		{"Platform", platformName(c.opts.GOOS) + " " + c.opts.GOARCH},
		{"Git", c.probe([]string{"git"}, []string{"--version"}, nil, parseGit)},
		{"Clipboard", available},
		{"Go", c.probe([]string{"go"}, []string{"version"}, nil, parseGo)},
		{"Java", c.probe([]string{"java"}, []string{"-version"}, nil, parseJava)},
		{"Maven", c.probe([]string{"mvn"}, []string{"--version"}, nil, prefixedVersion("Apache Maven"))},
		{"Gradle", c.probe([]string{"gradle"}, []string{"--version"}, nil, prefixedVersion("Gradle"))},
		{"Node.js", c.probe([]string{"node", "nodejs"}, []string{"--version"}, nil, parseSimpleVersion)},
		{"npm", c.probe([]string{"npm"}, []string{"--version"}, map[string]string{"npm_config_update_notifier": "false"}, parseSimpleVersion)},
	}
	pythonPath, pythonVersion := c.python()
	results = append(results, result{"Python", pythonVersion})
	results = append(results, result{"pip", c.pip(pythonPath)})
	results = append(results,
		result{".NET", c.probe([]string{"dotnet"}, []string{"--list-sdks"}, map[string]string{
			"DOTNET_CLI_TELEMETRY_OPTOUT":               "1",
			"DOTNET_NOLOGO":                             "1",
			"DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE": "1",
		}, parseDotnetSDKs)},
		result{"CMake", c.probe([]string{"cmake"}, []string{"--version"}, nil, prefixedVersion("cmake version"))},
		result{"C/C++ compiler", c.compiler()},
	)
	return results
}

func (c collector) python() (string, string) {
	return c.probePath([]string{"python3", "python"}, []string{"--version"}, nil, parsePython)
}

func (c collector) pip(pythonPath string) string {
	if pythonPath != "" {
		if value, ok := c.run(pythonPath, []string{"-m", "pip", "--version"}, nil, parsePip, false); ok {
			return value
		}
	}
	return c.probe([]string{"pip3", "pip"}, []string{"--version"}, nil, parsePip)
}

func (c collector) compiler() string {
	if c.opts.GOOS == "windows" {
		if path, err := c.opts.Runner.LookPath("cl.exe"); err == nil {
			if value, ok := c.run(path, nil, nil, parseCompiler, true); ok {
				return value
			}
		}
		return c.probe([]string{"clang-cl", "clang", "gcc", "cc"}, []string{"--version"}, nil, parseCompiler)
	}
	return c.probe([]string{"clang", "gcc", "cc"}, []string{"--version"}, nil, parseCompiler)
}

func (c collector) probe(names, args []string, overrides map[string]string, parse func(string) (string, bool)) string {
	_, value := c.probePath(names, args, overrides, parse)
	return value
}

func (c collector) probePath(names, args []string, overrides map[string]string, parse func(string) (string, bool)) (string, string) {
	found := false
	for _, name := range names {
		path, err := c.opts.Runner.LookPath(name)
		if err != nil {
			continue
		}
		found = true
		if value, ok := c.run(path, args, overrides, parse, false); ok {
			return path, value
		}
	}
	if found {
		return "", "unavailable"
	}
	return "", "not found"
}

func (c collector) run(path string, args []string, overrides map[string]string, parse func(string) (string, bool), acceptNonzero bool) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.Timeout)
	defer cancel()
	output, err := c.opts.Runner.Run(ctx, path, append([]string(nil), args...), mergeEnvironment(c.opts.Environment, overrides, c.opts.GOOS))
	if err != nil && !acceptNonzero {
		return "", false
	}
	return parse(output)
}

func mergeEnvironment(base []string, overrides map[string]string, goos string) []string {
	if len(overrides) == 0 {
		return append([]string(nil), base...)
	}
	equal := func(a, b string) bool {
		if goos == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	merged := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		replaced := false
		for override := range overrides {
			if equal(key, override) {
				replaced = true
				break
			}
		}
		if !replaced {
			merged = append(merged, item)
		}
	}
	keys := make([]string, 0, len(overrides))
	for key := range overrides {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := overrides[key]
		merged = append(merged, key+"="+value)
	}
	return merged
}

func platformName(goos string) string {
	switch goos {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	default:
		return goos
	}
}
