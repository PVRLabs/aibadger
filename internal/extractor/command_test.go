package extractor

import (
	"reflect"
	"testing"
)

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
