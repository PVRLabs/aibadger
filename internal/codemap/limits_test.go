package codemap

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanLimits(t *testing.T) {
	tests := []struct {
		name   string
		paths  []string
		limits scanLimits
		want   string
	}{
		{"depth boundary", []string{"a/b/main.go"}, scanLimits{2, 3, 1, 10}, ""},
		{"depth exceeded", []string{"a/b/c/main.go"}, scanLimits{2, 10, 10, 10}, "directory depth limit (2)"},
		{"directory count", []string{"a/main.go", "b/main.go"}, scanLimits{5, 2, 10, 10}, "directory limit (2)"},
		{"file count", []string{"a.go", "b.go"}, scanLimits{5, 10, 1, 10}, "recognized-file limit (1)"},
		{"file bytes", []string{"main.go"}, scanLimits{5, 10, 10, 1}, "source file size limit (1 bytes)"},
		{"excluded trees and extensions", []string{"node_modules/deep/file.go", "ignored.bin", "main.go"}, scanLimits{0, 1, 1, 10}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, path := range tt.paths {
				writeTestFile(t, root, path, "x\n")
			}
			tree, err := scanProjectWithLimits(root, tt.limits)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				if tree.Files != 1 {
					t.Fatalf("files = %d", tree.Files)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) || tree != nil {
				t.Fatalf("tree=%v error=%v, want %s", tree, err, tt.want)
			}
		})
	}
}

func TestBoundedLineCounting(t *testing.T) {
	for _, contents := range []string{
		"", " \t\r\n\u2003\u00a0\n", "one\r\n\nlast", strings.Repeat("x", 100_000),
		strings.Repeat(" ", 4095) + "\u2003\nhello\n", "invalid\xff\n",
	} {
		want := 0
		for _, line := range strings.Split(contents, "\n") {
			if strings.TrimSpace(line) != "" {
				want++
			}
		}
		got, err := countLinesBounded(strings.NewReader(contents), int64(len(contents)))
		if err != nil || got != want {
			t.Fatalf("length %d: got %d, %v; want %d", len(contents), got, err, want)
		}
	}
	// Reader enforcement also catches data that grows after the file's stat.
	if _, err := countLinesBounded(strings.NewReader("123456"), 5); err == nil {
		t.Fatal("reader exceeded its byte budget")
	}
}

func TestOversizedSourceRejectedBeforeReading(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.go")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Sparse file: no large fixture or download needed.
	if err := file.Truncate(maxSourceBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	file.Close()
	if _, err := countNonblankLines(path); err == nil || !strings.Contains(err.Error(), "source file size limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestProductionScanLimits(t *testing.T) {
	if maxDirectoryDepth != 50 || maxDirectories != 20_000 || maxSourceFiles != 100_000 || maxSourceBytes != 50*1024*1024 {
		t.Fatalf("production scan limits changed: %d/%d/%d/%d", maxDirectoryDepth, maxDirectories, maxSourceFiles, maxSourceBytes)
	}
}

func TestEmptyDirectoriesConsumeBudgets(t *testing.T) {
	tests := []struct {
		name   string
		dirs   []string
		limits scanLimits
		want   string
	}{
		{"root exact", nil, scanLimits{0, 1, 0, 0}, ""},
		{"root exceeded", nil, scanLimits{0, 0, 0, 0}, "directory limit (0)"},
		{"empty exact", []string{"a/b"}, scanLimits{2, 3, 0, 0}, ""},
		{"empty depth", []string{"a/b"}, scanLimits{1, 3, 0, 0}, "directory depth limit (1)"},
		{"empty count", []string{"a/b"}, scanLimits{2, 2, 0, 0}, "directory limit (2)"},
		{"excluded at root", []string{".venv/a/b", "node_modules/c"}, scanLimits{0, 1, 0, 0}, ""},
		{"excluded at depth", []string{"a/.tox/b", "a/venv/c"}, scanLimits{1, 2, 0, 0}, ""},
		{"near match counts", []string{"venv_tools"}, scanLimits{0, 1, 0, 0}, "directory depth limit (0)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for _, dir := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			got, err := scanProjectWithLimits(root, tt.limits)
			if tt.want == "" {
				if err != nil || got == nil {
					t.Fatalf("scan: %v %v", got, err)
				}
				if got.Files != 0 || got.Lines != 0 || len(got.Children) != 0 {
					t.Fatalf("empty tree: %#v", got)
				}
			} else if err == nil || got != nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), root) {
				t.Fatalf("scan: %v %v; want nil and %q with input path", got, err, tt.want)
			}
		})
	}
}

func TestFileQuotaBoundaries(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		files         int
		bytes         int64
		want          string
	}{
		{"exact", "ab", 1, 2, ""}, {"bytes exceeded", "abc", 1, 2, "source file size limit (2 bytes)"},
		{"blank file exact", "", 1, 0, ""}, {"blank file exceeds count", "", 0, 0, "recognized-file limit (0)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestFile(t, root, "main.go", tt.content)
			writeTestFile(t, root, "ignored.txt", "arbitrary data beyond byte budget")
			writeTestFile(t, root, ".venv/deep/ignored.py", "arbitrary data beyond byte budget")
			got, err := scanProjectWithLimits(root, scanLimits{0, 1, tt.files, tt.bytes})
			if tt.want == "" {
				if err != nil || got == nil || got.Files != 1 {
					t.Fatalf("scan: %v %v", got, err)
				}
			} else if err == nil || got != nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), filepath.Join(root, "main.go")) {
				t.Fatalf("scan: %v %v; want nil and %q with input path", got, err, tt.want)
			}
		})
	}
}

func TestScanAcrossDirectoryBatches(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 257; i++ {
		writeTestFile(t, root, fmt.Sprintf("%03d.go", i), "x\n")
	}
	got, err := scanProjectWithLimits(root, scanLimits{0, 1, 257, 2})
	if err != nil || got == nil || got.Files != 257 || got.Lines != 257 || len(got.FileList) != 50 {
		t.Fatalf("exact batch scan: %v %v", got, err)
	}
	got, err = scanProjectWithLimits(root, scanLimits{0, 1, 256, 2})
	if err == nil || got != nil || !strings.Contains(err.Error(), "recognized-file limit (256)") || !strings.Contains(err.Error(), root) {
		t.Fatalf("batch overflow: %v %v", got, err)
	}
}

func TestSparseOversizeFailsWholeScan(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, "valid.go", "code\n")
	path := filepath.Join(root, "oversize.go")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(maxSourceBytes + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := scanProject(root)
	if got != nil || err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "source file size limit (52428800 bytes)") {
		t.Fatalf("oversize scan: %v %v", got, err)
	}
}

type measuredReader struct {
	input                io.Reader
	consumed, maxRequest int
}

func (r *measuredReader) Read(p []byte) (int, error) {
	if len(p) > r.maxRequest {
		r.maxRequest = len(p)
	}
	n, err := r.input.Read(p)
	r.consumed += n
	return n, err
}

func TestBoundedReaderStorageAndGrowth(t *testing.T) {
	// Supply more bytes than the budget, as when a source grows after Stat.
	reader := &measuredReader{input: strings.NewReader(strings.Repeat("x", 100_000))}
	got, err := countLinesBounded(reader, 5)
	if got != 0 || err == nil || !strings.Contains(err.Error(), "source file size limit (5 bytes)") {
		t.Fatalf("growth: %d %v", got, err)
	}
	if reader.consumed != 6 {
		t.Fatalf("read %d bytes, want only limit+1", reader.consumed)
	}
	reader = &measuredReader{input: strings.NewReader(strings.Repeat("x", 100_000))}
	got, err = countLinesBounded(reader, 100_000)
	if err != nil || got != 1 || reader.maxRequest > 4096 {
		t.Fatalf("long line: %d %v; max read %d", got, err, reader.maxRequest)
	}
	for _, text := range []string{"\u2003", "\u00a0", "界", "\xff"} {
		for split := 4093; split <= 4096; split++ {
			contents := strings.Repeat(" ", split) + text + "\nlast"
			want := 1
			if strings.TrimSpace(text) != "" {
				want++
			}
			got, err := countLinesBounded(strings.NewReader(contents), int64(len(contents)))
			if got != want || err != nil {
				t.Fatalf("split %d %q: %d %v", split, text, got, err)
			}
			got, err = countLinesBounded(strings.NewReader(contents), int64(len(contents)-1))
			if got != 0 || err == nil {
				t.Fatalf("split overflow: %d %v", got, err)
			}
		}
	}
}

type failingReader struct {
	err  error
	sent bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.sent {
		return 0, r.err
	}
	r.sent = true
	return copy(p, "code\npartial"), r.err
}
func TestReaderErrorsDiscardCounts(t *testing.T) {
	failure := errors.New("source read failed")
	for _, input := range []io.Reader{&failingReader{err: failure}, &failingReader{err: failure, sent: true}} {
		got, err := countLinesBounded(input, 100)
		if got != 0 || !errors.Is(err, failure) {
			t.Fatalf("read failure: %d %v", got, err)
		}
	}
}
