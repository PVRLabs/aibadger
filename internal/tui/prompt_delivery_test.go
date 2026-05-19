package tui

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSavePromptToTempAtWritesExactReadableTextFile(t *testing.T) {
	tempRoot := t.TempDir()
	prompt := "[CODE CONTEXT]\nhello\n"
	when := time.Date(2026, 5, 1, 14, 32, 0, 0, time.Local)

	path, err := savePromptToTempAt(codeContextPromptKind, prompt, tempRoot, when)
	if err != nil {
		t.Fatalf("savePromptToTempAt() error = %v", err)
	}

	wantSuffix := filepath.Join("badger", "prompt-2-code-context-2026-05-01-1432.txt")
	if !strings.HasSuffix(path, wantSuffix) {
		t.Fatalf("path = %q, want suffix %q", path, wantSuffix)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(content) != prompt {
		t.Fatalf("saved content = %q, want exact prompt %q", string(content), prompt)
	}
}

func TestSavePromptToTempAtRetriesReadableNameOnCollision(t *testing.T) {
	tempRoot := t.TempDir()
	when := time.Date(2026, 5, 1, 14, 32, 0, 0, time.Local)
	first, err := savePromptToTempAt(topologyPromptKind, "first", tempRoot, when)
	if err != nil {
		t.Fatalf("first savePromptToTempAt() error = %v", err)
	}

	second, err := savePromptToTempAt(topologyPromptKind, "second", tempRoot, when)
	if err != nil {
		t.Fatalf("second savePromptToTempAt() error = %v", err)
	}

	if first == second {
		t.Fatalf("paths are equal after collision: %q", first)
	}
	if !strings.HasSuffix(second, filepath.Join("badger", "prompt-1-topology-2026-05-01-1432-2.txt")) {
		t.Fatalf("second path = %q, want readable collision suffix", second)
	}
	content, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("ReadFile(second) error = %v", err)
	}
	if string(content) != "second" {
		t.Fatalf("second content = %q, want second", string(content))
	}
}

func TestSavePromptToTempAtFallsBackWhenBadgerPathIsFile(t *testing.T) {
	tempRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempRoot, "badger"), []byte("binary"), 0600); err != nil {
		t.Fatalf("WriteFile(badger) error = %v", err)
	}
	when := time.Date(2026, 5, 1, 14, 32, 0, 0, time.Local)

	path, err := savePromptToTempAt(topologyPromptKind, "prompt", tempRoot, when)
	if err != nil {
		t.Fatalf("savePromptToTempAt() error = %v", err)
	}
	if !strings.HasSuffix(path, filepath.Join("badger_tmp", "prompt-1-topology-2026-05-01-1432.txt")) {
		t.Fatalf("path = %q, want badger_tmp fallback", path)
	}
}

func TestSavePromptToTempAtFailsWhenBothPromptDirsAreFiles(t *testing.T) {
	tempRoot := t.TempDir()
	for _, name := range []string{"badger", "badger_tmp"} {
		if err := os.WriteFile(filepath.Join(tempRoot, name), []byte("binary"), 0600); err != nil {
			t.Fatalf("WriteFile(%s) error = %v", name, err)
		}
	}
	when := time.Date(2026, 5, 1, 14, 32, 0, 0, time.Local)

	_, err := savePromptToTempAt(topologyPromptKind, "prompt", tempRoot, when)
	if err == nil {
		t.Fatal("savePromptToTempAt() error = nil, want directory creation failure")
	}
	if !strings.Contains(err.Error(), "could not create temp prompt directory") {
		t.Fatalf("error = %v, want temp prompt directory failure", err)
	}
}

func TestIsLargePromptUsesStrictThreshold(t *testing.T) {
	if isLargePrompt("12345", 5) {
		t.Fatal("prompt at threshold was treated as large")
	}
	if !isLargePrompt("123456", 5) {
		t.Fatal("prompt above threshold was not treated as large")
	}
}

func TestPromptFileRevealCommandForSupportedPlatforms(t *testing.T) {
	originalGOOS := promptFileRevealGOOS
	originalLookPath := promptFileRevealLookPath
	defer func() {
		promptFileRevealGOOS = originalGOOS
		promptFileRevealLookPath = originalLookPath
	}()

	promptFileRevealLookPath = func(name string) (string, error) {
		return filepath.Join("/bin", name), nil
	}

	tests := []struct {
		name     string
		goos     string
		path     string
		wantName string
		wantArgs []string
	}{
		{
			name:     "macOS reveals selected file",
			goos:     "darwin",
			path:     "/tmp/badger/prompt-1.txt",
			wantName: "open",
			wantArgs: []string{"-R", "/tmp/badger/prompt-1.txt"},
		},
		{
			name:     "Windows reveals selected file",
			goos:     "windows",
			path:     `C:\Temp\badger\prompt-2.txt`,
			wantName: "explorer",
			wantArgs: []string{`/select,C:\Temp\badger\prompt-2.txt`},
		},
		{
			name:     "Linux opens containing directory",
			goos:     "linux",
			path:     "/tmp/badger/prompt-2.txt",
			wantName: "xdg-open",
			wantArgs: []string{"/tmp/badger"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			promptFileRevealGOOS = tt.goos
			got, ok := promptFileRevealCommandFor(tt.path)
			if !ok {
				t.Fatal("promptFileRevealCommandFor() not supported")
			}
			if got.name != tt.wantName || !reflect.DeepEqual(got.args, tt.wantArgs) {
				t.Fatalf("command = %#v, want name=%q args=%#v", got, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestPromptFileRevealUnavailableWhenOpenerMissing(t *testing.T) {
	originalGOOS := promptFileRevealGOOS
	originalLookPath := promptFileRevealLookPath
	defer func() {
		promptFileRevealGOOS = originalGOOS
		promptFileRevealLookPath = originalLookPath
	}()

	promptFileRevealGOOS = "linux"
	promptFileRevealLookPath = func(string) (string, error) {
		return "", errors.New("missing")
	}

	if promptFileRevealAvailable("/tmp/badger/prompt.txt") {
		t.Fatal("promptFileRevealAvailable() = true, want false")
	}
}
