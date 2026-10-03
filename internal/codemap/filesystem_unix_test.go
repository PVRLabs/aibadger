//go:build darwin || linux

package codemap

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNonRegularEntriesDoNotConsumeQuotas(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "pipe.go")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	// Opening this FIFO would block; it must be rejected from entry metadata.
	got, err := scanProjectWithLimits(root, scanLimits{0, 1, 0, 0})
	if err != nil || got == nil || got.Files != 0 {
		t.Fatalf("FIFO scan: %v %v", got, err)
	}
}

func TestUnreadableInputsFailWholeScan(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix permission checks")
	}
	for _, kind := range []string{"file", "directory", "root"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := root
			if kind == "file" {
				path = filepath.Join(root, "denied.go")
				writeTestFile(t, root, "denied.go", "code\n")
			} else if kind == "directory" {
				path = filepath.Join(root, "denied")
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Chmod(path, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(path, 0o700); err != nil {
					t.Error(err)
				}
			})
			got, err := scanProject(root)
			if got != nil || err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("unreadable scan: %v %v", got, err)
			}
		})
	}
}
