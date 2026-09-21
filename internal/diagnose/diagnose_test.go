package diagnose

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeResponse struct {
	output string
	err    error
}

type fakeCall struct {
	name string
	args []string
	env  []string
}

type fakeRunner struct {
	paths     map[string]string
	responses map[string]fakeResponse
	lookups   []string
	calls     []fakeCall
}

func (f *fakeRunner) LookPath(name string) (string, error) {
	f.lookups = append(f.lookups, name)
	path, ok := f.paths[name]
	if !ok {
		return "", errors.New("not found")
	}
	return path, nil
}

func TestRunUsesOnlyFixedToolLookupsWhenNothingIsInstalled(t *testing.T) {
	runner := &fakeRunner{paths: map[string]string{}}
	Run(&bytes.Buffer{}, Options{Version: "v1", GOOS: "linux", GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: []string{}})
	want := []string{"git", "go", "java", "mvn", "gradle", "node", "nodejs", "npm", "python3", "python", "pip3", "pip", "dotnet", "cmake", "clang", "gcc", "cc"}
	if !reflect.DeepEqual(runner.lookups, want) {
		t.Fatalf("lookups = %v, want fixed probes %v", runner.lookups, want)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("commands ran without successful executable lookup: %#v", runner.calls)
	}
}

func (f *fakeRunner) Run(_ context.Context, path string, args []string, env []string) (string, error) {
	name := filepath.Base(path)
	f.calls = append(f.calls, fakeCall{name: name, args: append([]string(nil), args...), env: append([]string(nil), env...)})
	response := f.responses[name+" "+strings.Join(args, " ")]
	return response.output, response.err
}

func TestRunRendersFixedOrderedNormalizedReport(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]string{
			"git": "/tools/git", "go": "/tools/go", "java": "/tools/java",
			"mvn": "/tools/mvn", "gradle": "/tools/gradle", "node": "/tools/node",
			"npm": "/tools/npm", "python3": "/tools/python3", "dotnet": "/tools/dotnet",
			"cmake": "/tools/cmake", "clang": "/tools/clang",
		},
		responses: map[string]fakeResponse{
			"git --version":            {output: "git version 2.51.0\n"},
			"go version":               {output: "go version go1.27.1 darwin/arm64\n"},
			"java -version":            {output: "openjdk version \"25.0.2\" 2026-01-01\ninstallation /secret/java\n"},
			"mvn --version":            {output: "Apache Maven 3.9.11 (/secret/maven)\n"},
			"gradle --version":         {output: "Gradle 9.1.0\nGradle home: /secret/gradle\n"},
			"node --version":           {output: "v25.9.0\n"},
			"npm --version":            {output: "11.6.0\n"},
			"python3 --version":        {output: "Python 3.14.6\n"},
			"python3 -m pip --version": {output: "pip 25.2 from /Users/alice/secret/pip (python 3.14)\n"},
			"dotnet --version":         {output: "10.0.100\n"},
			"cmake --version":          {output: "cmake version 4.1.0\n"},
			"clang --version":          {output: "Apple clang version 17.0.0 (clang-1700)\nTarget: arm64-apple-darwin\n"},
		},
	}
	var out bytes.Buffer
	Run(&out, Options{
		Version: "v0.6.1-dev", GOOS: "darwin", GOARCH: "arm64",
		ClipboardAvailable: func() bool { return true }, Runner: runner,
		Environment: []string{"PATH=/tools", "USER=alice"},
	})
	want := `AI Badger Diagnose

Environment
  Badger: 0.6.1-dev
  Platform: macOS arm64
  Git: 2.51.0
  Clipboard command: available

Development tools
  Go: 1.27.1
  Java: 25.0.2
  Maven: 3.9.11
  Gradle: 9.1.0
  Node.js: 25.9.0
  npm: 11.6.0
  Python: 3.14.6
  pip: 25.2
  .NET: 10.0.100
  CMake: 4.1.0
  C/C++ compiler: Clang 17.0.0
`
	if out.String() != want {
		t.Fatalf("Run() output:\n%s\nwant:\n%s", out.String(), want)
	}
	for _, private := range []string{"alice", "/secret", "PATH=", "USER="} {
		if strings.Contains(out.String(), private) {
			t.Fatalf("Run() output contains private value %q:\n%s", private, out.String())
		}
	}
}

func TestRunHandlesMissingFailingAndMalformedProbesIndependently(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]string{"git": "/tools/git", "go": "/tools/go", "java": "/tools/java"},
		responses: map[string]fakeResponse{
			"git --version": {err: errors.New("failed")},
			"go version":    {output: "unexpected"},
			"java -version": {output: "openjdk version \"21.0.5\""},
		},
	}
	var out bytes.Buffer
	Run(&out, Options{Version: "v1.0.0", GOOS: "linux", GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: []string{}})
	for _, want := range []string{"Git: unavailable", "Go: unavailable", "Java: 21.0.5", "Maven: not found", "Clipboard command: unavailable"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
}

func TestPipUsesSelectedPythonThenFallsBack(t *testing.T) {
	runner := &fakeRunner{
		paths: map[string]string{"python3": "/tools/python3", "pip3": "/tools/pip3"},
		responses: map[string]fakeResponse{
			"python3 --version":        {output: "Python 3.12.9"},
			"python3 -m pip --version": {err: errors.New("module missing")},
			"pip3 --version":           {output: "pip 24.3 from /private/pip"},
		},
	}
	var out bytes.Buffer
	Run(&out, Options{Version: "v1", GOOS: "linux", GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: []string{}})
	if !strings.Contains(out.String(), "Python: 3.12.9\n  pip: 24.3") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
	var got []string
	for _, call := range runner.calls {
		if call.name == "python3" || call.name == "pip3" {
			got = append(got, call.name+" "+strings.Join(call.args, " "))
		}
	}
	want := []string{"python3 --version", "python3 -m pip --version", "pip3 --version"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
}

func TestProbeEnvironmentOverridesAreInheritedAndPrivate(t *testing.T) {
	runner := &fakeRunner{
		paths:     map[string]string{"npm": "/tools/npm", "dotnet": "/tools/dotnet"},
		responses: map[string]fakeResponse{"npm --version": {output: "11.0.0"}, "dotnet --version": {output: "10.0.100"}},
	}
	base := []string{
		"PATH=/tools", "SECRET=do-not-print", "NPM_CONFIG_UPDATE_NOTIFIER=true",
		"dotnet_cli_telemetry_optout=0", "DOTNET_NOLOGO=0",
		"DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE=0",
	}
	before := append([]string(nil), base...)
	var out bytes.Buffer
	Run(&out, Options{Version: "v1", GOOS: "windows", GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: base})
	if !reflect.DeepEqual(base, before) {
		t.Fatalf("parent environment mutated: %v", base)
	}
	for _, call := range runner.calls {
		env := strings.Join(call.env, "\n")
		if !strings.Contains(env, "PATH=/tools") || !strings.Contains(env, "SECRET=do-not-print") {
			t.Fatalf("%s environment did not inherit base: %v", call.name, call.env)
		}
		switch call.name {
		case "npm":
			if strings.Contains(strings.ToLower(env), "npm_config_update_notifier=true") || !strings.Contains(env, "npm_config_update_notifier=false") {
				t.Fatalf("npm environment = %v", call.env)
			}
		case "dotnet":
			if !reflect.DeepEqual(call.args, []string{"--version"}) {
				t.Fatalf("dotnet args = %v, want --version", call.args)
			}
			for _, want := range []string{"DOTNET_CLI_TELEMETRY_OPTOUT=1", "DOTNET_NOLOGO=1", "DOTNET_CLI_WORKLOAD_UPDATE_NOTIFY_DISABLE=1"} {
				if !strings.Contains(env, want) {
					t.Fatalf("dotnet environment missing %q: %v", want, call.env)
				}
			}
			if strings.Contains(strings.ToLower(env), "dotnet_cli_telemetry_optout=0") {
				t.Fatalf("dotnet environment retained conflicting key: %v", call.env)
			}
		}
	}
	if strings.Contains(out.String(), "do-not-print") {
		t.Fatalf("output exposed environment value:\n%s", out.String())
	}
}

type timeoutRunner struct{ fakeRunner }

func (r *timeoutRunner) Run(ctx context.Context, _ string, _ []string, _ []string) (string, error) {
	<-ctx.Done()
	return "private late output", ctx.Err()
}

func TestProbeTimeoutIsUnavailableAndDoesNotLeakOutput(t *testing.T) {
	runner := &timeoutRunner{fakeRunner: fakeRunner{paths: map[string]string{"git": "/tools/git"}}}
	var out bytes.Buffer
	Run(&out, Options{Version: "v1", GOOS: "linux", GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: []string{}, Timeout: time.Millisecond})
	if !strings.Contains(out.String(), "Git: unavailable") || strings.Contains(out.String(), "private late output") {
		t.Fatalf("unexpected output:\n%s", out.String())
	}
}

func TestCompilerCandidatesArePlatformAware(t *testing.T) {
	tests := []struct {
		goos  string
		paths map[string]string
		resp  map[string]fakeResponse
		want  string
		call  string
	}{
		{goos: "linux", paths: map[string]string{"gcc": "/tools/gcc"}, resp: map[string]fakeResponse{"gcc --version": {output: "gcc (GCC) 14.2.0"}}, want: "GCC 14.2.0", call: "gcc"},
		{goos: "windows", paths: map[string]string{"cl.exe": "/tools/cl.exe"}, resp: map[string]fakeResponse{"cl.exe ": {output: "Microsoft (R) C/C++ Optimizing Compiler Version 19.44.35207", err: errors.New("exit status 2")}}, want: "MSVC 19.44.35207", call: "cl.exe"},
		{goos: "windows-mingw", paths: map[string]string{"gcc": "/tools/gcc"}, resp: map[string]fakeResponse{"gcc --version": {output: "gcc.exe (Rev3, Built by MSYS2 project) 15.2.0"}}, want: "GCC 15.2.0", call: "gcc"},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			runner := &fakeRunner{paths: test.paths, responses: test.resp}
			goos := test.goos
			if goos == "windows-mingw" {
				goos = "windows"
			}
			value := (collector{opts: defaults(Options{Version: "v1", GOOS: goos, GOARCH: "amd64", ClipboardAvailable: func() bool { return false }, Runner: runner, Environment: []string{}})}).compiler()
			if value != test.want {
				t.Fatalf("compiler() = %q, want %q", value, test.want)
			}
			if len(runner.calls) != 1 || runner.calls[0].name != test.call {
				t.Fatalf("calls = %#v, want %s", runner.calls, test.call)
			}
		})
	}
}

func TestPrintHelpDoesNotRunProbes(t *testing.T) {
	var out bytes.Buffer
	PrintHelp(&out)
	for _, want := range []string{"badger diagnose", "does not analyze project files", "network requests"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("help missing %q:\n%s", want, out.String())
		}
	}
}
