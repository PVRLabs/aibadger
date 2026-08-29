package downloads

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDownloadsDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveDownloadsDirectory(func() (string, error) { return dir, nil }, os.Stat)
	if err != nil {
		t.Fatalf("resolveDownloadsDirectory() error = %v", err)
	}
	if got != dir {
		t.Fatalf("resolveDownloadsDirectory() = %q, want %q", got, dir)
	}
}

func TestResolveDownloadsDirectoryFailures(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	file := filepath.Join(t.TempDir(), "Downloads")
	if err := os.WriteFile(file, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		pathErr error
		want    string
	}{
		{name: "resolver error", pathErr: errors.New("native failure"), want: "native failure"},
		{name: "empty path", want: "empty path"},
		{name: "relative path", path: "Downloads", want: "non-absolute path"},
		{name: "missing path", path: missing, want: "no such file or directory"},
		{name: "path is file", path: file, want: "not a directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := resolveDownloadsDirectory(func() (string, error) { return tt.path, tt.pathErr }, os.Stat)
			if err == nil {
				t.Fatal("resolveDownloadsDirectory() error = nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestResolveDownloadsDirectoryDoesNotCreateMissingPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := resolveDownloadsDirectory(func() (string, error) { return missing, nil }, os.Stat)
	if err == nil {
		t.Fatal("resolveDownloadsDirectory() error = nil")
	}
	if _, statErr := os.Stat(missing); !os.IsNotExist(statErr) {
		t.Fatalf("missing path stat error = %v, want not exist", statErr)
	}
}
