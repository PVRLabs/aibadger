package codemap

import (
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PVRLabs/aibadger/internal/pathmatch"
)

func TestPowerShellSmartQuoteEscaping(t *testing.T) {
	for _, tc := range []struct{ pattern, quoted string }{
		{"don't", "'don''t'"},
		{"team‘s", "'team‘‘s'"},
		{"team’s", "'team’’s'"},
		{"team‚s", "'team‚‚s'"},
		{"team‛s", "'team‛‛s'"},
		{"'‘’‚‛", "'''‘‘’’‚‚‛‛'"},
		{"x’; throw 'injected'; #", "'x’’; throw ''injected''; #'"},
		{"“quoted” $(); &", "'“quoted” $(); &'"},
	} {
		if err := pathmatch.Validate([]string{tc.pattern}); err != nil {
			t.Fatal(err)
		}
		settings := reproductionFor(Options{ExcludePatterns: []string{tc.pattern}}, "windows")
		if want := "badger map . --exclude=" + tc.quoted; settings.Command != want {
			t.Errorf("%q: got %q want %q", tc.pattern, settings.Command, want)
		}
		if settings.Patterns[0] != tc.pattern {
			t.Fatal("original pattern changed")
		}
	}
}

// Use a harmless badger function to capture the generated command's arguments.
// A quoting regression either changes the round-trip values, adds output, or
// executes a throw sentinel. No real Badger binary or filesystem scan is used.
func TestPowerShellReproductionRoundTrip(t *testing.T) {
	shell, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed; run this test on a PowerShell-equipped host")
	}
	patterns := []string{"don't", "team‘s", "team’s", "team‚s", "team‛s", "'‘’‚‛",
		"x‘; throw 'injected'; #", "x’; throw 'injected'; #",
		"x‚; throw 'injected'; #", "x‛; throw 'injected'; #",
		"日本 語 “double” „quotes“", "-private", "duplicates", "duplicates",
		"$name $(throw 'injected') &; " + string(rune(96)) + "nope",
	}
	if err := pathmatch.Validate(patterns); err != nil {
		t.Fatal(err)
	}
	settings := reproductionFor(Options{ExcludePatterns: patterns}, "windows")
	script := `$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
function badger { [Console]::WriteLine((ConvertTo-Json -InputObject @($args) -Compress)) }
` + settings.Command
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, shell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script).CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell round-trip: %v\n%s", err, output)
	}
	var got []string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(string(output), "\ufeff")), &got); err != nil {
		t.Fatalf("unexpected PowerShell output: %v\n%s", err, output)
	}
	want := []string{"map", "."}
	for _, pattern := range patterns {
		want = append(want, "--exclude="+pattern)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("arguments changed:\ngot  %q\nwant %q", got, want)
	}
}
