//go:build linux

package downloads

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeDownloadsDirectoryIgnoresRelativeXDGConfigHome(t *testing.T) {
	home := t.TempDir()
	workingDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "Downloads"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workingDir, ".config"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"$HOME/Downloads\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workingDir, ".config", "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"/unrelated/project/Downloads\"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", ".config")
	t.Chdir(workingDir)

	got, err := nativeDownloadsDirectory()
	if err != nil {
		t.Fatalf("nativeDownloadsDirectory() error = %v", err)
	}
	if want := filepath.Join(home, "Downloads"); got != want {
		t.Fatalf("nativeDownloadsDirectory() = %q, want %q", got, want)
	}
}

func TestParseXDGDownloadsConfig(t *testing.T) {
	got, err := parseXDGDownloadsConfig(strings.NewReader("XDG_DESKTOP_DIR=\"$HOME/Desktop\"\nXDG_DOWNLOAD_DIR=\"$HOME/Downloads\"\n"), "/home/tester")
	if err != nil {
		t.Fatalf("parseXDGDownloadsConfig() error = %v", err)
	}
	if want := "/home/tester/Downloads"; got != want {
		t.Fatalf("parseXDGDownloadsConfig() = %q, want %q", got, want)
	}
}

func TestParseXDGDownloadsConfigRequiresEntry(t *testing.T) {
	_, err := parseXDGDownloadsConfig(strings.NewReader("XDG_DESKTOP_DIR=\"$HOME/Desktop\"\n"), "/home/tester")
	if err == nil {
		t.Fatal("parseXDGDownloadsConfig() error = nil")
	}
}
