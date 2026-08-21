package handoff

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConsumeValidHandoff(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		for _, mode := range []Mode{ModeReview, ModeDesign, ModeHandoff} {
			t.Run(string(mode)+newlineName(newline), func(t *testing.T) {
				root := t.TempDir()
				body := "  preserve this context\r\nincluding its final newline\n"
				writeHandoff(t, root, []byte("BADGER-HANDOFF-V1"+newline+"mode: "+string(mode)+newline+newline+body))

				got, err := Consume(root)
				if err != nil {
					t.Fatalf("Consume() error = %v", err)
				}
				if got.Mode != mode || got.Body != body {
					t.Fatalf("Consume() = %#v, want mode %q and body %q", got, mode, body)
				}
				if _, err := os.Stat(filepath.Join(root, Filename)); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("consumed handoff still exists: %v", err)
				}
			})
		}
	}
}

func TestConsumeRejectsInvalidHandoffWithoutRemovingIt(t *testing.T) {
	validPrefix := "BADGER-HANDOFF-V1\nmode: review\n\n"
	tests := map[string][]byte{
		"missing magic":   []byte("mode: review\n\nbody"),
		"wrong version":   []byte("BADGER-HANDOFF-V2\nmode: review\n\nbody"),
		"invalid mode":    []byte("BADGER-HANDOFF-V1\nmode: code\n\nbody"),
		"missing body":    []byte(validPrefix),
		"blank body":      []byte(validPrefix + " \r\n\t"),
		"mixed framing":   []byte("BADGER-HANDOFF-V1\r\nmode: review\n\r\nbody"),
		"byte order mark": []byte("\xef\xbb\xbf" + validPrefix + "body"),
		"invalid UTF-8":   append([]byte(validPrefix), 0xff),
	}

	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeHandoff(t, root, data)

			if _, err := Consume(root); err == nil {
				t.Fatal("Consume() error = nil, want validation error")
			}
			if _, err := os.Stat(filepath.Join(root, Filename)); err != nil {
				t.Fatalf("invalid handoff was removed: %v", err)
			}
		})
	}
}

func TestConsumeFileSizeLimit(t *testing.T) {
	prefix := "BADGER-HANDOFF-V1\nmode: handoff\n\n"
	tests := []struct {
		name string
		size int
	}{
		{name: "at limit", size: MaxFileBytes},
		{name: "above limit", size: MaxFileBytes + 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeHandoff(t, root, []byte(prefix+strings.Repeat("x", test.size-len(prefix))))

			_, err := Consume(root)
			if test.size == MaxFileBytes && err != nil {
				t.Fatalf("Consume() error at limit = %v", err)
			}
			if test.size > MaxFileBytes && err == nil {
				t.Fatal("Consume() error above limit = nil")
			}
		})
	}
}

func TestConsumeRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, Filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Consume(root); err == nil {
		t.Fatal("Consume() error = nil, want non-regular-file error")
	}
}

func writeHandoff(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, Filename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func newlineName(newline string) string {
	if newline == "\n" {
		return "/lf"
	}
	return "/crlf"
}
