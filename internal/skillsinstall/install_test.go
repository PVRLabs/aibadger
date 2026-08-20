package skillsinstall

import (
	"bytes"
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

	defs := skills.Definitions()
	for _, definition := range defs {
		path := filepath.Join(root, definition.Name, "SKILL.md")
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if string(got) != definition.Content {
			t.Errorf("%s content differs from embedded definition", definition.Name)
		}
		if mode := fileMode(t, path); mode.Perm() != installFileMode {
			t.Errorf("%s mode = %o, want %o", definition.Name, mode.Perm(), installFileMode)
		}
	}

	wantOutput := "installed badger-review: " + filepath.Join(root, "badger-review", "SKILL.md") + "\n" +
		"installed badger-handoff: " + filepath.Join(root, "badger-handoff", "SKILL.md") + "\n"
	if stdout.String() != wantOutput {
		t.Fatalf("stdout = %q, want %q", stdout.String(), wantOutput)
	}
}

func TestInstallUpdatesIdempotentlyAndPreservesUnrelatedFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	if err := Install(root, skills.Definitions(), ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("initial Install() error = %v", err)
	}

	keepRoot := filepath.Join(root, "other-skill")
	keepFile := filepath.Join(keepRoot, "SKILL.md")
	if err := os.MkdirAll(keepRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keepFile, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(root, "badger-review", "notes.txt")
	if err := os.WriteFile(extra, []byte("unrelated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "badger-review", "SKILL.md"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Install(root, skills.Definitions(), &stdout, &stderr); err != nil {
		t.Fatalf("update Install() error = %v", err)
	}
	if !strings.HasPrefix(stdout.String(), "updated badger-review: ") || !strings.Contains(stdout.String(), "updated badger-handoff: ") {
		t.Fatalf("stdout = %q, want updated reports", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	assertFileContents(t, filepath.Join(root, "badger-review", "SKILL.md"), skills.Definitions()[0].Content)
	assertFileContents(t, keepFile, "keep me\n")
	assertFileContents(t, extra, "unrelated\n")
}

func TestInstallAcceptsSymlinkedSharedParents(t *testing.T) {
	t.Run("agents parent", func(t *testing.T) {
		targetAgents := filepath.Join(t.TempDir(), "real-agents")
		if err := os.MkdirAll(targetAgents, 0o700); err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		if err := os.Symlink(targetAgents, filepath.Join(home, ".agents")); err != nil {
			t.Fatal(err)
		}
		assertInstallSuccess(t, filepath.Join(home, ".agents", "skills"))
	})

	t.Run("skills parent", func(t *testing.T) {
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
		assertInstallSuccess(t, filepath.Join(home, ".agents", "skills"))
	})
}

func TestInstallRejectsSymlinkedBadgerOwnedTargets(t *testing.T) {
	t.Run("skill directory", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "skills")
		if err := os.MkdirAll(root, 0o700); err != nil {
			t.Fatal(err)
		}
		external := t.TempDir()
		if err := os.Symlink(external, filepath.Join(root, "badger-review")); err != nil {
			t.Fatal(err)
		}
		assertInstallFailure(t, root, "skill directory is a symlink")
	})

	t.Run("skill file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "skills")
		dir := filepath.Join(root, "badger-review")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "SKILL.md")
		if err := os.WriteFile(external, []byte("external\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		assertInstallFailure(t, root, "SKILL.md target is a symlink")
		assertFileContents(t, external, "external\n")
	})
}

func TestInstallRejectsInvalidDestinationAndLeavesNoTemporaryFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	dir := filepath.Join(root, "badger-review")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "SKILL.md"), 0o700); err != nil {
		t.Fatal(err)
	}

	assertInstallFailure(t, root, "SKILL.md target is not a regular file")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".SKILL.md-") {
			t.Fatalf("temporary file %q remains after failure", entry.Name())
		}
	}

	invalidRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(invalidRoot, []byte("file"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertInstallFailureAt(t, invalidRoot, "create skills root")
}

func TestInstallValidatesFixedOfficialDefinitions(t *testing.T) {
	defs := skills.Definitions()
	defs[0].Name = "not-official"
	var stderr bytes.Buffer
	if err := Install(t.TempDir(), defs, ioDiscard{}, &stderr); err == nil || !strings.Contains(err.Error(), "missing official skill") {
		t.Fatalf("Install() error = %v, want missing official skill", err)
	}
	if !strings.Contains(stderr.String(), "skills install:") {
		t.Fatalf("stderr = %q, want installer diagnostic", stderr.String())
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func assertInstallSuccess(t *testing.T, root string) {
	t.Helper()
	if err := Install(root, skills.Definitions(), ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatalf("Install(%s) error = %v", root, err)
	}
	for _, definition := range skills.Definitions() {
		assertFileContents(t, filepath.Join(root, definition.Name, "SKILL.md"), definition.Content)
	}
}

func assertInstallFailure(t *testing.T, root, want string) {
	t.Helper()
	assertInstallFailureAt(t, root, want)
}

func assertInstallFailureAt(t *testing.T, root, want string) {
	t.Helper()
	var stderr bytes.Buffer
	err := Install(root, skills.Definitions(), ioDiscard{}, &stderr)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Install() error = %v, want %q", err, want)
	}
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr = %q, want %q", stderr.String(), want)
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s = %q, want %q", path, string(got), want)
	}
}

func fileMode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s) error = %v", path, err)
	}
	return info.Mode()
}
