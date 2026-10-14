package codemap

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Generate creates a complete offline report in the caller's current directory.
// It returns the absolute output path only after successful publication. Source
// contents are never retained in the report, and existing reports survive errors.
func Generate(source string) (string, error) {
	caller, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve output directory: %w", err)
	}
	scan := func(path string) (*directory, error) {
		destination := filepath.Join(caller, reportFilename(filepath.Base(filepath.Clean(path))))
		return scanProjectExcludingReport(path, destination)
	}
	return generateIn(source, caller, scan, renderHTML, publishHTML)
}

// generateIn keeps tests isolated from process-wide working-directory changes.
func generateIn(source, caller string, scan func(string) (*directory, error), render func(*directory) ([]byte, error), publish func(string, []byte) error) (string, error) {
	if source == "" {
		return "", fmt.Errorf("source directory is required")
	}
	if !filepath.IsAbs(source) {
		source = filepath.Join(caller, source)
	}
	root, err := scan(source)
	if err != nil {
		return "", fmt.Errorf("scan source directory %q: %w", source, err)
	}
	if root == nil {
		return "", fmt.Errorf("scan source directory %q: missing metadata", source)
	}
	if root.Lines == 0 {
		return "", fmt.Errorf("no recognized nonblank source lines found in %q", source)
	}
	page, err := render(root)
	if err != nil {
		return "", fmt.Errorf("render codebase map: %w", err)
	}
	destination := filepath.Join(caller, reportFilename(root.Name))
	if err := publish(destination, page); err != nil {
		return "", fmt.Errorf("publish codebase map %q: %w", destination, err)
	}
	return destination, nil
}

// reportFilename preserves ordinary basenames, including spaces and Unicode.
// Root paths use "root". Invalid Windows filename characters and overlong names
// are normalized with a hash suffix to avoid ambiguous fallback collisions.
// The prefixed full filename cannot be a reserved Windows device name.
func reportFilename(basename string) string {
	const prefix, suffix = "badger-codemap-", ".html"
	const maxNameBytes = 255 - len(prefix) - len(suffix)
	if basename == "" || basename == "/" || basename == `\` || basename == "." || basename == ".." {
		basename = "root"
	}
	safe := strings.Map(func(r rune) rune {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, strings.ToValidUTF8(basename, "_"))
	safe = strings.TrimRight(safe, " .")
	if safe == "" {
		safe = "root"
	}
	if safe != basename || len(safe) > maxNameBytes {
		hash := sha256.Sum256([]byte(basename))
		tag := fmt.Sprintf("-%x", hash[:4])
		maxBase := maxNameBytes - len(tag)
		if len(safe) > maxBase {
			n := maxBase
			for !utf8.RuneStart(safe[n]) {
				n--
			}
			safe = strings.TrimRight(safe[:n], " .")
		}
		safe += tag
	}
	return prefix + safe + suffix
}

type stagedReport interface {
	io.Writer
	Name() string
	Close() error
}

type publicationOps struct {
	lstat   func(string) (os.FileInfo, error)
	create  func(string) (stagedReport, error)
	replace func(string, string) error
	remove  func(string) error
}

func publishHTML(destination string, page []byte) error {
	return publishWithOps(destination, page, publicationOps{
		lstat:   os.Lstat,
		create:  func(dir string) (stagedReport, error) { return os.CreateTemp(dir, ".badger-map-*.tmp") },
		replace: replaceReport,
		remove:  os.Remove,
	})
}

func publishWithOps(destination string, page []byte, ops publicationOps) (err error) {
	if len(page) > maxHTMLBytes {
		return fmt.Errorf("HTML size limit (%d bytes) exceeded", maxHTMLBytes)
	}
	info, err := ops.lstat(destination)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing to replace non-regular report %q", destination)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing report: %w", err)
	}
	temp, err := ops.create(filepath.Dir(destination))
	if err != nil {
		return fmt.Errorf("create staged report: %w", err)
	}
	temporaryPath := temp.Name()
	closed := false
	published := false
	defer func() {
		if !closed {
			if closeErr := temp.Close(); closeErr != nil {
				err = errors.Join(err, fmt.Errorf("close staged report during cleanup: %w", closeErr))
			}
		}
		if !published {
			if removeErr := ops.remove(temporaryPath); removeErr != nil && !os.IsNotExist(removeErr) {
				err = errors.Join(err, fmt.Errorf("remove staged report %q: %w", temporaryPath, removeErr))
			}
		}
	}()
	written, err := temp.Write(page)
	if err != nil {
		return fmt.Errorf("write staged report: %w", err)
	}
	if written != len(page) {
		return fmt.Errorf("write staged report: %w", io.ErrShortWrite)
	}
	closeErr := temp.Close()
	closed = true
	if closeErr != nil {
		return fmt.Errorf("close staged report: %w", closeErr)
	}
	if err := ops.replace(temporaryPath, destination); err != nil {
		return fmt.Errorf("replace report: %w", err)
	}
	published = true
	return nil
}
