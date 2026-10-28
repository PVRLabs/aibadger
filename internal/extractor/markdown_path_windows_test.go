package extractor

import (
	"os"
	"testing"
)

func TestFileMarkdownIssue171NativeWindowsInput(t *testing.T) {
	data, err := os.ReadFile("testdata/issue171-selectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the original clipboard bytes, including native separators,
	// Markdown escapes, and the fixture's final line ending.
	testFileMarkdownIssue171(t, string(data))
}
