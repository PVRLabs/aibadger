package tui

// This file owns status message types, rendering helpers, and shared TUI
// styles.

import (
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

type displaySymbols struct {
	success     string
	warning     string
	error       string
	pipelineSep string
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

func defaultDisplaySymbols() displaySymbols {
	return displaySymbolsForRuntime(runtime.GOOS, os.Getenv)
}

func displaySymbolsForRuntime(goos string, getenv func(string) string) displaySymbols {
	if !terminalLikelySupportsUnicode(goos, getenv) {
		return displaySymbols{
			success:     "[OK]",
			warning:     "[!]",
			error:       "[X]",
			pipelineSep: " -> ",
		}
	}
	return displaySymbols{
		success:     "✓",
		warning:     "⚠️",
		error:       "⛔",
		pipelineSep: " → ",
	}
}

func terminalLikelySupportsUnicode(goos string, getenv func(string) string) bool {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if getenv("BADGER_ASCII") != "" || getenv("NO_UNICODE") != "" {
		return false
	}
	if getenv("WT_SESSION") != "" || getenv("ConEmuANSI") == "ON" || getenv("TERM_PROGRAM") != "" {
		return true
	}
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := strings.ToLower(getenv(key))
		if strings.Contains(value, "utf-8") || strings.Contains(value, "utf8") {
			return true
		}
	}
	return goos != "windows"
}

func renderMessage(msg tuiMessage) string {
	symbols := defaultDisplaySymbols()
	switch msg.severity {
	case messageSuccess:
		return successMarkerStyle.Render(symbols.success) + "  " + msg.text
	case messageWarning:
		return warningMarkerStyle.Render(symbols.warning) + "  " + msg.text
	case messageError:
		return errorMarkerStyle.Render(symbols.error) + "  " + msg.text
	default:
		return msg.text
	}
}

func renderMessageWithWidth(msg tuiMessage, width int) string {
	if width <= 0 {
		return renderMessage(msg)
	}

	symbols := defaultDisplaySymbols()
	switch msg.severity {
	case messageSuccess:
		return renderMarkedMessage(successMarkerStyle.Render(symbols.success), msg.text, width)
	case messageWarning:
		return renderMarkedMessage(warningMarkerStyle.Render(symbols.warning), msg.text, width)
	case messageError:
		return renderMarkedMessage(errorMarkerStyle.Render(symbols.error), msg.text, width)
	default:
		return lipgloss.NewStyle().Width(width).Render(msg.text)
	}
}

func renderMarkedMessage(marker, text string, width int) string {
	prefix := marker + "  "
	prefixWidth := lipgloss.Width(prefix)
	bodyWidth := width - prefixWidth
	if bodyWidth <= 0 {
		return prefix + text
	}

	wrapped := lipgloss.NewStyle().Width(bodyWidth).Render(text)
	lines := strings.Split(wrapped, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.Repeat(" ", prefixWidth) + lines[i]
	}
	return prefix + strings.Join(lines, "\n")
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
