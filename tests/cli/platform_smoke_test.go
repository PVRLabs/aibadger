package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// TestCLIPlatformSmoke uses a supplied binary and a tiny non-Git directory.
// Keep this separate from the broader, opt-in CLI integration suite.
func TestCLIPlatformSmoke(t *testing.T) {
	bin := os.Getenv("BADGER_PLATFORM_SMOKE_BIN")
	if bin == "" {
		t.Skip("set BADGER_PLATFORM_SMOKE_BIN to a built Badger executable")
	}
	bin, err := filepath.Abs(bin)
	if err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "tiny source")
	if err := os.Mkdir(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	source := []byte("package tiny\n\n// smoke\n")
	sourcePath := filepath.Join(fixture, "tiny.go")
	if err := os.WriteFile(sourcePath, source, 0o600); err != nil {
		t.Fatal(err)
	}
	invoke := func(args ...string) string {
		t.Helper()
		stdout, stderr := run(t, commandTimeout, fixture, os.Environ(), "platform smoke", bin, args...)
		if stderr != "" {
			t.Fatalf("unexpected stderr: %s", stderr)
		}
		return stdout
	}
	for _, args := range [][]string{{"version"}, {"--version"}} {
		assertContains(t, "version", invoke(args...), "badger v")
	}
	for _, args := range [][]string{{"--help"}, {"map", "--help"}, {"api", "--help"}, {"api", "topology", "--help"}} {
		assertContains(t, "help", invoke(args...), "badger")
	}
	var first []byte
	for generation := 0; generation < 3; generation++ {
		args := []string{"map"}
		if generation == 1 {
			args = append(args, ".")
		}
		output := invoke(args...)
		assertContains(t, "map success", output, "Generated codebase map HTML report.", "Use the following command to open:")
		reports, err := filepath.Glob(filepath.Join(fixture, "badger-codemap-*.html"))
		if err != nil || len(reports) != 1 {
			t.Fatalf("expected one report: %v, %v", reports, err)
		}
		reportPath, err := filepath.Abs(reports[0])
		if err != nil {
			t.Fatal(err)
		}
		command := "xdg-open '" + strings.ReplaceAll(reportPath, "'", "'\"'\"'") + "'"
		switch runtime.GOOS {
		case "windows":
			command = "PowerShell:\nInvoke-Item -LiteralPath '" + strings.ReplaceAll(reportPath, "'", "''") + "'"
		case "darwin":
			command = "open -- '" + strings.ReplaceAll(reportPath, "'", "'\"'\"'") + "'"
		}
		assertContains(t, "report opening command", output, "Use the following command to open:\n"+command+"\n")
		report, err := os.ReadFile(reports[0])
		if err != nil {
			t.Fatal(err)
		}
		match := regexp.MustCompile(`(?s)<script id="map-data" type="application/json">(.*?)</script>`).FindSubmatch(report)
		var totals struct{ Lines, Files int }
		if len(match) != 2 {
			t.Fatal("report metadata missing")
		}
		if err := json.Unmarshal(match[1], &totals); err != nil {
			t.Fatal(err)
		}
		if totals.Lines != 2 || totals.Files != 1 {
			t.Fatalf("unexpected totals: %+v", totals)
		}
		if generation == 0 {
			first = report
			// Force an observable replacement, then verify stable regeneration.
			if err := os.WriteFile(reports[0], []byte("old report"), 0o600); err != nil {
				t.Fatal(err)
			}
		} else if !bytes.Equal(first, report) {
			t.Fatal("replacement changed report bytes")
		}
	}
	got, err := os.ReadFile(sourcePath)
	if err != nil || !bytes.Equal(got, source) {
		t.Fatalf("source changed: %v", err)
	}
	entries, err := os.ReadDir(fixture)
	if err != nil || len(entries) != 2 {
		t.Fatalf("unexpected output or staging residue: %v, %v", entries, err)
	}
}
