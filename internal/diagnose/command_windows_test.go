package diagnose

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBatchVersionProbe(t *testing.T) {
	// Spaces, Unicode and shell metacharacters must stay literal.
	dir := filepath.Join(t.TempDir(), "tools & (Unicode é) !literal!")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{".cmd", ".bat", ".CMD"} {
		path := filepath.Join(dir, "version"+ext)
		script := "@echo off\r\nif not \"%~1\"==\"--version\" exit /b 2\r\nif not \"%npm_config_update_notifier%\"==\"false\" exit /b 3\r\necho 11.6.0\r\n"
		if err := os.WriteFile(path, []byte(script), 0600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		output, err := (systemRunner{}).Run(ctx, path, []string{"--version"}, append(os.Environ(), "npm_config_update_notifier=false"))
		cancel()
		if err != nil || strings.TrimSpace(output) != "11.6.0" {
			t.Fatalf("%s: output=%q error=%v", ext, output, err)
		}
	}
}

func TestBatchProbeRejectsExpansionAndUnexpectedArguments(t *testing.T) {
	for _, path := range []string{`C:\tools\%NAME%\npm.cmd`, "C:\\tools\\bad\nname.cmd", `C:\tools\bad"name.cmd`, `relative\npm.cmd`} {
		if _, err := probeCommand(context.Background(), path, []string{"--version"}); err == nil {
			t.Fatalf("accepted unsafe path %q", path)
		}
	}
	for _, args := range [][]string{nil, {"--version", "&", "echo"}, {"-m", "pip", "--version"}} {
		if _, err := probeCommand(context.Background(), `C:\tools\npm.cmd`, args); err == nil {
			t.Fatalf("accepted arguments %q", args)
		}
	}
}

func TestExecutableProbeDoesNotUseShell(t *testing.T) {
	cmd, err := probeCommand(context.Background(), `C:\tools\node.exe`, []string{"--version"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != `C:\tools\node.exe` || cmd.SysProcAttr != nil {
		t.Fatal("exe probe was routed through shell")
	}
}

// TestInstalledNPMVersionProbe is opt-in because it executes an installed tool.
// Match the smoke fixture, environment override, deadline and parser exactly.
// Report only classifications: errors and raw output may contain private paths.
func TestInstalledNPMVersionProbe(t *testing.T) {
	if os.Getenv("BADGER_WINDOWS_INSTALLED_TOOLS") != "1" {
		t.Skip("set BADGER_WINDOWS_INSTALLED_TOOLS=1 to check installed npm")
	}
	path, err := exec.LookPath("npm")
	if err != nil {
		t.Skip("npm not installed")
	}
	dir := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal("cannot create disposable npm probe directory")
	}
	t.Chdir(dir)
	c := collector{opts: defaults(Options{})}
	ctx, cancel := context.WithTimeout(context.Background(), c.probeDeadline(path))
	defer cancel()
	env := mergeEnvironment(os.Environ(), map[string]string{"npm_config_update_notifier": "false"}, "windows")
	output, err := (systemRunner{}).Run(ctx, path, []string{"--version"}, env)
	reason := "success"
	switch {
	case ctx.Err() != nil:
		reason = "deadline exceeded"
	case errors.Is(err, exec.ErrWaitDelay):
		reason = "inherited output pipes stayed open"
	case err != nil:
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			reason = fmt.Sprintf("exit status %d", exit.ExitCode())
		} else {
			reason = "launch or I/O error"
		}
	}
	_, parsed := parseSimpleVersion(output)
	if err == nil && parsed {
		return
	}
	versionLines, warningLines, otherLines := 0, 0, 0
	for _, line := range lines(output) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if _, ok := parseSimpleVersion(line); ok {
			versionLines++
		} else if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "npm warn") {
			warningLines++
		} else {
			otherLines++
		}
	}
	kind := "other"
	switch strings.ToLower(filepath.Ext(path)) {
	case ".cmd":
		kind = "cmd"
	case ".bat":
		kind = "bat"
	case ".exe":
		kind = "exe"
	}
	t.Fatalf("installed npm probe: launcher=%s; execution=%s; version parsed=%t; numeric lines=%d; npm warning lines=%d; other nonempty lines=%d (raw output suppressed)", kind, reason, parsed, versionLines, warningLines, otherLines)
}
