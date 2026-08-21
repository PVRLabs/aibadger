package skillsinstall

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/skills"
)

func TestInstallWritesExactDefinitionsInDeterministicOrder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "agents", "skills")
	var stdout, stderr bytes.Buffer
	if err := Install(root, skills.Definitions(), &stdout, &stderr); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	for _, definition := range skills.Definitions() {
		path := filepath.Join(root, definition.Name, "SKILL.md")
		assertFileContents(t, path, definition.Content)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != installFileMode {
			t.Errorf("%s mode = %o, want %o", path, info.Mode().Perm(), installFileMode)
		}
	}
	want := "installed handoff: " + filepath.Join(root, "handoff", "SKILL.md") + "\n" +
		"installed badger-review: " + filepath.Join(root, "badger-review", "SKILL.md") + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestInstallUpdatesAndPreservesUnrelatedContent(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := Install(root, skills.Definitions(), io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "other-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(other), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(root, "handoff", "notes.txt")
	if err := os.WriteFile(extra, []byte("keep extra\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "handoff", "SKILL.md"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Install(root, skills.Definitions(), &stdout, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(stdout.String(), "updated handoff: ") || !strings.Contains(stdout.String(), "updated badger-review: ") {
		t.Fatalf("stdout = %q, want deterministic updated reports", stdout.String())
	}
	assertFileContents(t, other, "keep\n")
	assertFileContents(t, extra, "keep extra\n")
	assertFileContents(t, filepath.Join(root, "handoff", "SKILL.md"), skills.Definitions()[0].Content)
}

func TestInstallAcceptsSymlinkedSharedParent(t *testing.T) {
	realSkills := filepath.Join(t.TempDir(), "real-skills")
	if err := os.MkdirAll(realSkills, 0o700); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".agents"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realSkills, filepath.Join(home, ".agents", "skills")); err != nil {
		t.Fatal(err)
	}
	if err := Install(filepath.Join(home, ".agents", "skills"), skills.Definitions(), io.Discard, io.Discard); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
}

func TestInstallRejectsSymlinkedBadgerOwnedTargets(t *testing.T) {
	for _, test := range []struct {
		name, want string
		setup      func(root string) string
	}{
		{name: "skill directory", want: "skill directory is a symlink", setup: func(root string) string {
			if err := os.Symlink(t.TempDir(), filepath.Join(root, "handoff")); err != nil {
				t.Fatal(err)
			}
			return ""
		}},
		{name: "skill file", want: "SKILL.md target is a symlink", setup: func(root string) string {
			dir := filepath.Join(root, "handoff")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(t.TempDir(), "SKILL.md")
			if err := os.WriteFile(external, []byte("external\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(external, filepath.Join(dir, "SKILL.md")); err != nil {
				t.Fatal(err)
			}
			return external
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "skills")
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			external := test.setup(root)
			var stderr bytes.Buffer
			err := Install(root, skills.Definitions(), io.Discard, &stderr)
			if err == nil || !strings.Contains(err.Error(), test.want) || !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("error = %v, stderr = %q, want %q", err, stderr.String(), test.want)
			}
			if external != "" {
				assertFileContents(t, external, "external\n")
			}
		})
	}
}

func TestInstallRejectsInvalidDefinitionSet(t *testing.T) {
	defs := skills.Definitions()
	defs[0].Name = "not-official"
	err := Install(t.TempDir(), defs, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "missing official skill") {
		t.Fatalf("Install() error = %v, want missing official skill", err)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}
