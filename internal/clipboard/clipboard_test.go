package clipboard

import (
	"errors"
	"reflect"
	"testing"
)

func TestNativeCommandDetectsSupportedClipboardTools(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		available   map[string]bool
		wantCommand command
		wantOK      bool
	}{
		{
			name:        "darwin pbcopy",
			goos:        "darwin",
			available:   map[string]bool{"pbcopy": true},
			wantCommand: command{name: "pbcopy", pipeExample: "pbcopy"},
			wantOK:      true,
		},
		{
			name:        "linux xclip",
			goos:        "linux",
			available:   map[string]bool{"xclip": true},
			wantCommand: command{name: "xclip", args: []string{"-selection", "clipboard"}, pipeExample: "xclip -selection clipboard"},
			wantOK:      true,
		},
		{
			name:        "windows clip",
			goos:        "windows",
			available:   map[string]bool{"clip": true},
			wantCommand: command{name: "clip", pipeExample: "clip"},
			wantOK:      true,
		},
		{
			name:      "missing tool",
			goos:      "darwin",
			available: map[string]bool{},
			wantOK:    false,
		},
		{
			name:      "unsupported platform",
			goos:      "plan9",
			available: map[string]bool{"pbcopy": true},
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLookPath(t, func(name string) (string, error) {
				if tt.available[name] {
					return "/usr/bin/" + name, nil
				}
				return "", errors.New("not found")
			})

			got, ok := nativeCommand(tt.goos)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !reflect.DeepEqual(got, tt.wantCommand) {
				t.Fatalf("command = %#v, want %#v", got, tt.wantCommand)
			}
		})
	}
}

func TestPipeCommandUsesDetectedClipboardCommand(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		available map[string]bool
		want      string
		wantOK    bool
	}{
		{
			name:      "darwin pbcopy",
			goos:      "darwin",
			available: map[string]bool{"pbcopy": true},
			want:      "pbcopy",
			wantOK:    true,
		},
		{
			name:      "linux xclip",
			goos:      "linux",
			available: map[string]bool{"xclip": true},
			want:      "xclip -selection clipboard",
			wantOK:    true,
		},
		{
			name:      "windows clip",
			goos:      "windows",
			available: map[string]bool{"clip": true},
			want:      "clip",
			wantOK:    true,
		},
		{
			name:      "missing tool fallback",
			goos:      "darwin",
			available: map[string]bool{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLookPath(t, func(name string) (string, error) {
				if tt.available[name] {
					return "/usr/bin/" + name, nil
				}
				return "", errors.New("not found")
			})

			got, ok := pipeCommand(tt.goos)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("pipe command = %q, want %q", got, tt.want)
			}
		})
	}
}

func withLookPath(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	original := lookPath
	lookPath = fn
	t.Cleanup(func() {
		lookPath = original
	})
}
