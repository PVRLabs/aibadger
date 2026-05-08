package writer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpdateKind identifies how a planned file operation should be applied.
type UpdateKind string

const (
	UpdateKindWrite  UpdateKind = "write"
	UpdateKindDelete UpdateKind = "delete"
)

// FileUpdate represents a single file operation planned from the AI response.
type FileUpdate struct {
	Path    string
	Content string
	Kind    UpdateKind
}

// ParseResult captures both file updates and any non-file text in an AI response.
type ParseResult struct {
	Updates []FileUpdate
	Text    string
	Errors  []error
}

// ParseAIResponse extracts file updates from the AI's response.
func ParseAIResponse(input string) []FileUpdate {
	return ParseAIResponseDetailed(input).Updates
}

// ParseAIResponseDetailed extracts file updates and preserves non-file text.
func ParseAIResponseDetailed(input string) ParseResult {
	var updates []FileUpdate
	var currentPath string
	var currentContent strings.Builder
	var textContent strings.Builder
	var errs []error
	inBlock := false
	inMarkdownFence := false
	currentKind := UpdateKindWrite

	lines := scanLines(input)
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)

		// Ignore markdown fences only when they are framing the response, not when
		// they are part of a file's literal content.
		if !inBlock && strings.HasPrefix(line, "```") {
			continue
		}

		if inBlock {
			// Get the raw line from the scanner to preserve intentional indentation
			if line == "--- End File ---" && !(isMarkdownPath(currentPath) && inMarkdownFence) {
				updates = append(updates, FileUpdate{
					Path:    currentPath,
					Content: currentContent.String(),
					Kind:    currentKind,
				})
				inBlock = false
				inMarkdownFence = false
				continue
			}
			if isMarkdownPath(currentPath) && strings.HasPrefix(line, "```") {
				inMarkdownFence = !inMarkdownFence
			}
			currentContent.WriteString(rawLine)
			currentContent.WriteString("\n")
			continue
		}

		if strings.HasPrefix(line, "--- File: ") && strings.HasSuffix(line, " ---") {
			currentPath = strings.TrimPrefix(line, "--- File: ")
			currentPath = strings.TrimSuffix(currentPath, " ---")
			if isProtocolExamplePath(currentPath) {
				textContent.WriteString(rawLine)
				textContent.WriteString("\n")
				continue
			}
			if err := validatePlannedPath(currentPath); err != nil {
				errs = append(errs, fmt.Errorf("write file %q: %w", currentPath, err))
				continue
			}
			currentContent.Reset()
			currentKind = UpdateKindWrite
			inBlock = true
			inMarkdownFence = false
			continue
		}

		if strings.HasPrefix(line, "--- Delete File: ") && strings.HasSuffix(line, " ---") {
			currentPath = strings.TrimPrefix(line, "--- Delete File: ")
			currentPath = strings.TrimSuffix(currentPath, " ---")
			if isProtocolExamplePath(currentPath) {
				textContent.WriteString(rawLine)
				textContent.WriteString("\n")
				continue
			}
			if err := validatePlannedPath(currentPath); err != nil {
				errs = append(errs, fmt.Errorf("delete file %q: %w", currentPath, err))
				continue
			}
			updates = append(updates, FileUpdate{
				Path: currentPath,
				Kind: UpdateKindDelete,
			})
			continue
		}

		textContent.WriteString(rawLine)
		textContent.WriteString("\n")
	}

	return ParseResult{
		Updates: updates,
		Text:    strings.TrimSpace(textContent.String()),
		Errors:  errs,
	}
}

func scanLines(input string) []string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func isMarkdownPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown" || ext == ".mdown" || ext == ".mkd"
}

func isProtocolExamplePath(path string) bool {
	return strings.ContainsAny(path, "<>") || strings.Contains(path, "(Extracted Block)")
}

// WriteFile writes the update to disk, creating parent directories if needed.
func WriteFile(projectRoot string, update FileUpdate, mode WhitespaceMode) error {
	fullPath, err := resolvePlannedPath(projectRoot, update.Path)
	if err != nil {
		return err
	}

	if update.Kind == UpdateKindDelete {
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("cannot delete missing file %s", update.Path)
			}
			return fmt.Errorf("failed to stat %s: %w", update.Path, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("cannot delete non-regular file %s", update.Path)
		}
		if err := os.Remove(fullPath); err != nil {
			return fmt.Errorf("failed to delete %s: %w", update.Path, err)
		}
		return nil
	}

	// Create parent directories for writes.
	parent := filepath.Dir(fullPath)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", parent, err)
	}

	content := update.Content
	if mode != WhitespaceModeExact {
		existing, err := os.ReadFile(fullPath)
		if err == nil {
			content = normalizeContent(update.Content, string(existing), mode)
		}
	}

	return os.WriteFile(fullPath, []byte(content), 0644)
}

func resolvePlannedPath(projectRoot, relPath string) (string, error) {
	if err := validatePlannedPath(relPath); err != nil {
		return "", err
	}

	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return "", fmt.Errorf("failed to resolve project root: %w", err)
	}
	realRoot := absRoot
	if resolvedRoot, err := filepath.EvalSymlinks(absRoot); err == nil {
		realRoot = resolvedRoot
	}

	fullPath := filepath.Clean(filepath.Join(absRoot, relPath))
	resolvedPath, err := resolveExistingPlannedPath(fullPath)
	if err != nil {
		return "", err
	}

	rel, err := filepath.Rel(realRoot, resolvedPath)
	if err != nil {
		return "", fmt.Errorf("failed to validate %s: %w", relPath, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes project root: %s", relPath)
	}
	return fullPath, nil
}

func resolveExistingPlannedPath(path string) (string, error) {
	current := path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if resolved, err := filepath.EvalSymlinks(current); err == nil {
				return resolved, nil
			} else if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("failed to resolve symlink %s: %w", current, err)
			}
			return current, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to inspect %s: %w", current, err)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("path does not exist: %s", path)
		}
		current = parent
	}
}

func validatePlannedPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("path is empty")
	}
	if filepath.IsAbs(path) {
		return fmt.Errorf("absolute paths are not allowed")
	}
	if strings.ContainsAny(path, "*?[]") {
		return fmt.Errorf("wildcards are not allowed")
	}
	if strings.ContainsAny(path, "<>") {
		return fmt.Errorf("placeholder paths are not allowed")
	}
	if hasParentTraversal(path) {
		return fmt.Errorf("parent traversal is not allowed")
	}
	if filepath.Clean(path) == "." {
		return fmt.Errorf("path is not valid")
	}
	return nil
}

func hasParentTraversal(path string) bool {
	parts := strings.FieldsFunc(path, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}
