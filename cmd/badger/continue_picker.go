package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/PVRLabs/aibadger/internal/brand"
	"github.com/PVRLabs/aibadger/internal/sessionimport"
	"github.com/PVRLabs/aibadger/internal/version"
	"github.com/PVRLabs/aibadger/internal/workflow"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

const pickerMaxHeight = 24

type codexPickerModel struct {
	sessions       []sessionimport.Summary
	location       *time.Location
	width, height  int
	selected, top  int
	done, canceled bool
}

func newCodexPickerModel(sessions []sessionimport.Summary, location *time.Location) codexPickerModel {
	return codexPickerModel{sessions: sessions, location: location, width: 80, height: pickerMaxHeight}
}

func runCodexPicker(sessions []sessionimport.Summary, input io.Reader, output io.Writer, location *time.Location) (string, error) {
	model := newCodexPickerModel(sessions, location)
	result, err := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(input), tea.WithOutput(output)).Run()
	if err != nil {
		return "", err
	}
	final, ok := result.(codexPickerModel)
	if !ok || final.canceled || !final.done {
		return "", fmt.Errorf("Codex session selection canceled")
	}
	return final.sessions[final.selected].ID, nil
}

func (m codexPickerModel) Init() tea.Cmd { return nil }

func (m codexPickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected+1 < len(m.sessions) {
				m.selected++
			}
		case "enter":
			if len(m.sessions) > 0 {
				m.done = true
				return m, tea.Quit
			}
		case "esc", "q", "ctrl+c":
			m.canceled = true
			return m, tea.Quit
		}
	}
	m.keepSelectedVisible()
	return m, nil
}

func (m codexPickerModel) rows() int {
	height := m.height
	if height <= 0 || height > pickerMaxHeight {
		height = pickerMaxHeight
	}
	if height >= 12 {
		return height - 10
	} // full header, title, separators, footer
	if height >= 3 {
		return height - 2
	} // compact header and footer
	return 0
}

func (m *codexPickerModel) keepSelectedVisible() {
	rows := m.rows()
	if rows <= 0 {
		return
	}
	if m.selected < m.top {
		m.top = m.selected
	}
	if m.selected >= m.top+rows {
		m.top = m.selected - rows + 1
	}
	maxTop := len(m.sessions) - rows
	if maxTop < 0 {
		maxTop = 0
	}
	if m.top > maxTop {
		m.top = maxTop
	}
}

func (m codexPickerModel) View() string {
	height := m.height
	if height <= 0 || height > pickerMaxHeight {
		height = pickerMaxHeight
	}
	width := m.width
	if width <= 0 {
		width = 80
	}
	clip := func(s string) string { return runewidth.Truncate(s, width, "") }
	var lines []string
	if height >= 12 {
		lines = append(lines,
			clip(brand.HeaderRule),
			clip(brand.HeaderLine(" /\\_/\\", brand.VersionedName(version.Version))),
			clip(brand.HeaderLine("( o.o )", brand.Subtitle)),
			clip(brand.HeaderLine(" > ^ <", "Pipeline: [Map] → Extract → "+workflow.PipelineFinalLabel)),
			clip(brand.HeaderRule),
			"", clip("Choose a Codex session"), "",
		)
	} else if height > 0 {
		lines = append(lines, clip(brand.VersionedName(version.Version)))
	}
	rows := m.rows()
	for i := 0; i < rows; i++ {
		index := m.top + i
		if index >= len(m.sessions) {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, m.row(index, width))
	}
	if height >= 12 {
		lines = append(lines, "")
	}
	if height >= 2 {
		footer := fmt.Sprintf("↑/↓ move · Enter select · Esc/q cancel  (%d/%d)", m.selected+1, len(m.sessions))
		lines = append(lines, clip(footer))
	}
	return strings.Join(lines, "\n")
}

func (m codexPickerModel) row(index, width int) string {
	item := m.sessions[index]
	marker := "  "
	if index == m.selected {
		marker = "> "
	}
	prefix := fmt.Sprintf("%s%2d. %s  ", marker, index+1, item.StartedAt.In(m.location).Format("2006-01-02 15:04"))
	association := "repo unknown"
	if item.ProjectMatch {
		association = "current repo"
	} else if item.WorkingDirectory != "" {
		association = "repo " + strings.Join(strings.Fields(filepath.Base(item.WorkingDirectory)), " ")
	}
	suffix := "  [" + association + "]"
	labelWidth := width - runewidth.StringWidth(prefix) - runewidth.StringWidth(suffix)
	if labelWidth < 0 {
		labelWidth = 0
	}
	label := runewidth.Truncate(item.Label, labelWidth, "…")
	return runewidth.Truncate(prefix+label+suffix, width, "")
}
