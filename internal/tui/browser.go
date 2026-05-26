package tui

import (
	"fmt"
	"os/exec"
	"runtime"
)

const badgeRepoURL = "https://github.com/PVRLabs/aibadger"

func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Run()
	case "linux":
		return exec.Command("xdg-open", url).Run()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Run()
	default:
		return fmt.Errorf("browser open not supported on %s", runtime.GOOS)
	}
}
