//go:build !windows

package tui

import "os/exec"

func runPromptFileReveal(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}
