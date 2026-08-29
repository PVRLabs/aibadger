// Package downloads resolves the current user's existing Downloads directory.
package downloads

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const PromptFilename = "badger-prompt.txt"

// Resolve returns the user's existing Downloads directory. It never creates a
// directory or substitutes another destination when Downloads is unavailable.
func Resolve() (string, error) {
	return resolveDownloadsDirectory(nativeDownloadsDirectory, os.Stat)
}

// SavePrompt installs payload as the stable prompt handoff in directory. The
// directory must already be the resolved user's Downloads directory.
func SavePrompt(directory string, payload []byte) (string, error) {
	info, err := os.Stat(directory)
	if err != nil {
		return "", fmt.Errorf("save Downloads prompt: validate directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("save Downloads prompt: path %q is not a directory", directory)
	}

	return savePromptWithOps(directory, payload, promptFileOps{
		lstat:     os.Lstat,
		createTmp: createPromptTempFile,
		replace:   replacePromptFile,
	})
}

type promptFileOps struct {
	lstat     func(string) (os.FileInfo, error)
	createTmp func(string) (*os.File, error)
	replace   func(string, string) error
}

func savePromptWithOps(directory string, payload []byte, ops promptFileOps) (string, error) {
	destination := filepath.Join(directory, PromptFilename)
	info, err := ops.lstat(destination)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("save Downloads prompt: refusing to overwrite symlink %q", destination)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("save Downloads prompt: refusing to overwrite non-regular file %q", destination)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("save Downloads prompt: inspect destination %q: %w", destination, err)
	}

	temporary, err := ops.createTmp(directory)
	if err != nil {
		return "", fmt.Errorf("save Downloads prompt: create temporary file in %q: %w", directory, err)
	}
	temporaryPath := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryPath)
		}
	}()

	if err := temporary.Chmod(0600); err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("save Downloads prompt: set temporary file permissions: %w", err)
	}
	written, err := temporary.Write(payload)
	if err != nil {
		_ = temporary.Close()
		return "", fmt.Errorf("save Downloads prompt: write temporary file: %w", err)
	}
	if written != len(payload) {
		_ = temporary.Close()
		return "", fmt.Errorf("save Downloads prompt: write temporary file: %w", io.ErrShortWrite)
	}
	if err := temporary.Close(); err != nil {
		return "", fmt.Errorf("save Downloads prompt: close temporary file: %w", err)
	}
	if err := ops.replace(temporaryPath, destination); err != nil {
		return "", fmt.Errorf("save Downloads prompt: install %q: %w", destination, err)
	}
	removeTemporary = false
	return destination, nil
}

func createPromptTempFile(directory string) (*os.File, error) {
	return os.CreateTemp(directory, ".badger-prompt-*")
}

func resolveDownloadsDirectory(pathFunc func() (string, error), statFunc func(string) (os.FileInfo, error)) (string, error) {
	path, err := pathFunc()
	if err != nil {
		return "", fmt.Errorf("resolve Downloads directory: %w", err)
	}
	if path == "" {
		return "", fmt.Errorf("resolve Downloads directory: resolver returned an empty path")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("resolve Downloads directory: resolver returned a non-absolute path %q", path)
	}

	info, err := statFunc(path)
	if err != nil {
		return "", fmt.Errorf("validate Downloads directory %q: %w", path, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("validate Downloads directory %q: path is not a directory", path)
	}
	return path, nil
}
