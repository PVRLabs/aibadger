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
	oldHome, oldInstall := userHomeDirFunc, installSkillsFunc
	userHomeDirFunc = func() (string, error) { return home, nil }
	t.Cleanup(func() { userHomeDirFunc, installSkillsFunc = oldHome, oldInstall })
	var stdout, stderr bytes.Buffer
	if err := runSkillsCommand([]string{"install"}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
	for _, name := range []string{"handoff", "badger-review"} {
		path := filepath.Join(home, ".agents", "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout.String(), path) {
			t.Fatalf("stdout = %q, want %s", stdout.String(), path)
		}
	}
	for _, want := range []string{
		"Badger Skills are ready.",
		"handoff        Transfer the current session to Badger",
		"badger-review  Prepare recent work for an independent Badger review",
		"The Skill will tell you where to run `badger continue`.",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

func TestRunSkillsCommandHelp(t *testing.T) {
	for _, test := range []struct{ args, want []string }{
		{args: []string{"--help"}, want: []string{"badger skills install", "command details"}},
		{args: []string{"install", "--help"}, want: []string{"~/.agents/skills/", "handoff", "badger-review", "no network"}},
	} {
		var stdout, stderr bytes.Buffer
		if err := runSkillsCommand(test.args, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		for _, want := range test.want {
			if !strings.Contains(stdout.String(), want) {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
		}
		if stderr.Len() != 0 {
			t.Fatalf("stderr = %q", stderr.String())
		}
	}
}

func TestRunSkillsCommandRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{nil, "requires install"},
		{[]string{"list"}, "unknown skills command"},
		{[]string{"install", "extra"}, "does not accept arguments"},
	} {
		var stderr bytes.Buffer
		err := runSkillsCommand(test.args, io.Discard, &stderr)
		if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(stderr.String(), test.want) {
			t.Fatalf("error = %v, stderr = %q, want %q", err, stderr.String(), test.want)
		}
	}
}

func TestRunSkillsCommandReportsFailures(t *testing.T) {
	oldHome, oldInstall := userHomeDirFunc, installSkillsFunc
	t.Cleanup(func() { userHomeDirFunc, installSkillsFunc = oldHome, oldInstall })
	userHomeDirFunc = func() (string, error) { return "", errors.New("home unavailable") }
	var stderr bytes.Buffer
	if err := runSkillsCommand([]string{"install"}, io.Discard, &stderr); err == nil || !strings.Contains(stderr.String(), "resolve user home") {
		t.Fatalf("error = %v, stderr = %q", err, stderr.String())
	}
	userHomeDirFunc = func() (string, error) { return "/home/tester", nil }
	installSkillsFunc = func(string, io.Writer, io.Writer) error { return errors.New("destination unavailable") }
	stderr.Reset()
	if err := runSkillsCommand([]string{"install"}, io.Discard, &stderr); err == nil || !strings.Contains(stderr.String(), "Error: destination unavailable") {
		t.Fatalf("error = %v, stderr = %q", err, stderr.String())
	}
}
