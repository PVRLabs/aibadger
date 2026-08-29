package downloads

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSavePromptCreatesStablePrivateFile(t *testing.T) {
	directory := t.TempDir()
	payload := []byte("Prompt 1\x00exact bytes\nPrompt 2")

	path, err := SavePrompt(directory, payload)
	if err != nil {
		t.Fatalf("SavePrompt() error = %v", err)
	}
	if want := filepath.Join(directory, PromptFilename); path != want {
		t.Fatalf("SavePrompt() path = %q, want %q", path, want)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("saved payload = %q, want %q", got, payload)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("saved permissions = %04o, want 0600", mode)
	}
	assertNoPromptTemporaryFiles(t, directory)
}

func TestSavePromptReplacesRegularFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, PromptFilename)
	if err := os.WriteFile(path, []byte("old prompt"), 0644); err != nil {
		t.Fatal(err)
	}

	want := []byte("new prompt\nwith exact framing")
	gotPath, err := SavePrompt(directory, want)
	if err != nil {
		t.Fatalf("SavePrompt() error = %v", err)
	}
	if gotPath != path {
		t.Fatalf("SavePrompt() path = %q, want %q", gotPath, path)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("saved payload = %q, want %q", got, want)
	}
	assertNoPromptTemporaryFiles(t, directory)
}

func TestSavePromptRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	path := filepath.Join(directory, PromptFilename)
	if err := os.WriteFile(target, []byte("target remains"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	_, err := SavePrompt(directory, []byte("replacement"))
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("SavePrompt() error = %v, want symlink error", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "target remains" {
		t.Fatalf("symlink target = %q, want unchanged target", got)
	}
	assertNoPromptTemporaryFiles(t, directory)
}

func TestSavePromptRejectsNonRegularDestination(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, PromptFilename)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}

	_, err := SavePrompt(directory, []byte("replacement"))
	if err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("SavePrompt() error = %v, want non-regular error", err)
	}
	if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
		t.Fatalf("destination changed after rejection: info=%v err=%v", info, statErr)
	}
	assertNoPromptTemporaryFiles(t, directory)
}

func TestSavePromptPreservesExistingFileOnWriteFailure(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, PromptFilename)
	if err := os.WriteFile(destination, []byte("old prompt"), 0600); err != nil {
		t.Fatal(err)
	}

	createTmp := func(directory string) (*os.File, error) {
		file, err := os.CreateTemp(directory, ".badger-prompt-*")
		if err != nil {
			return nil, err
		}
		if err := file.Close(); err != nil {
			return nil, err
		}
		return file, nil
	}
	_, err := savePromptWithOps(directory, []byte("new prompt"), promptFileOps{
		lstat:     os.Lstat,
		createTmp: createTmp,
		replace:   replacePromptFile,
	})
	if err == nil {
		t.Fatal("savePromptWithOps() error = nil")
	}
	assertFileContents(t, destination, "old prompt")
	assertNoPromptTemporaryFiles(t, directory)
}

func TestSavePromptPreservesExistingFileOnInstallFailure(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, PromptFilename)
	if err := os.WriteFile(destination, []byte("old prompt"), 0600); err != nil {
		t.Fatal(err)
	}

	_, err := savePromptWithOps(directory, []byte("new prompt"), promptFileOps{
		lstat:     os.Lstat,
		createTmp: func(directory string) (*os.File, error) { return os.CreateTemp(directory, ".badger-prompt-*") },
		replace:   func(_, _ string) error { return errors.New("install failed") },
	})
	if err == nil || !strings.Contains(err.Error(), "install failed") {
		t.Fatalf("savePromptWithOps() error = %v, want install failure", err)
	}
	assertFileContents(t, destination, "old prompt")
	assertNoPromptTemporaryFiles(t, directory)
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("file contents = %q, want %q", got, want)
	}
}

func assertNoPromptTemporaryFiles(t *testing.T, directory string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, ".badger-prompt-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary prompt files remain: %v", matches)
	}
}
