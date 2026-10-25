//go:build !windows

package diagnose

import (
	"context"
	"os/exec"
)

func probeCommand(ctx context.Context, path string, args []string) (*exec.Cmd, error) {
	return exec.CommandContext(ctx, path, args...), nil
}
