package tui

// This file owns status message types, rendering helpers, and shared TUI
// styles.

import "github.com/charmbracelet/lipgloss"

type messageSeverity int

const (
	messageNeutral messageSeverity = iota
	messageSuccess
	messageWarning
	messageError
)

type tuiMessage struct {
	severity messageSeverity
	text     string
}

func neutralMessage(text string) tuiMessage {
	return tuiMessage{severity: messageNeutral, text: text}
}

func successMessage(text string) tuiMessage {
	return tuiMessage{severity: messageSuccess, text: text}
}

func warningMessage(text string) tuiMessage {
	return tuiMessage{severity: messageWarning, text: text}
}

func errorMessage(text string) tuiMessage {
	return tuiMessage{severity: messageError, text: text}
}

func (m tuiMessage) empty() bool {
	return m.text == ""
}

func renderMessage(msg tuiMessage) string {
	switch msg.severity {
	case messageSuccess:
		return successMarkerStyle.Render("✓") + "  " + msg.text
	case messageWarning:
		return warningMarkerStyle.Render("⚠️") + "  " + msg.text
	case messageError:
		return errorMarkerStyle.Render("⛔") + "  " + msg.text
	default:
		return msg.text
	}
}

func renderWarningLine(text string) string {
	return renderMessage(warningMessage(text))
}

func renderBold(text string) string {
	return structuralStyle.Render(text)
}

func renderLabel(label string) string {
	return renderBold(label)
}

var (
	titleStyle         = lipgloss.NewStyle().Bold(true)
	structuralStyle    = lipgloss.NewStyle().Bold(true)
	helpStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	successMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warningMarkerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	errorMarkerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	boxStyle           = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1, 2)
)
