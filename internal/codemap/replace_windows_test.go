//go:build windows

package codemap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsReplacementRejectsDifferentDirectory(t *testing.T) {
	sourceDir, targetDir := t.TempDir(), t.TempDir()
	source := filepath.Join(sourceDir, "staged.tmp")
	target := filepath.Join(targetDir, "report.html")
	if err := os.WriteFile(source, []byte("new report"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old report"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := replaceReport(source, target); err == nil || !strings.Contains(err.Error(), "same directory") {
		t.Fatalf("cross-directory replacement: %v", err)
	}
	assertOnlyReport(t, targetDir, target, "old report")
}

func TestWindowsLockedDestinationPreservesReportAndCleansStage(t *testing.T) {
	caller := t.TempDir()
	target := filepath.Join(caller, "report.html")
	if err := os.WriteFile(target, []byte("old report"), 0o600); err != nil {
		t.Fatal(err)
	}
	// os.Open does not grant FILE_SHARE_DELETE, so native replacement must fail.
	held, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if err := publishHTML(target, []byte("new report")); err == nil {
		t.Fatal("locked destination replaced")
	}
	assertOnlyReport(t, caller, target, "old report")
}

func TestWindowsNativeReplacementSucceeds(t *testing.T) {
	caller := t.TempDir()
	target := filepath.Join(caller, "日本語 report.html")
	if err := os.WriteFile(target, []byte("old report"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := os.CreateTemp(caller, ".badger-map-*.tmp")
	if err != nil {
		t.Fatal(err)
	}
	stagePath := stage.Name()
	t.Cleanup(func() { _ = stage.Close(); _ = os.Remove(stagePath) })
	if _, err := stage.Write([]byte("complete new report")); err != nil {
		t.Fatal(err)
	}
	if err := stage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := replaceReport(stagePath, target); err != nil {
		t.Fatalf("native replacement failed: %v", err)
	}
	assertOnlyReport(t, caller, target, "complete new report")
	if _, err := os.Lstat(stagePath); !os.IsNotExist(err) {
		t.Fatalf("staging path remains: %v", err)
	}
	// Also cover the real publication path on a second replacement.
	if err := publishHTML(target, []byte("second complete report")); err != nil {
		t.Fatal(err)
	}
	assertOnlyReport(t, caller, target, "second complete report")
}
