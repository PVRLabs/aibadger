package clipboard

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	nativeclipboard "github.com/atotto/clipboard"
)

const DocsURL = "https://github.com/PVRLabs/aibadger/blob/main/docs/clipboard.md"

var lookPath = exec.LookPath

type command struct {
	name        string
	args        []string
	pipeExample string
}

// Copy copies the given text to the system clipboard.
func Copy(text string) error {
	return copyText(runtime.GOOS, text, nativeclipboard.WriteAll)
}

func copyText(goos, text string, writeUnicode func(string) error) error {
	if goos == "windows" {
		return writeUnicode(text)
	}
	clip, ok := nativeCommand(goos)
	if !ok {
		return fmt.Errorf("clipboard unavailable on platform: %s", goos)
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

// Available reports whether this platform supports clipboard copying.
// It deliberately does not expose the command or its installation path.
func Available() bool {
	return available(runtime.GOOS)
}

func available(goos string) bool {
	if goos == "windows" {
		return true
	}
	_, ok := nativeCommand(goos)
	return ok
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
