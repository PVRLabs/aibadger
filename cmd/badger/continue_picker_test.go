package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/sessionimport"
	"github.com/PVRLabs/aibadger/internal/version"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

func pickerSessions(count int) []sessionimport.Summary {
	base := time.Date(2026, 9, 22, 22, 10, 0, 0, time.UTC)
	out := make([]sessionimport.Summary, count)
	for i := range out {
		out[i] = sessionimport.Summary{
			ID:               fmt.Sprintf("%08d-1111-1111-1111-111111111111", i),
			StartedAt:        base.Add(-time.Duration(i) * time.Hour),
			Label:            fmt.Sprintf("Session %d with a useful task description", i+1),
			WorkingDirectory: "/tmp/repository",
			ProjectMatch:     i == 0,
		}
	}
	return out
}

func updatePicker(m codexPickerModel, msg tea.Msg) codexPickerModel {
	next, _ := m.Update(msg)
	return next.(codexPickerModel)
}

func TestCodexPickerHeaderLocalTimeAndHeight(t *testing.T) {
	location := time.FixedZone("PDT", -7*60*60)
	m := newCodexPickerModel(pickerSessions(40), location)
	m = updatePicker(m, tea.WindowSizeMsg{Width: 80, Height: 50})
	view := m.View()
	for _, part := range []string{" /\\_/\\", "🦡 AIBADGER " + version.Version, "Local-first code context for any AI chat", "Pipeline: [Map] → Extract → Apply", "2026-09-22 15:10", "[current repo]"} {
		if !strings.Contains(view, part) {
			t.Fatalf("picker view missing %q:\n%s", part, view)
		}
	}
	if strings.Contains(view, "2026-09-22 22:10") || strings.Contains(view, "Session 40") {
		t.Fatalf("picker did not convert time or bound viewport:\n%s", view)
	}
	if got := strings.Count(view, "\n") + 1; got != pickerMaxHeight {
		t.Fatalf("rendered rows = %d, want %d", got, pickerMaxHeight)
	}
	for _, line := range strings.Split(view, "\n") {
		if runewidth.StringWidth(line) > 80 {
			t.Fatalf("row exceeds width: %q", line)
		}
	}
}

func TestCodexPickerArrowNavigationScrollResizeAndSelection(t *testing.T) {
	m := newCodexPickerModel(pickerSessions(40), time.UTC)
	if m.selected != 0 || m.top != 0 || !strings.Contains(m.View(), ">  1.") {
		t.Fatalf("initial selection = %+v", m)
	}
	for i := 0; i < 20; i++ {
		m = updatePicker(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if m.selected != 20 || m.top != 7 || !strings.Contains(m.View(), "> 21.") || strings.Contains(m.View(), "Session 1 ") {
		t.Fatalf("scroll position = selected %d, top %d\n%s", m.selected, m.top, m.View())
	}
	m = updatePicker(m, tea.WindowSizeMsg{Width: 64, Height: 12})
	if m.rows() != 2 || m.top != 19 || strings.Count(m.View(), "\n")+1 != 12 {
		t.Fatalf("short terminal = selected %d, top %d\n%s", m.selected, m.top, m.View())
	}
	m = updatePicker(m, tea.KeyMsg{Type: tea.KeyUp})
	if m.selected != 19 || !strings.Contains(m.View(), "> 20.") {
		t.Fatalf("up key = selected %d\n%s", m.selected, m.View())
	}
	m = updatePicker(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !m.done || m.canceled || m.sessions[m.selected].ID != pickerSessions(40)[19].ID {
		t.Fatalf("selected session = %+v", m)
	}
}

func TestCodexPickerCancellationAndVerySmallTerminal(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyEsc}, {Type: tea.KeyRunes, Runes: []rune{'q'}}} {
		m := updatePicker(newCodexPickerModel(pickerSessions(2), time.UTC), key)
		if !m.canceled || m.done {
			t.Fatalf("cancel key %q = %+v", key.String(), m)
		}
	}
	m := updatePicker(newCodexPickerModel(pickerSessions(2), time.UTC), tea.WindowSizeMsg{Width: 30, Height: 6})
	if got := strings.Count(m.View(), "\n") + 1; got != 6 {
		t.Fatalf("tiny terminal rows = %d\n%s", got, m.View())
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if runewidth.StringWidth(line) > 30 {
			t.Fatalf("tiny terminal row exceeds width: %q", line)
		}
	}
}
