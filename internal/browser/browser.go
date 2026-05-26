package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

const AIBadgerRepoURL = "https://github.com/PVRLabs/aibadger"

// Open launches the given URL in the system browser.
func Open(url string) error {
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
