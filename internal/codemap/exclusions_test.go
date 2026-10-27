package codemap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/pathmatch"
)

func reportData(t *testing.T, page []byte) directory {
	t.Helper()
	_, data, ok := strings.Cut(string(page), `<script id="map-data" type="application/json">`)
	if !ok {
		t.Fatal("missing report data")
	}
	data, _, _ = strings.Cut(data, "</script>")
	var root directory
	if err := json.Unmarshal([]byte(data), &root); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestExclusionsPruneBeforeLimitsAndReading(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "main.go", "package main\n\n// comment")
	writeTestFile(t, root, "public/assets/deep/generated.js", strings.Repeat("x", 100))
	writeTestFile(t, root, "huge.ps1", strings.Repeat("x", 100))
	writeTestFile(t, root, "node_modules/deep/app.js", "ignored")
	patterns := []string{"public", "*.ps1", "unmatched", "public"}
	reads := []string{}
	readDir := func(dir *os.File, n int) ([]os.DirEntry, error) {
		reads = append(reads, dir.Name())
		return dir.ReadDir(n)
	}
	got, err := scanProjectWithExclusions(root, scanLimits{0, 1, 1, 30}, readDir, "", patterns)
	if err != nil {
		t.Fatal(err)
	}
	if got.Files != 1 || got.Lines != 2 || got.Languages["Go"] != (languageStats{Lines: 2, Files: 1}) || len(reads) != 2 {
		t.Fatalf("tree=%+v directory reads=%v", got, reads)
	}
	// Root-anchored patterns preserve nested near matches; basename matches prune
	// those same paths wherever they occur.
	writeTestFile(t, root, "nested/public/assets/app.js", "source")
	got, err = scanProjectWithExclusions(root, scanLimits{5, 10, 10, 30}, (*os.File).ReadDir, "", []string{"public/assets/**", "*.ps1"})
	if err != nil || got.Files != 2 || got.Languages["JavaScript"].Files != 1 {
		t.Fatalf("root anchored: %+v %v", got, err)
	}
	// Validation must precede even opening a missing source.
	_, err = scanProjectWithExclusions(filepath.Join(root, "missing"), scanLimits{}, (*os.File).ReadDir, "", []string{"C:foo"})
	if err == nil || !strings.Contains(err.Error(), "invalid --exclude") {
		t.Fatalf("validation: %v", err)
	}
}

func TestRegenerationMetadataPrivacyAndQuoting(t *testing.T) {
	patterns := []string{"vendor", " a'b $();&{} ", "**/*.min.js", "vendor", "</script>", "-secret", "日本 語", "`nope`"}
	if err := pathmatch.Validate(patterns); err != nil {
		t.Fatal(err)
	}
	for _, goos := range []string{"linux", "windows"} {
		for _, excluded := range []bool{false, true} {
			for _, hidden := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/excluded=%v/hidden=%v", goos, excluded, hidden), func(t *testing.T) {
					opts := Options{Version: "v-test", NoReproCommand: hidden}
					if excluded {
						opts.ExcludePatterns = patterns
					}
					root := reportFixture()
					root.GeneratorVersion = opts.Version
					root.Reproduction = reproductionFor(opts, goos)
					page, err := renderHTML(root)
					if err != nil {
						t.Fatal(err)
					}
					decoded := reportData(t, page)
					present := excluded && !hidden
					if strings.Contains(string(page), `aria-controls="regeneration-panel"`) != present ||
						strings.Contains(string(page), `<button id="regeneration"`) != present ||
						strings.Contains(string(page), `"reproduction":`) != present {
						t.Fatalf("incorrect visibility, excluded=%v hidden=%v", excluded, hidden)
					}
					if decoded.GeneratorVersion != "v-test" {
						t.Fatal("missing generator version")
					}
					if !present {
						if decoded.Reproduction != nil || strings.Contains(string(page), "--exclude=") || strings.Contains(string(page), "a'b") {
							t.Fatal("private settings or command leaked")
						}
						return
					}
					control := strings.Index(string(page), `<button id="regeneration"`)
					levels := strings.Index(string(page), `<label class="depth-label"`)
					crumbs := strings.Index(string(page), `<nav id="crumbs"`)
					main := strings.Index(string(page), "<main>")
					if !(crumbs < control && control < levels && levels < main) || !strings.Contains(string(page), `aria-label="Regeneration instructions" hidden>`) {
						t.Fatal("exclusion control must be collapsed after the breadcrumb and before Visible levels")
					}
					settings := decoded.Reproduction
					if settings == nil || !reflect.DeepEqual(settings.Patterns, patterns) {
						t.Fatalf("patterns changed: %+v", settings)
					}
					wantShell, wantQuote := "POSIX", `--exclude=' a'"'"'b $();&{} '`
					if goos == "windows" {
						wantShell, wantQuote = "PowerShell", `--exclude=' a''b $();&{} '`
					}
					if settings.Shell != wantShell || !strings.HasPrefix(settings.Command, "badger map . --exclude='vendor'") ||
						!strings.Contains(settings.Command, wantQuote) || strings.Count(settings.Command, "--exclude='vendor'") != 2 ||
						!strings.Contains(settings.Command, "--exclude='-secret'") {
						t.Fatalf("command/shell: %+v", settings)
					}
					if strings.Contains(string(page), "</script> --exclude") {
						t.Fatal("unsafe or expanded report")
					}
					if !strings.Contains(string(page), `\u003c/script\u003e`) {
						t.Fatal("script-shaped pattern not JSON escaped")
					}
					exact, err := renderHTMLTemplate(root, pageTemplate, len(page))
					if err != nil || len(exact) != len(page) {
						t.Fatalf("exact metadata budget: %v", err)
					}
					if _, err = renderHTMLTemplate(root, pageTemplate, len(page)-1); err == nil {
						t.Fatal("metadata bypassed size budget")
					}
				})
			}
		}
	}
}

func TestExclusionGenerationFromMappedDirectory(t *testing.T) {
	source, caller := t.TempDir(), t.TempDir()
	writeTestFile(t, source, "sub/main.go", "package main\n")
	writeTestFile(t, source, "sub/secret/data.go", "excluded\n")
	writeTestFile(t, source, "other.go", "outside supplied target\n")
	t.Chdir(caller)
	target := filepath.Join(source, "sub")
	opts := Options{ExcludePatterns: []string{"secret"}, Version: "v-test"}
	path, err := GenerateWithOptions(target, opts)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	root := reportData(t, original)
	if root.Files != 1 || root.Lines != 1 || root.Reproduction.Command != "badger map . --exclude='secret'" ||
		strings.Contains(string(original), source) || strings.Contains(string(original), caller) {
		t.Fatalf("target leakage or accounting: %+v", root)
	}
	// Relative, absolute and dot targets regenerate from the same mapped
	// directory without discovering an enclosing repository.
	t.Chdir(target)
	for _, arg := range []string{".", filepath.Join("..", "sub"), target} {
		replay, err := GenerateWithOptions(arg, opts)
		if err != nil {
			t.Fatal(err)
		}
		page, err := os.ReadFile(replay)
		if err != nil {
			t.Fatal(err)
		}
		got := reportData(t, page)
		if got.Files != root.Files || got.Lines != root.Lines || got.Reproduction.Command != root.Reproduction.Command {
			t.Fatalf("replay: %+v", got)
		}
	}
	// Invalid and all-excluded attempts preserve previously published reports.
	destination := filepath.Join(target, reportFilename("sub"))
	previous, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	for _, patterns := range [][]string{{"C:foo"}, {"**"}} {
		if _, err := GenerateWithOptions(".", Options{ExcludePatterns: patterns}); err == nil {
			t.Fatal("invalid/all-excluded generation succeeded")
		}
		after, err := os.ReadFile(destination)
		if err != nil || string(after) != string(previous) {
			t.Fatalf("prior report changed: %v", err)
		}
	}
	opts.NoReproCommand = true
	path, err = GenerateWithOptions(".", opts)
	if err != nil {
		t.Fatal(err)
	}
	hidden, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	private := reportData(t, hidden)
	if private.Files != 1 || private.Lines != 1 || private.Reproduction != nil || strings.Contains(string(hidden), "--exclude=") {
		t.Fatalf("opt-out changed filtering or leaked command: %+v", private)
	}
}

func TestNestedCallerReplayBoundary(t *testing.T) {
	source := t.TempDir()
	writeTestFile(t, source, "main.go", "package main\n")
	nested := filepath.Join(source, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	opts := Options{ExcludePatterns: []string{"unmatched"}, Version: "v-test"}
	path, err := GenerateWithOptions("..", opts)
	if err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	initial := reportData(t, first)
	t.Chdir(source)
	path, err = GenerateWithOptions(".", opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	replay := reportData(t, second)
	if initial.Files != 1 || replay.Files != 2 || replay.Lines <= initial.Lines {
		t.Fatalf("nested stale report boundary: first=%+v replay=%+v", initial, replay)
	}
	// Repeating from the same root omits the current destination again.
	path, err = GenerateWithOptions(".", opts)
	if err != nil {
		t.Fatal(err)
	}
	third, err := os.ReadFile(path)
	if err != nil || string(third) != string(second) {
		t.Fatalf("same-root regeneration changed: %v", err)
	}
	// Explicitly excluding the stale report restores the source-only totals.
	opts.ExcludePatterns = append(opts.ExcludePatterns, "nested/*.html")
	path, err = GenerateWithOptions(".", opts)
	if err != nil {
		t.Fatal(err)
	}
	final, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reportData(t, final); got.Files != 1 || got.Lines != 1 {
		t.Fatalf("stale report exclusion: %+v", got)
	}
}
