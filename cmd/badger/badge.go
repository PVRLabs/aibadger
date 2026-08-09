package main

import (
	"errors"
	"os"

	"github.com/PVRLabs/aibadger/pkg/badger"
	"github.com/charmbracelet/x/term"
)

const badgeStartupGoal = "/badge"

var terminalInteractiveFunc = defaultTerminalInteractive

func applyBadgeStartup(badgerCfg *badger.Config) error {
	if !terminalInteractiveFunc() {
		return errors.New("badger badge requires an interactive terminal")
	}

	badgerCfg.SkipOnboarding = true
	badgerCfg.Startup = badger.StartupContext{Goal: badgeStartupGoal}
	return nil
}

func defaultTerminalInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}
