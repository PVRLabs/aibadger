package diagnose

import (
	"context"
	"os"
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
