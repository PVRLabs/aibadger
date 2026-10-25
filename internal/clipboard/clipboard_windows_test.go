package clipboard

import (
	nativeclipboard "github.com/atotto/clipboard"
	"testing"
)

// This test uses the real Windows clipboard and must not run in parallel.
// It overwrites existing clipboard contents. Cleanup attempts to restore text
// only when the initial read succeeds; non-text formats are not preserved, and
// restoration can fail. Run on CI or with disposable clipboard contents.
func TestWindowsUnicodeClipboardRoundTrip(t *testing.T) {
	original, readErr := nativeclipboard.ReadAll()
	if readErr == nil {
		t.Cleanup(func() { _ = nativeclipboard.WriteAll(original) })
	}
	for _, text := range []string{"plain ASCII prompt", "→ “prompt” ç ğ ı İ ö ş ü 😀"} {
		if err := Copy(text); err != nil {
			t.Fatalf("Copy: %v", err)
		}
		got, err := nativeclipboard.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if got != text {
			t.Fatalf("clipboard = %q, want %q", got, text)
		}
	}
}
