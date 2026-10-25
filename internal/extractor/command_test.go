package extractor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSelectorMarkdownWrappers(t *testing.T) {
	e := NewExtractor("", nil)
	for _, input := range []string{
		"`FILE:src/App.java`", "``FILE:src/App.java``",
		"- FILE:src/App.java", "* FILE:src/App.java", "+ FILE:src/App.java",
		"1. FILE:src/App.java", "12) FILE:src/App.java",
		"> FILE:src/App.java", "> - `FILE:src/App.java`",
		"```text\nFILE:src/App.java\n```",
		"\n````\n\n- `FILE:src/App.java`\n\n````\n",
	} {
		t.Run(input, func(t *testing.T) {
			want := []Command{{Type: "FILE", Path: "src/App.java"}}
			for name, parse := range map[string]func(string) CommandParseResult{
				"regular": e.ParseCommandsDetailed, "strict": e.ParseStrictCommandsDetailed,
			} {
				got := parse(input)
				if len(got.Failures) != 0 || !reflect.DeepEqual(got.Commands, want) {
					t.Errorf("%s = %+v, want %+v without failures", name, got, want)
				}
			}
		})
	}
}

func TestStrictSelectorMarkdownRejectsMixedProseAndMalformedFences(t *testing.T) {
	e := NewExtractor("", nil)
	for _, input := range []string{
		"- Finding: FILE:src/App.java", "> See FILE:src/App.java",
		"```text\nFILE:src/App.java\nThis file needs work.\n```",
		"```\nFILE:src/App.java", "```\nFILE:src/App.java\n````",
		"```\nFILE:src/App.java\n```\nA finding.",
		"```\nPREFIX:src/App.java\n```", "```\n\n```",
	} {
		t.Run(input, func(t *testing.T) {
			if got := e.ParseStrictCommandsDetailed(input); len(got.Failures) == 0 {
				t.Fatalf("strict parse = %+v, want malformed response diagnostics", got)
			}
		})
	}
}

func TestBacktickFilePathsPreferExistingLiteralFiles(t *testing.T) {
	root := t.TempDir()
	e := NewExtractor(root, nil)
	if err := os.WriteFile(filepath.Join(root, "App.java"), []byte("class App {}"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, parse := range []func(string) CommandParseResult{e.ParseCommandsDetailed, e.ParseStrictCommandsDetailed} {
		got := parse("FILE:`App.java`")
		if len(got.Commands) != 1 || got.Commands[0].Path != "App.java" {
			t.Fatalf("parse = %+v, want unwrapped existing path", got)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "`App.java`"), []byte("literal"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, parse := range []func(string) CommandParseResult{e.ParseCommandsDetailed, e.ParseStrictCommandsDetailed} {
		for _, path := range []string{"`App.java`", "`missing.java`"} {
			got := parse("FILE:" + path)
			if len(got.Commands) != 1 || got.Commands[0].Path != path {
				t.Fatalf("parse = %+v, want preserved path %q", got, path)
			}
		}
	}
}

func TestMarkdownWrappersPreservePatternsAndAmbiguousPaths(t *testing.T) {
	e := NewExtractor("", nil)
	for _, input := range []string{
		"PREFIX:src/App.java#literal FILE:other.FILE:example`\\",
		"NEAR:src/App.java#literal FILE:other.FILE:example`\\",
		"FILE:src/App.java.", "FILE:src/App.java:42", "FILE:src/my file.java",
	} {
		want, ok := parseCommandLine(input)
		if !ok {
			t.Fatal("invalid test selector")
		}
		for _, wrapped := range []string{"- " + input, "> " + input, "`" + input + "`"} {
			got := e.ParseStrictCommandsDetailed(wrapped)
			if len(got.Failures) != 0 || len(got.Commands) != 1 || got.Commands[0] != want {
				t.Errorf("parse %q = %+v, want %+v", wrapped, got, want)
			}
		}
	}
}

func TestFileSelectorsWithMarkdownLineBreaks(t *testing.T) {
	e := NewExtractor("", nil)
	input := "FILE:src/ConnectionManager.java\\\nFILE:src/Session.java\\ \t\nFILE:src/ExecutionStatisticsTest.java"
	want := []Command{
		{Type: "FILE", Path: "src/ConnectionManager.java"},
		{Type: "FILE", Path: "src/Session.java"},
		{Type: "FILE", Path: "src/ExecutionStatisticsTest.java"},
	}
	for name, parse := range map[string]func(string) CommandParseResult{
		"detailed": e.ParseCommandsDetailed,
		"strict":   e.ParseStrictCommandsDetailed,
	} {
		t.Run(name, func(t *testing.T) {
			got := parse(input)
			if len(got.Failures) != 0 || !reflect.DeepEqual(got.Commands, want) {
				t.Fatalf("parse = %+v, want commands %+v without failures", got, want)
			}
		})
	}
	if got := e.ParseCommands("AI requests: FILE:src/ConnectionManager.java\\"); !reflect.DeepEqual(got, want[:1]) {
		t.Fatalf("embedded selectors = %+v, want %+v", got, want[:1])
	}
}

func TestFileSelectorLineBreakNormalizationBoundaries(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  Command
		ok    bool
	}{
		{`FILE:src\main.go\`, Command{Type: "FILE", Path: `src\main.go`}, true},
		{`FILE:main.go\\`, Command{Type: "FILE", Path: `main.go\`}, true},
		{`PREFIX:src\main.go\#pattern\`, Command{Type: "PREFIX", Path: `src\main.go\`, Pattern: `pattern\`}, true},
		{`NEAR:src\main.go\#pattern\`, Command{Type: "NEAR", Path: `src\main.go\`, Pattern: `pattern\`}, true},
		{`FILE:\`, Command{}, false},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := parseCommandLine(tt.input)
			if ok != tt.ok || (ok && got != tt.want) {
				t.Fatalf("parseCommandLine = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
