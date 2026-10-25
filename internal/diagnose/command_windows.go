package diagnose

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func probeCommand(ctx context.Context, path string, args []string) (*exec.Cmd, error) {
	ext := filepath.Ext(path)
	if !strings.EqualFold(ext, ".cmd") && !strings.EqualFold(ext, ".bat") {
		return exec.CommandContext(ctx, path, args...), nil
	}
	// Only the fixed version probe may use cmd.exe. Do not accept arbitrary
	// arguments or paths containing expansion syntax, quotes, or newlines.
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "%\"\r\n\x00") || len(args) != 1 || args[0] != "--version" {
		return nil, errors.New("unsupported batch version probe")
	}
	shell, err := exec.LookPath("cmd.exe")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, shell)
	// A shell child may inherit the output pipes; bound the wait after cancellation.
	cmd.WaitDelay = 100 * time.Millisecond
	// cmd.exe does not use CommandLineToArgvW quoting. /s strips the outer
	// pair; the inner pair protects spaces and shell metacharacters in paths.
	// Disable AutoRun and delayed expansion (including ! in directory names).
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + shell + `" /d /v:off /s /c ""` + path + `" --version"`}
	return cmd, nil
}
