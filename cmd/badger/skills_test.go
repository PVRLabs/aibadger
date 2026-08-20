package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunSkillsCommandInstallsIntoResolvedHome(t *testing.T) {
	home := t.TempDir()
	oldHome := userHomeDirFunc
	oldInstall := installSkillsFunc
	userHomeDirFunc = func() (string, error) { return home, nil }
	installSkillsFunc = oldInstall
	t.Cleanup(func() {
		userHomeDirFunc = oldHome
		installSkillsFunc = oldInstall
	})

	var stdout, stderr bytes.Buffer
	if err := runSkillsCommand([]string{"install"}, &stdout, &stderr); err != nil {
		t.Fatalf("runSkillsCommand() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, name := range []string{"badger-review", "badger-handoff"} {
		path := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("Stat(%s) error = %v", path, err)
		}
		if !strings.Contains(stdout.String(), path) {
			t.Fatalf("stdout = %q, want destination %s", stdout.String(), path)
		}
	}
}

func TestRunSkillsCommandHelp(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want []string
	}{
		{name: "skills help", args: []string{"--help"}, want: []string{"badger skills install", "command details"}},
		{name: "install help", args: []string{"install", "--help"}, want: []string{"~/.agents/skills/", "badger-review", "badger-handoff", "no network"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := runSkillsCommand(test.args, &stdout, &stderr); err != nil {
				t.Fatalf("runSkillsCommand() error = %v", err)
			}
			for _, want := range test.want {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestRunSkillsCommandRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing command", args: nil, want: "requires install"},
		{name: "unknown command", args: []string{"list"}, want: "unknown skills command: list"},
		{name: "extra argument", args: []string{"install", "extra"}, want: "does not accept arguments: extra"},
		{name: "flag", args: []string{"install", "--root", "/tmp"}, want: "does not accept arguments: --root"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := runSkillsCommand(test.args, &stdout, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want %q", stderr.String(), test.want)
			}
		})
	}
}

func TestRunSkillsCommandReportsHomeResolutionFailure(t *testing.T) {
	oldHome := userHomeDirFunc
	oldInstall := installSkillsFunc
	userHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
	installSkillsFunc = func(string, io.Writer, io.Writer) error {
		t.Fatal("installer called after home resolution failure")
		return nil
	}
	t.Cleanup(func() {
		userHomeDirFunc = oldHome
		installSkillsFunc = oldInstall
	})

	var stdout, stderr bytes.Buffer
	err := runSkillsCommand([]string{"install"}, &stdout, &stderr)
	if err == nil || !strings.Contains(err.Error(), "home unavailable") {
		t.Fatalf("error = %v, want home resolution error", err)
	}
	if !strings.Contains(stderr.String(), "resolve user home") {
		t.Fatalf("stderr = %q, want home resolution diagnostic", stderr.String())
	}
}

func TestRunSkillsCommandReportsInstallerFailure(t *testing.T) {
	oldHome := userHomeDirFunc
	oldInstall := installSkillsFunc
	userHomeDirFunc = func() (string, error) { return "/home/tester", nil }
	installSkillsFunc = func(root string, stdout, stderr io.Writer) error {
		if root != filepath.Join("/home/tester", ".agents", "skills") {
			t.Fatalf("root = %q, want resolved skills root", root)
		}
		return errors.New("destination unavailable")
	}
	t.Cleanup(func() {
		userHomeDirFunc = oldHome
		installSkillsFunc = oldInstall
	})

	var stdout, stderr bytes.Buffer
	err := runSkillsCommand([]string{"install"}, &stdout, &stderr)
	if err == nil || err.Error() != "destination unavailable" {
		t.Fatalf("error = %v, want installer error", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "Error: destination unavailable") {
		t.Fatalf("stderr = %q, want installer diagnostic", stderr.String())
	}
}
