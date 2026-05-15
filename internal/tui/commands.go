package tui

// This file owns Bubble Tea command constructors used by the TUI workflow.

import (
	"time"

	"github.com/PVRLabs/aibadger/internal/clipboard"
	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/extractor"
	"github.com/PVRLabs/aibadger/internal/workflow"
	"github.com/PVRLabs/aibadger/internal/writer"
	tea "github.com/charmbracelet/bubbletea"
)

func scanProjectCmd(root string) tea.Cmd {
	return func() tea.Msg {
		eng, err := engine.New(root)
		return scanDoneMsg{eng: eng, err: err}
	}
}

func scanTick() tea.Cmd {
	return tea.Tick(180*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func copyCmd(kind, text string) tea.Cmd {
	return func() tea.Msg {
		return copyDoneMsg{kind: kind, text: text, err: clipboard.Copy(text)}
	}
}

func savePromptCmd(kind, text string) tea.Cmd {
	return func() tea.Msg {
		path, err := savePromptToTemp(kind, text)
		return savePromptDoneMsg{kind: kind, path: path, canReveal: err == nil && promptFileRevealAvailable(path), err: err}
	}
}

func openPromptFileCmd(kind, path string) tea.Cmd {
	return func() tea.Msg {
		return openPromptFileDoneMsg{kind: kind, path: path, err: revealPromptFile(path)}
	}
}

func contextCmd(session *workflow.Session, goal string, commands []extractor.Command) tea.Cmd {
	return func() tea.Msg {
		schema, metadata, extractedCount, failedCommands, err := session.GenerateContextDetailed(goal, commands)
		return contextDoneMsg{
			schema:         schema,
			metadata:       metadata,
			extractedCount: extractedCount,
			failedCommands: failedCommands,
			err:            err,
		}
	}
}

func writeCmd(session *workflow.Session, updates []writer.FileUpdate) tea.Cmd {
	return func() tea.Msg {
		applied, errs := session.ApplyWrites(updates)
		return writeDoneMsg{updates: applied, errs: errs}
	}
}

func countAppliedKinds(updates []writer.FileUpdate) (writes, deletes int) {
	for _, update := range updates {
		if update.Kind == writer.UpdateKindDelete {
			deletes++
			continue
		}
		writes++
	}
	return writes, deletes
}
