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

func TestGenerateUnreadableSourcePreservesReport(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix permission checks")
	}
	for _, kind := range []string{"root", "file"} {
		t.Run(kind, func(t *testing.T) {
			caller := t.TempDir()
			source := filepath.Join(t.TempDir(), "project")
			if err := os.Mkdir(source, 0o700); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, source, "denied.go", "code\n")
			denied := source
			if kind == "file" {
				denied = filepath.Join(source, "denied.go")
			}
			if err := os.Chmod(denied, 0); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.Chmod(denied, 0o700); err != nil {
					t.Error(err)
				}
			})
			destination := filepath.Join(caller, reportFilename("project"))
			if err := os.WriteFile(destination, []byte("old report"), 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := generateIn(source, caller, scanProject, renderHTML, publishHTML)
			if got != "" || err == nil || !strings.Contains(err.Error(), denied) {
				t.Fatalf("unreadable generation: %q %v", got, err)
			}
			assertOnlyReport(t, caller, destination, "old report")
		})
	}
}

func TestPublicationRejectsSymlinkAndPreservesTarget(t *testing.T) {
	caller := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.html")
	if err := os.WriteFile(target, []byte("untouched target"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(caller, "report.html")
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}
	if err := publishHTML(destination, []byte("new page")); err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("symlink overwrite: %v", err)
	}
	bytes, err := os.ReadFile(target)
	if err != nil || string(bytes) != "untouched target" {
		t.Fatal("symlink target changed")
	}
	info, err := os.Lstat(destination)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink replaced")
	}
	entries, err := os.ReadDir(caller)
	if err != nil || len(entries) != 1 {
		t.Fatal("unexpected staged artifacts")
	}
}

func TestReportPublicationKeepsPrivatePermissions(t *testing.T) {
	caller := t.TempDir()
	target := filepath.Join(caller, "report.html")
	for _, existingMode := range []os.FileMode{0, 0o600, 0o644} {
		if existingMode != 0 {
			if err := os.Chmod(target, existingMode); err != nil {
				t.Fatal(err)
			}
		}
		if err := publishHTML(target, []byte("private report")); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(target)
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("permissions broadened: %v %v", info, err)
		}
		assertOnlyReport(t, caller, target, "private report")
	}
}
