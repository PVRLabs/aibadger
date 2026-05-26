package main

import (
	"errors"
	"os"

	"github.com/PVRLabs/aibadger/pkg/badger"
	"github.com/charmbracelet/x/term"
)

const badgeStartupGoal = "/badge"

var terminalInteractiveFunc = defaultTerminalInteractive

func applyBadgeStartup(app *appConfig, badgerCfg *badger.Config) error {
	if app.headless {
		return errors.New("badger badge does not support --headless")
	}
	if !terminalInteractiveFunc() {
		return errors.New("badger badge requires an interactive terminal")
	}

	badgerCfg.SkipOnboarding = true
	badgerCfg.StartupGoal = badgeStartupGoal
	return nil
}

func defaultTerminalInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}
