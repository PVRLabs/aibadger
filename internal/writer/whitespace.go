package writer

import (
	"strings"
)

const indentScanLimit = 200

type WhitespaceMode string

const (
	WhitespaceModeExact  WhitespaceMode = "exact"
	WhitespaceModeSmart  WhitespaceMode = "smart"
	WhitespaceModeIgnore WhitespaceMode = "ignore"
)

const DefaultWhitespaceMode = WhitespaceModeSmart

type indentStyle struct {
	useTabs    bool
	spaceWidth int
}

func detectIndentStyle(content string) indentStyle {
	tabVotes := 0
	spaceVotes := 0
	spaceWidths := make(map[int]int)
	scannedIndented := 0

	for _, line := range strings.Split(content, "\n") {
		if scannedIndented >= indentScanLimit {
			break
		}
		if line == "" {
			continue
		}
		switch line[0] {
		case '\t':
			tabVotes++
			scannedIndented++
		case ' ':
			width := countLeadingChars(line, ' ')
			if width == 0 {
				continue
			}
			spaceVotes++
			spaceWidths[width]++
			scannedIndented++
		}
	}

	if tabVotes == 0 && spaceVotes == 0 {
		return indentStyle{useTabs: true}
	}
	if tabVotes >= spaceVotes {
		return indentStyle{useTabs: true}
	}

	bestWidth := 4
	bestVotes := -1
	for width, votes := range spaceWidths {
		if votes > bestVotes || (votes == bestVotes && width < bestWidth) {
			bestWidth = width
			bestVotes = votes
		}
	}
	if bestWidth <= 0 {
		bestWidth = 4
	}
	return indentStyle{spaceWidth: bestWidth}
}

func normalizeContent(incoming, existing string, mode WhitespaceMode) string {
	if existing == "" {
		return incoming
	}

	target := detectIndentStyle(existing)
	source := detectIndentStyle(incoming)

	sourceUnit := 1
	if !source.useTabs {
		sourceUnit = source.spaceWidth
	}
	if sourceUnit <= 0 {
		sourceUnit = 1
	}

	stripAll := mode == WhitespaceModeIgnore
	lines := strings.Split(incoming, "\n")
	for i, line := range lines {
		lines[i] = normalizeLine(line, target, sourceUnit, stripAll)
	}
	return strings.Join(lines, "\n")
}

func normalizeLine(line string, target indentStyle, sourceUnit int, stripAll bool) string {
	if line == "" {
		return line
	}
	if sourceUnit <= 0 {
		sourceUnit = 1
	}

	indentChars, rest := splitLeadingIndent(line)
	if rest == "" {
		return ""
	}

	depth := 0
	remainder := 0
	if stripAll {
		depth = indentChars / sourceUnit
	} else if line[0] == '\t' {
		depth = countLeadingChars(line, '\t')
	} else if line[0] == ' ' {
		leadingSpaces := countLeadingChars(line, ' ')
		depth = leadingSpaces / sourceUnit
		remainder = leadingSpaces % sourceUnit
	}
	if depth < 0 {
		depth = 0
	}

	if target.useTabs {
		return strings.Repeat("\t", depth) + strings.Repeat(" ", remainder) + rest
	}
	return strings.Repeat(" ", depth*target.spaceWidth+remainder) + rest
}

func countLeadingChars(line string, want byte) int {
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] != want {
			break
		}
		count++
	}
	return count
}

func splitLeadingIndent(line string) (int, string) {
	count := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t':
			count++
			continue
		}
		return count, line[i:]
	}
	return count, ""
}
