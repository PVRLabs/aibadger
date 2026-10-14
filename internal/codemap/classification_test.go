package codemap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Model a filesystem whose directory entries do not expose file type bits.
type unknownTypeEntry struct {
	os.DirEntry
	info os.FileInfo
	err  error
}

func (e unknownTypeEntry) Type() os.FileMode          { return 0 }
func (e unknownTypeEntry) IsDir() bool                { return false }
func (e unknownTypeEntry) Info() (os.FileInfo, error) { return e.info, e.err }

type fileInfoWithMode struct {
	os.FileInfo
	mode os.FileMode
}

func (i fileInfoWithMode) Mode() os.FileMode { return i.mode }

func TestClassificationResolvesUnknownEntryTypes(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "candidate.GO", "code\n")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		mode os.FileMode
		want string
	}{
		{"regular", 0o644, "Go"},
		{"FIFO", os.ModeNamedPipe | 0o600, ""},
		{"socket", os.ModeSocket | 0o600, ""},
		{"device", os.ModeDevice | 0o600, ""},
		{"character device", os.ModeDevice | os.ModeCharDevice | 0o600, ""},
		{"symlink", os.ModeSymlink | 0o777, ""},
		{"directory", os.ModeDir | 0o755, ""},
		{"irregular", os.ModeIrregular, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			entry := unknownTypeEntry{DirEntry: entries[0], info: fileInfoWithMode{FileInfo: info, mode: tt.mode}}
			got, err := classifyEntry(entry)
			if err != nil || (got.language != tt.want || got.isDir != tt.mode.IsDir()) {
				t.Fatalf("classification = %+v, %v; want language %q", got, err, tt.want)
			}
		})
	}
	failure := errors.New("metadata unavailable")
	got, err := classifyEntry(unknownTypeEntry{DirEntry: entries[0], err: failure})
	if got.language != "" || !errors.Is(err, failure) {
		t.Fatalf("metadata failure: %+v, %v", got, err)
	}
}

func TestUnknownTypeDirectoriesAreTraversed(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "src/nested/main.go", "code\n")
	// A directory can itself have a recognized source extension.
	writeTestFile(t, root, "folder.go/child.py", "code\n")
	writeTestFile(t, root, "src/node_modules/deep/ignored.go", "ignored\n")
	readUnknown := func(dir *os.File, n int) ([]os.DirEntry, error) {
		entries, err := dir.ReadDir(n)
		for i, entry := range entries {
			info, infoErr := entry.Info()
			entries[i] = unknownTypeEntry{DirEntry: entry, info: info, err: infoErr}
		}
		return entries, err
	}
	limits := scanLimits{2, 4, 2, 5}
	got, err := scanProjectWithReadDir(root, limits, readUnknown)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 2 || got.Lines != 2 || len(got.Children) != 2 {
		t.Fatalf("subtree omitted: %#v", got)
	}
	if got.Children[1].Path != "src" || got.Children[1].Children[0].Path != "src/nested" {
		t.Fatalf("nested directories missing: %#v", got.Children)
	}
	for _, tt := range []struct {
		limits scanLimits
		want   string
	}{
		{scanLimits{1, 4, 2, 5}, "directory depth limit (1)"},
		{scanLimits{2, 3, 2, 5}, "directory limit (3)"},
	} {
		got, err := scanProjectWithReadDir(root, tt.limits, readUnknown)
		if got != nil || err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Fatalf("directory quotas: %v, %v", got, err)
		}
	}
}

func TestUnknownTypeMetadataFailureStopsScan(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "unknown.txt", "ignored\n")
	failure := errors.New("metadata unavailable")
	readBroken := func(dir *os.File, n int) ([]os.DirEntry, error) {
		entries, err := dir.ReadDir(n)
		for i, entry := range entries {
			entries[i] = unknownTypeEntry{DirEntry: entry, err: failure}
		}
		return entries, err
	}
	got, err := scanProjectWithReadDir(root, scanLimits{0, 1, 0, 0}, readBroken)
	if got != nil || !errors.Is(err, failure) || !strings.Contains(err.Error(), filepath.Join(root, "unknown.txt")) {
		t.Fatalf("unresolved entry must not silently omit a possible directory: %v, %v", got, err)
	}
}
