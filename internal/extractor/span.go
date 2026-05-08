package extractor

// This file contains local span extraction and fallback-window algorithms.

import "strings"

const (
	braceLookaheadLimit        = 48
	indentLookaheadLimit       = 24
	declarationLookaheadLimit  = 24
	wholeFileFallbackLineLimit = 24
	wholeFileFallbackByteLimit = 4 * 1024
	fallbackWindowBeforeLines  = 3
	fallbackWindowAfterLines   = 6
)

func (e *Extractor) extractBlock(content, cmdType, pattern string) (string, bool, error) {
	lines := strings.Split(content, "\n")
	startLine := findStartLine(lines, cmdType, pattern)
	if startLine == -1 {
		return buildFallbackWindow(content, lines, -1), len(lines) <= wholeFileFallbackLineLimit && len(content) <= wholeFileFallbackByteLimit, nil
	}

	anchorStart := findAnchorStart(lines, startLine)

	if extracted, ok := extractBraceSpan(lines, anchorStart); ok && isUsefulSpan(extracted) {
		return extracted, false, nil
	}

	if extracted, ok := extractIndentSpan(lines, anchorStart); ok && isUsefulSpan(extracted) {
		return extracted, false, nil
	}

	if extracted, ok := extractDeclarationSpan(lines, anchorStart); ok && isUsefulSpan(extracted) {
		return extracted, false, nil
	}

	return buildFallbackWindow(content, lines, startLine), len(lines) <= wholeFileFallbackLineLimit && len(content) <= wholeFileFallbackByteLimit, nil
}

func findStartLine(lines []string, cmdType, pattern string) int {
	for i, line := range lines {
		if lineMatchesCommand(strings.TrimSpace(line), cmdType, pattern) {
			return i
		}
	}
	return -1
}

func lineMatchesCommand(trimmedLine, cmdType, pattern string) bool {
	if cmdType == "PREFIX" {
		return strings.HasPrefix(trimmedLine, pattern)
	}
	if cmdType == "NEAR" {
		return strings.Contains(trimmedLine, pattern)
	}
	return false
}

func findAnchorStart(lines []string, anchorLine int) int {
	start := anchorLine
	if start < 0 || start >= len(lines) {
		return start
	}

	if !isAttachmentLine(strings.TrimSpace(lines[start])) {
		return start
	}

	for start > 0 {
		prev := strings.TrimSpace(lines[start-1])
		if prev == "" || !isAttachmentLine(prev) {
			break
		}
		start--
	}

	return start
}

func isAttachmentLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	return isCommentLine(trimmed) || strings.HasPrefix(trimmed, "@")
}

func isCommentLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "*/") ||
		strings.HasPrefix(trimmed, "#")
}

func isPythonBlockIntro(trimmed string) bool {
	return strings.HasPrefix(trimmed, "def ") ||
		strings.HasPrefix(trimmed, "class ") ||
		strings.HasPrefix(trimmed, "async def ")
}

func extractBraceSpan(lines []string, startLine int) (string, bool) {
	endLine, ok := findBraceBlockEnd(lines, startLine, braceLookaheadLimit)
	if !ok {
		return "", false
	}
	return buildLineRange(lines, startLine, endLine), true
}

func extractIndentSpan(lines []string, startLine int) (string, bool) {
	endLine, ok := findIndentBlockEnd(lines, startLine, indentLookaheadLimit)
	if !ok {
		return "", false
	}
	return buildLineRange(lines, startLine, endLine), true
}

func extractDeclarationSpan(lines []string, startLine int) (string, bool) {
	endLine, ok := findDeclarationSpanEnd(lines, startLine, declarationLookaheadLimit)
	if !ok {
		return "", false
	}
	return buildLineRange(lines, startLine, endLine), true
}

func findBraceBlockEnd(lines []string, startLine int, maxLookahead int) (int, bool) {
	if startLine < 0 || startLine >= len(lines) {
		return -1, false
	}

	state := newStructuralScanState()
	braceDepth := 0
	opened := false
	endLine := -1

	limit := startLine + maxLookahead
	if limit > len(lines) {
		limit = len(lines)
	}

	for i := startLine; i < limit; i++ {
		delta := scanLineForStructure(lines[i], &state)
		if delta.openBrace > 0 {
			opened = true
		}
		braceDepth += delta.openBrace - delta.closeBrace
		if opened {
			endLine = i
		}
		if opened && braceDepth <= 0 {
			return endLine, true
		}
	}

	return -1, false
}

func findIndentBlockEnd(lines []string, startLine int, maxLookahead int) (int, bool) {
	if startLine < 0 || startLine >= len(lines) {
		return -1, false
	}

	introLine := -1
	introIndent := 0
	limit := startLine + maxLookahead
	if limit > len(lines) {
		limit = len(lines)
	}

	for i := startLine; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if introLine == -1 {
				continue
			}
			break
		}
		if isCommentLine(trimmed) || strings.HasPrefix(trimmed, "@") {
			if introLine == -1 {
				continue
			}
			break
		}
		if isPythonBlockIntro(trimmed) && strings.HasSuffix(trimmed, ":") {
			introLine = i
			introIndent = leadingIndent(lines[i])
			break
		}
		return -1, false
	}

	if introLine == -1 {
		return -1, false
	}

	endLine := introLine
	seenBody := false
	for i := introLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if seenBody {
				endLine = i
			}
			continue
		}
		if isCommentLine(trimmed) && leadingIndent(lines[i]) > introIndent {
			endLine = i
			continue
		}
		if leadingIndent(lines[i]) <= introIndent {
			break
		}
		seenBody = true
		endLine = i
	}

	return endLine, true
}

func findDeclarationSpanEnd(lines []string, startLine int, maxLookahead int) (int, bool) {
	if startLine < 0 || startLine >= len(lines) {
		return -1, false
	}

	limit := startLine + maxLookahead
	if limit > len(lines) {
		limit = len(lines)
	}

	state := newStructuralScanState()
	endLine := -1
	seenCode := false
	startIndent := leadingIndent(lines[startLine])

	for i := startLine; i < limit; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if seenCode {
				break
			}
			continue
		}

		if isCommentLine(trimmed) || strings.HasPrefix(trimmed, "@") {
			if !seenCode {
				endLine = i
				continue
			}
			if leadingIndent(lines[i]) > startIndent {
				endLine = i
				continue
			}
			break
		}

		if isPythonBlockIntro(trimmed) && strings.HasSuffix(trimmed, ":") {
			return -1, false
		}
		if !seenCode && !looksLikeDeclarationLine(trimmed) {
			return -1, false
		}
		if seenCode && !looksLikeDeclarationLine(trimmed) && !lineHasTrailingContinuation(trimmed) {
			break
		}

		delta := scanLineForStructure(lines[i], &state)
		if delta.openBrace > 0 {
			return -1, false
		}

		seenCode = true
		endLine = i

		if delta.semicolon > 0 {
			break
		}
		if lineHasTrailingContinuation(trimmed) {
			continue
		}
		if i+1 < limit {
			nextTrimmed := strings.TrimSpace(lines[i+1])
			if nextTrimmed != "" && leadingIndent(lines[i+1]) > startIndent {
				continue
			}
		}
		break
	}

	if endLine < startLine {
		return -1, false
	}

	return endLine, true
}

func looksLikeDeclarationLine(trimmed string) bool {
	if trimmed == "" {
		return false
	}
	if isPythonBlockIntro(trimmed) {
		return true
	}
	if strings.Contains(trimmed, "=>") || strings.Contains(trimmed, "->") {
		return true
	}
	if strings.ContainsAny(trimmed, "=(){}[]<>;") {
		return true
	}

	prefixes := []string{
		"const ",
		"let ",
		"var ",
		"func ",
		"type ",
		"interface ",
		"class ",
		"export ",
		"public ",
		"private ",
		"protected ",
		"static ",
		"package ",
		"import ",
		"return ",
		"if ",
		"for ",
		"switch ",
		"case ",
		"async ",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}

	return false
}

func buildFallbackWindow(content string, lines []string, anchorLine int) string {
	if len(lines) <= wholeFileFallbackLineLimit && len(content) <= wholeFileFallbackByteLimit {
		return content
	}

	if anchorLine < 0 {
		anchorLine = 0
	}
	start := anchorLine - fallbackWindowBeforeLines
	if start < 0 {
		start = 0
	}
	end := anchorLine + fallbackWindowAfterLines
	if end >= len(lines) {
		end = len(lines) - 1
	}

	return buildLineRange(lines, start, end)
}

func buildLineRange(lines []string, startLine, endLine int) string {
	if startLine < 0 || endLine < startLine || startLine >= len(lines) {
		return ""
	}
	if endLine >= len(lines) {
		endLine = len(lines) - 1
	}

	var result strings.Builder
	for i := startLine; i <= endLine; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}
	return result.String()
}

func isUsefulSpan(span string) bool {
	lines := strings.Split(span, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !isCommentLine(trimmed) {
			return true
		}
	}
	return false
}

func leadingIndent(line string) int {
	count := 0
	for _, r := range line {
		if r == ' ' {
			count++
			continue
		}
		if r == '\t' {
			count += 4
			continue
		}
		break
	}
	return count
}

func lineHasTrailingContinuation(trimmed string) bool {
	return strings.HasSuffix(trimmed, ",") ||
		strings.HasSuffix(trimmed, ".") ||
		strings.HasSuffix(trimmed, "+") ||
		strings.HasSuffix(trimmed, "-") ||
		strings.HasSuffix(trimmed, "*") ||
		strings.HasSuffix(trimmed, "/") ||
		strings.HasSuffix(trimmed, "=") ||
		strings.HasSuffix(trimmed, "(") ||
		strings.HasSuffix(trimmed, "[") ||
		strings.HasSuffix(trimmed, "{") ||
		strings.HasSuffix(trimmed, "\\") ||
		strings.HasSuffix(trimmed, ":")
}

type structuralScanState struct {
	inBlockComment bool
	inSingleQuote  bool
	inDoubleQuote  bool
	inBacktick     bool
}

type structuralDelta struct {
	openBrace  int
	closeBrace int
	semicolon  int
}

func newStructuralScanState() structuralScanState {
	return structuralScanState{}
}

// scanLineForStructure counts structural braces and semicolons in a line,
// ignoring tokens inside comments or quoted strings. The state tracks
// multi-line block comments and string contexts across scanned lines.
func scanLineForStructure(line string, state *structuralScanState) structuralDelta {
	var delta structuralDelta

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if state.inBlockComment {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				state.inBlockComment = false
				i++
			}
			continue
		}
		if state.inSingleQuote {
			if ch == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if ch == '\'' {
				state.inSingleQuote = false
			}
			continue
		}
		if state.inDoubleQuote {
			if ch == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if ch == '"' {
				state.inDoubleQuote = false
			}
			continue
		}
		if state.inBacktick {
			if ch == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if ch == '`' {
				state.inBacktick = false
			}
			continue
		}

		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}
		if ch == '/' && i+1 < len(line) && line[i+1] == '*' {
			state.inBlockComment = true
			i++
			continue
		}
		if ch == '#' {
			break
		}

		switch ch {
		case '\'':
			state.inSingleQuote = true
		case '"':
			state.inDoubleQuote = true
		case '`':
			state.inBacktick = true
		case '{':
			delta.openBrace++
		case '}':
			delta.closeBrace++
		case ';':
			delta.semicolon++
		}
	}

	return delta
}
