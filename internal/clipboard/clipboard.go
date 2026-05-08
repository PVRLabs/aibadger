package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var lookPath = exec.LookPath

type command struct {
	name        string
	args        []string
	pipeExample string
}

// Copy copies the given text to the system clipboard.
func Copy(text string) error {
	clip, ok := nativeCommand(runtime.GOOS)
	if !ok {
		return fmt.Errorf("clipboard unavailable on platform: %s", runtime.GOOS)
	}

	cmd := exec.Command(clip.name, clip.args...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

// PipeCommand returns the shell command that reads stdin into the native
// clipboard, when this platform has a supported clipboard tool available.
func PipeCommand() (string, bool) {
	return pipeCommand(runtime.GOOS)
}

func pipeCommand(goos string) (string, bool) {
	clip, ok := nativeCommand(goos)
	if !ok {
		return "", false
	}
	return clip.pipeExample, true
}

func nativeCommand(goos string) (command, bool) {
	switch goos {
	case "darwin":
		return commandIfAvailable("pbcopy", nil, "pbcopy")
	case "linux":
		return commandIfAvailable("xclip", []string{"-selection", "clipboard"}, "xclip -selection clipboard")
	case "windows":
		return commandIfAvailable("clip", nil, "clip")
	default:
		return command{}, false
	}
}

func commandIfAvailable(name string, args []string, pipeExample string) (command, bool) {
	if _, err := lookPath(name); err != nil {
		return command{}, false
	}
	return command{name: name, args: append([]string(nil), args...), pipeExample: pipeExample}, true
}
