// Package handoff consumes workspace-local Badger handoff files.
package handoff

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	Filename     = ".badger-handoff"
	MaxFileBytes = 64 * 1024
)

type Mode string

const (
	ModeReview  Mode = "review"
	ModeDesign  Mode = "design"
	ModeHandoff Mode = "handoff"
)

type Content struct {
	Mode Mode
	Body string
}

func Consume(root string) (Content, error) {
	path := filepath.Join(root, Filename)
	file, err := os.Open(path)
	if err != nil {
		return Content{}, fmt.Errorf("opening handoff file: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return Content{}, fmt.Errorf("inspecting handoff file: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Content{}, fmt.Errorf("handoff path is not a regular file")
	}

	data, err := io.ReadAll(io.LimitReader(file, MaxFileBytes+1))
	if err != nil {
		return Content{}, fmt.Errorf("reading handoff file: %w", err)
	}
	if len(data) > MaxFileBytes {
		return Content{}, fmt.Errorf("handoff file exceeds %d bytes", MaxFileBytes)
	}
	content, err := parse(data)
	if err != nil {
		return Content{}, err
	}
	if err := file.Close(); err != nil {
		return Content{}, fmt.Errorf("closing handoff file: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return Content{}, fmt.Errorf("removing handoff file: %w", err)
	}
	return content, nil
}

func parse(data []byte) (Content, error) {
	if !utf8.Valid(data) {
		return Content{}, fmt.Errorf("handoff file is not valid UTF-8")
	}
	for _, newline := range []string{"\n", "\r\n"} {
		for _, mode := range []Mode{ModeReview, ModeDesign, ModeHandoff} {
			prefix := []byte("BADGER-HANDOFF-V1" + newline + "mode: " + string(mode) + newline + newline)
			if !bytes.HasPrefix(data, prefix) {
				continue
			}
			body := string(data[len(prefix):])
			if strings.TrimSpace(body) == "" {
				return Content{}, fmt.Errorf("handoff body is empty")
			}
			return Content{Mode: mode, Body: body}, nil
		}
	}
	return Content{}, fmt.Errorf("invalid BADGER-HANDOFF-V1 framing")
}
