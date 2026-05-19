package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func isLargePrompt(text string, threshold int) bool {
	return threshold >= 0 && len(text) > threshold
}

func savePromptToTemp(kind, text string) (string, error) {
	return savePromptToTempAt(kind, text, os.TempDir(), time.Now())
}

func savePromptToTempAt(kind, text, tempRoot string, now time.Time) (string, error) {
	dir, err := promptTempDir(tempRoot)
	if err != nil {
		return "", err
	}

	base := fmt.Sprintf("%s-%s", promptFileSlug(kind), now.Format("2006-01-02-1504"))
	for i := 0; i < 100; i++ {
		name := base + ".txt"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.txt", base, i+1)
		}
		path := filepath.Join(dir, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if _, err := file.WriteString(text); err != nil {
			_ = file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		return path, nil
	}
	return "", fmt.Errorf("could not create unique temp prompt file")
}

func promptTempDir(tempRoot string) (string, error) {
	for _, name := range []string{"badger", "badger_tmp"} {
		dir := filepath.Join(tempRoot, name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			continue
		}
		return dir, nil
	}
	return "", fmt.Errorf("could not create temp prompt directory under %s", tempRoot)
}

func promptFileSlug(kind string) string {
	switch kind {
	case topologyPromptKind:
		return "prompt-1-topology"
	case codeContextPromptKind:
		return "prompt-2-code-context"
	}
	slug := strings.ToLower(kind)
	slug = strings.ReplaceAll(slug, ":", "")
	slug = strings.ReplaceAll(slug, " ", "-")
	return slug
}
