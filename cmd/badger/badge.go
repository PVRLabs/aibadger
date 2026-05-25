package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/PVRLabs/aibadger/internal/brand"
	"github.com/PVRLabs/aibadger/internal/github"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

const badgeRepoURL = "https://github.com/PVRLabs/aibadger"

var fetchStargazersFunc = github.FetchStargazers
var openBrowserFunc = openBrowser
var terminalInteractiveFunc = defaultTerminalInteractive

func runBadge(stdin io.Reader, stdout io.Writer, _ io.Writer) error {
	if !terminalInteractiveFunc() {
		return errors.New("badger badge requires an interactive terminal")
	}

	fmt.Fprint(stdout, renderBadgePermissionPrompt())

	choice, err := readKeypress(stdin)
	if err != nil {
		return err
	}
	if choice != 'y' && choice != 'Y' {
		fmt.Fprintln(stdout, "👍 No problem! Run 'badger badge' anytime to see the scoreboard.")
		return nil
	}

	fmt.Fprintln(stdout, "📡 Fetching...")
	logins, total, err := fetchStargazersFunc()
	if err != nil {
		fmt.Fprintln(stdout, renderBadgeError(err))
		return nil
	}

	fmt.Fprint(stdout, renderBadgeScoreboard(logins, total))
	fmt.Fprint(stdout, renderBadgeActionPrompt())

	for {
		choice, err := readKeypress(stdin)
		if err != nil {
			return nil
		}

		switch choice {
		case 's', 'S':
			if err := openBrowserFunc(badgeRepoURL); err != nil {
				fmt.Fprintln(stdout, badgeRepoURL)
			}
		default:
			return nil
		}

		fmt.Fprint(stdout, renderBadgeActionPrompt())
	}
}

func readKeypress(stdin io.Reader) (rune, error) {
	if f, ok := stdin.(*os.File); ok && isCharDevice(f) {
		fd := f.Fd()
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return 0, err
		}
		defer term.Restore(fd, oldState)
		var b [1]byte
		if _, err := f.Read(b[:]); err != nil {
			return 0, err
		}
		return rune(b[0]), nil
	}
	var b [1]byte
	if _, err := stdin.Read(b[:]); err != nil {
		return 0, err
	}
	return rune(b[0]), nil
}

func renderBadgePermissionPrompt() string {
	return strings.Join([]string{
		brand.HeaderRule,
		brand.BadgeHeaderLine("   /\\_/\\", brand.VersionedName(version.Version)),
		brand.BadgeHeaderLine("  ( o.o )", "Local-first code context for any AI chat"),
		brand.BadgeHeaderLine("   > ^ <", "Pipeline: [Map] → Extract → Apply"),
		brand.HeaderRule,
		"",
		"   " + badgeBold("📡 Fetch supporter scoreboard from GitHub?"),
		"",
		"   This will make 1 API call to:",
		"     api.github.com/repos/PVRLabs/aibadger/stargazers",
		"   No data saved. One-time check.",
		"",
		"   " + badgeBold("Fetch scoreboard? (y/N)"),
		"",
	}, "\n")
}

func renderBadgeScoreboard(logins []string, total int) string {
	if total >= 100 {
		var lines []string
		lines = append(lines, "   🦡🦡🦡 A GAZILLION BADGERS have starred this repo!")
		lines = append(lines, "   (Results may be cached — the true number is probably even higher)")
		lines = append(lines, "")
		lines = append(lines, "   🌟 Recent supporters (last 10):")
		for _, login := range logins {
			lines = append(lines, "     @"+login)
		}
		lines = append(lines, "")
		return strings.Join(lines, "\n")
	}

	var lines []string
	lines = append(lines, "   ─────────────────────────────────────────────────")
	lines = append(lines, fmt.Sprintf("   ⭐ TOTAL STARS: %d", total))
	lines = append(lines, "   🌟 Recent supporters (last 10):")
	for _, login := range logins {
		lines = append(lines, "     @"+login)
	}
	lines = append(lines, "   ─────────────────────────────────────────────────")
	lines = append(lines, "")
	lines = append(lines, "   ✨ Your name not here yet?")
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func renderBadgeError(err error) string {
	if errors.Is(err, github.ErrRateLimit) {
		return "⚠️ " + github.ErrRateLimit.Error()
	}
	return "❌ " + err.Error()
}

func renderBadgeActionPrompt() string {
	return strings.Join([]string{
		"",
		"   [S]tar the repo in browser     [Enter] return home",
		"",
	}, "\n")
}

func badgeBold(text string) string {
	return lipgloss.NewStyle().Bold(true).Render(text)
}

func defaultTerminalInteractive() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

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
