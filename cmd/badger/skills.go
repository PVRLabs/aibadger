package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PVRLabs/aibadger/internal/skillsinstall"
	"github.com/PVRLabs/aibadger/skills"
)

var userHomeDirFunc = os.UserHomeDir

var installSkillsFunc = func(root string, stdout, stderr io.Writer) error {
	return skillsinstall.Install(root, skills.Definitions(), stdout, stderr)
}

func runSkillsCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return reportSkillsCommandError(stderr, fmt.Errorf("skills command requires install; use `badger skills install`"))
	}
	if isHelpArg(args[0]) {
		if len(args) != 1 {
			return reportSkillsCommandError(stderr, fmt.Errorf("badger skills help does not accept extra arguments"))
		}
		printSkillsHelp(stdout)
		return nil
	}
	if args[0] != "install" {
		return reportSkillsCommandError(stderr, fmt.Errorf("unknown skills command: %s", args[0]))
	}
	if len(args) > 1 {
		if len(args) == 2 && isHelpArg(args[1]) {
			printSkillsInstallHelp(stdout)
			return nil
		}
		return reportSkillsCommandError(stderr, fmt.Errorf("badger skills install does not accept arguments: %s", args[1]))
	}

	home, err := userHomeDirFunc()
	if err != nil {
		return reportSkillsCommandError(stderr, fmt.Errorf("resolve user home: %w", err))
	}
	root := filepath.Join(home, ".agents", "skills")
	diagnostics := &trackingWriter{Writer: stderr}
	err = installSkillsFunc(root, stdout, diagnostics)
	if err != nil && !diagnostics.Wrote {
		return reportSkillsCommandError(stderr, err)
	}
	return err
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h"
}

func reportSkillsCommandError(stderr io.Writer, err error) error {
	if stderr != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
	}
	return err
}

type trackingWriter struct {
	io.Writer
	Wrote bool
}

func (w *trackingWriter) Write(p []byte) (int, error) {
	w.Wrote = true
	if w.Writer == nil {
		return len(p), nil
	}
	return w.Writer.Write(p)
}

func printSkillsHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  badger skills install

Purpose:
  Install Badger's bundled official Agent Skills offline.

Run `+"`badger skills install --help`"+` for command details.
`)
}

func printSkillsInstallHelp(w io.Writer) {
	fmt.Fprint(w, `Usage:
  badger skills install

Purpose:
  Install or update all bundled official Agent Skills from this binary.

Destination:
  ~/.agents/skills/

The command installs badger-review and badger-handoff, preserves unrelated
skills and files, uses no network, and does not inspect a repository.
`)
}
