package engine

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var ErrProjectDisabled = errors.New("project explicitly disabled via .badger-disable")

const DisableFileName = ".badger-disable"

type DisabledProjectMessage struct {
	Title  string
	Body   string
	Action string
}

func DisabledMessage() DisabledProjectMessage {
	return DisabledProjectMessage{
		Title:  "⚠️  Project explicitly opted out.",
		Body:   "A .badger-disable file exists in this project's root.\nBadger will not scan, summarize, copy, or generate prompts.",
		Action: "To proceed, remove .badger-disable from the project root.",
	}
}

func CheckDisabled(root string) error {
	disablePath := filepath.Join(root, DisableFileName)
	info, err := os.Stat(disablePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	return ErrProjectDisabled
}

func CheckDisabledAndExit(root string, w io.Writer) error {
	if err := CheckDisabled(root); err != nil {
		if errors.Is(err, ErrProjectDisabled) {
			msg := DisabledMessage()
			fmt.Fprintln(w, msg.Title)
			fmt.Fprintln(w)
			for _, line := range strings.Split(msg.Body, "\n") {
				fmt.Fprintln(w, line)
			}
			fmt.Fprintln(w)
			fmt.Fprintln(w, msg.Action)
			return ErrProjectDisabled
		}
		fmt.Fprintf(os.Stderr, "Error checking project status: %v\n", err)
		return err
	}
	return nil
}
