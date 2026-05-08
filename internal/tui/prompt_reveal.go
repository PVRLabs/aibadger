package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
)

type promptFileRevealCommand struct {
	name string
	args []string
}

var (
	promptFileRevealGOOS     = runtime.GOOS
	promptFileRevealLookPath = exec.LookPath
	promptFileRevealRun      = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
)

func promptFileRevealAvailable(path string) bool {
	_, ok := promptFileRevealCommandFor(path)
	return ok
}

func revealPromptFile(path string) error {
	cmd, ok := promptFileRevealCommandFor(path)
	if !ok {
		return fmt.Errorf("unsupported platform or missing file manager opener")
	}
	return promptFileRevealRun(cmd.name, cmd.args...)
}

func promptFileRevealCommandFor(path string) (promptFileRevealCommand, bool) {
	switch promptFileRevealGOOS {
	case "darwin":
		if _, err := promptFileRevealLookPath("open"); err != nil {
			return promptFileRevealCommand{}, false
		}
		return promptFileRevealCommand{name: "open", args: []string{"-R", path}}, true
	case "windows":
		if _, err := promptFileRevealLookPath("explorer"); err != nil {
			return promptFileRevealCommand{}, false
		}
		return promptFileRevealCommand{name: "explorer", args: []string{"/select," + path}}, true
	case "linux":
		if _, err := promptFileRevealLookPath("xdg-open"); err != nil {
			return promptFileRevealCommand{}, false
		}
		return promptFileRevealCommand{name: "xdg-open", args: []string{filepath.Dir(path)}}, true
	default:
		return promptFileRevealCommand{}, false
	}
}
