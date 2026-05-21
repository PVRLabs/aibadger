package tui

// This file owns input and paste utility helpers shared by the TUI workflow.

import (
	"fmt"
	"unicode/utf8"

	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) enforceGoalByteLimit(warn bool) {
	actualSize := len(m.goalInput.Value())
	retained := trimStringBytes(m.goalInput.Value(), workflow.LargePromptBytes)
	retainedSize := len(retained)
	if actualSize <= retainedSize {
		return
	}
	m.setGoalInputValue(retained)
	if warn {
		m.status = warningMessage(fmt.Sprintf("Pasted goal was truncated from %s to %s.", protocol.FormatFileSize(int64(actualSize)), protocol.FormatFileSize(int64(retainedSize))))
	}
}

func pastedKeyMsg(msg tea.Msg) bool {
	key, ok := msg.(tea.KeyMsg)
	return ok && key.Paste
}

func trimStringBytes(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	trimmed := s[:limit]
	for len(trimmed) > 0 && !utf8.ValidString(trimmed) {
		trimmed = trimmed[:len(trimmed)-1]
	}
	return trimmed
}
