package tui

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func explorerRevealCommand(name, selection string) (*exec.Cmd, error) {
	path := strings.TrimPrefix(selection, "/select,")
	if path == selection || strings.ContainsAny(path, "\"\x00\r\n") {
		return nil, fmt.Errorf("invalid Explorer selection path")
	}
	cmd := exec.Command(name)
	// Explorer parses /select itself; Go's ordinary argv quoting is unsuitable.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(name) + ` /select,"` + path + `"`}
	return cmd, nil
}

func runPromptFileReveal(name string, args ...string) error {
	if len(args) != 1 {
		return fmt.Errorf("expected one Explorer selection")
	}
	cmd, err := explorerRevealCommand(name, args[0])
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Explorer may delegate to an existing process. Its exit status does not
	// establish whether the selection succeeded, but Wait must reap resources.
	go func() { _ = cmd.Wait() }()
	return nil
}
