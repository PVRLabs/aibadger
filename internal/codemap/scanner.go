// Package codemap accounts for source files in standalone codebase maps.
package codemap

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

var extensionLanguages = map[string]string{
	".ads":         "Ada",
	".adb":         "Ada",
	".ada":         "Ada",
	".jcl":         "JCL",
	".sv":          "SystemVerilog",
	".svh":         "SystemVerilog",
	".vhd":         "VHDL",
	".vhdl":        "VHDL",
	".f77":         "Fortran",
	".f90":         "Fortran",
	".f95":         "Fortran",
	".f03":         "Fortran",
	".f08":         "Fortran",
	".fpp":         "Fortran",
	".ftn":         "Fortran",
	".pli":         "PL/I",
	".pl1":         "PL/I",
	".rpgle":       "RPG",
	".sqlrpgle":    "RPG",
	".rpgleinc":    "RPG",
	".sqlrpg":      "RPG",
	".clle":        "IBM CL",
	".clp":         "IBM CL",
	".clp38":       "IBM CL",
	".abap":        "ABAP",
	".json":        "JSON",
	".webmanifest": "JSON",
	".yaml":        "YAML",
	".yml":         "YAML",
	".toml":        "TOML",
	".xml":         "XML",
	".ini":         "INI",
	".conf":        "Config",
	".properties":  "Properties",
	".cbl":         "COBOL",
	".cob":         "COBOL",
	".cobol":       "COBOL",
	".ccp":         "COBOL",
	".cpy":         "COBOL",
	".go":          "Go",
	".java":        "Java",
	".kt":          "Kotlin",
	".kts":         "Kotlin",
	".js":          "JavaScript",
	".mjs":         "JavaScript",
	".cjs":         "JavaScript",
	".jsx":         "JavaScript",
	".ts":          "TypeScript",
	".mts":         "TypeScript",
	".cts":         "TypeScript",
	".tsx":         "TypeScript",
	".py":          "Python",
	".pyw":         "Python",
	".rs":          "Rust",
	".c":           "C",
	".h":           "C/C++",
	".cc":          "C++",
	".cpp":         "C++",
	".cxx":         "C++",
	".hh":          "C++",
	".hpp":         "C++",
	".hxx":         "C++",
	".cs":          "C#",
	".swift":       "Swift",
	".m":           "Objective-C",
	".mm":          "Objective-C++",
	".rb":          "Ruby",
	".php":         "PHP",
	".dart":        "Dart",
	".scala":       "Scala",
	".sc":          "Scala",
	".sh":          "Shell",
	".bash":        "Shell",
	".zsh":         "Shell",
	".fish":        "Shell",
	".sql":         "SQL",
	".html":        "HTML",
	".htm":         "HTML",
	".css":         "CSS",
	".scss":        "SCSS",
	".sass":        "Sass",
	".vue":         "Vue",
	".svelte":      "Svelte",
	".md":          "Markdown",
	".markdown":    "Markdown",
	".mdx":         "Markdown",
	".lua":         "Lua",
	".r":           "R",
	".pl":          "Perl",
	".pm":          "Perl",
	".ex":          "Elixir",
	".exs":         "Elixir",
	".erl":         "Erlang",
	".hrl":         "Erlang",
	".hs":          "Haskell",
	".lhs":         "Haskell",
	".clj":         "Clojure",
	".cljs":        "ClojureScript",
	".groovy":      "Groovy",
	".gradle":      "Groovy",
	".fs":          "F#",
	".fsx":         "F#",
	".vb":          "Visual Basic",
	".zig":         "Zig",
	".sol":         "Solidity",
	".tf":          "Terraform",
	".proto":       "Protocol Buffers",
}

var skippedDirectories = map[string]bool{
	".git":             true,
	".hg":              true,
	".svn":             true,
	".idea":            true,
	".vscode":          true,
	".gradle":          true,
	".next":            true,
	".nuxt":            true,
	".svelte-kit":      true,
	".terraform":       true,
	"__pycache__":      true,
	".venv":            true,
	"venv":             true,
	".tox":             true,
	".nox":             true,
	"bower_components": true,
	"build":            true,
	"coverage":         true,
	"dist":             true,
	"node_modules":     true,
	"obj":              true,
	"out":              true,
	"target":           true,
	"vendor":           true,
}

const (
	maxDirectoryDepth       = 50
	maxDirectories          = 20_000
	maxSourceFiles          = 100_000
	maxSourceBytes    int64 = 50 * 1024 * 1024
)

type scanLimits struct {
	depth, directories, files int
	sourceBytes               int64
}

// scanProject returns metadata, including blank recognized files. A zero-line
// result is valid here; report generation decides whether it can be rendered.
// Symlink roots are rejected and symlink entries are never traversed.
func scanProject(rootPath string) (*directory, error) {
	return scanProjectWithLimits(rootPath, scanLimits{maxDirectoryDepth, maxDirectories, maxSourceFiles, maxSourceBytes})
}

func scanProjectWithLimits(rootPath string, limits scanLimits) (*directory, error) {
	return scanProjectWithReadDir(rootPath, limits, (*os.File).ReadDir)
}

func scanProjectWithReadDir(rootPath string, limits scanLimits, readDir func(*os.File, int) ([]os.DirEntry, error)) (*directory, error) {
	return scanProjectWithReportExclusion(rootPath, limits, readDir, "")
}

// Exclude only the report being replaced, never a filename pattern or all HTML.
func scanProjectExcludingReport(rootPath, reportPath string) (*directory, error) {
	return scanProjectWithReportExclusion(rootPath, scanLimits{maxDirectoryDepth, maxDirectories, maxSourceFiles, maxSourceBytes}, (*os.File).ReadDir, reportPath)
}

func scanProjectWithReportExclusion(rootPath string, limits scanLimits, readDir func(*os.File, int) ([]os.DirEntry, error), reportPath string) (*directory, error) {
	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolve source directory %q: %w", rootPath, err)
	}
	info, err := os.Lstat(absRoot)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("symlink root is not allowed: %s", rootPath)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", rootPath)
	}

	root := newDirectory(filepath.Base(filepath.Clean(absRoot)), ".")
	directories := map[string]*directory{absRoot: root}

	if limits.directories < 1 {
		return nil, fmt.Errorf("directory limit (%d) exceeded at %s", limits.directories, absRoot)
	}
	var reportInfo os.FileInfo
	if reportPath != "" {
		reportPath = filepath.Clean(reportPath)
		reportInfo, _ = os.Lstat(reportPath)
	}
	sourceFiles := 0
	err = walkSourceTree(absRoot, 0, limits.depth, readDir, func(path string, entry classifiedEntry) error {
		if entry.isDir {
			if skippedDirectories[entry.name] {
				return nil
			}
			rel, err := filepath.Rel(absRoot, path)
			if err != nil {
				return err
			}
			if len(directories) >= limits.directories {
				return fmt.Errorf("directory limit (%d) exceeded at %s", limits.directories, path)
			}
			directories[path] = newDirectory(entry.name, filepath.ToSlash(rel))
			return nil
		}
		// Exclusion precedes file/byte quotas and line reading. SameFile handles
		// case/path aliases; matching the parent prevents excluding hard links
		// elsewhere in the tree merely because they share the report's inode.
		if reportPath != "" {
			if path == reportPath {
				return nil
			}
			if reportInfo != nil && strings.EqualFold(entry.name, filepath.Base(reportPath)) {
				candidate, candidateErr := os.Lstat(path)
				if candidateErr == nil && os.SameFile(candidate, reportInfo) {
					parent, parentErr := os.Stat(filepath.Dir(path))
					reportParent, reportParentErr := os.Stat(filepath.Dir(reportPath))
					if parentErr == nil && reportParentErr == nil && os.SameFile(parent, reportParent) {
						return nil
					}
				}
			}
		}
		language := entry.language
		if language == "" {
			return nil
		}
		if sourceFiles >= limits.files {
			return fmt.Errorf("recognized-file limit (%d) exceeded at %s", limits.files, path)
		}
		sourceFiles++
		lines, err := countNonblankLinesLimit(path, limits.sourceBytes)
		if err != nil {
			return err
		}
		parent := directories[filepath.Dir(path)]
		if parent == nil {
			return fmt.Errorf("internal error: missing directory for %s", path)
		}
		parent.DirectLines += lines
		parent.DirectFiles++
		retainSourceFile(parent, sourceFile{Name: entry.name, Lines: lines})
		addLanguage(parent.DirectByLang, language, lines, 1)
		return nil
	})
	if err != nil {
		return nil, err
	}

	paths := make([]string, 0, len(directories)-1)
	for path := range directories {
		if path != absRoot {
			paths = append(paths, path)
		}
	}
	sort.Slice(paths, func(i, j int) bool {
		depthI := strings.Count(filepath.Clean(paths[i]), string(filepath.Separator))
		depthJ := strings.Count(filepath.Clean(paths[j]), string(filepath.Separator))
		if depthI != depthJ {
			return depthI > depthJ
		}
		return paths[i] < paths[j]
	})

	for _, path := range paths {
		node := directories[path]
		finalizeDirectory(node)
		parent := directories[filepath.Dir(path)]
		parent.Children = append(parent.Children, node)
	}
	finalizeDirectory(root)
	return root, nil
}

type classifiedEntry struct {
	name     string
	isDir    bool
	language string
}

// classifyEntry resolves unknown types before deciding whether to traverse or
// count an entry. Info reports the entry itself without following symlinks.
func classifyEntry(entry os.DirEntry) (classifiedEntry, error) {
	result := classifiedEntry{name: entry.Name()}
	mode := entry.Type()
	if mode == 0 {
		info, err := entry.Info()
		if err != nil {
			return result, err
		}
		mode = info.Mode()
	}
	result.isDir = mode.IsDir()
	if mode.IsRegular() {
		// Generated dependency lockfiles can dwarf application source. Exclude
		// exact basenames before source quotas or line reading, at any depth.
		switch strings.ToLower(result.name) {
		case "package-lock.json", "pnpm-lock.yaml":
			return result, nil
		}
		result.language = extensionLanguages[strings.ToLower(filepath.Ext(result.name))]
	}
	return result, nil
}

func newDirectory(name, path string) *directory {
	return &directory{
		Name:         name,
		Path:         path,
		Languages:    make(map[string]languageStats),
		DirectByLang: make(map[string]languageStats),
		Children:     make([]*directory, 0),
		FileList:     make([]sourceFile, 0),
	}
}

func finalizeDirectory(node *directory) {
	node.Lines = node.DirectLines
	node.Files = node.DirectFiles
	for language, stats := range node.DirectByLang {
		node.Languages[language] = stats
	}
	for _, child := range node.Children {
		node.Lines += child.Lines
		node.Files += child.Files
		for language, stats := range child.Languages {
			addLanguage(node.Languages, language, stats.Lines, stats.Files)
		}
	}
	// Aggregate blank descendants before dropping their zero-area geometry.
	visibleChildren := node.Children[:0]
	for _, child := range node.Children {
		if child.Lines > 0 {
			visibleChildren = append(visibleChildren, child)
		}
	}
	clear(node.Children[len(visibleChildren):])
	node.Children = visibleChildren
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].Lines != node.Children[j].Lines {
			return node.Children[i].Lines > node.Children[j].Lines
		}
		return node.Children[i].Name < node.Children[j].Name
	})
}

func addLanguage(totals map[string]languageStats, language string, lines, files int) {
	current := totals[language]
	current.Lines += lines
	current.Files += files
	totals[language] = current
}

// Read directory entries in batches, rather than allocating every entry in a
// very wide directory before the scan limits can be checked.
func walkSourceTree(path string, depth, maxDepth int, readDir func(*os.File, int) ([]os.DirEntry, error), visit func(string, classifiedEntry) error) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	for {
		entries, readErr := readDir(dir, 128)
		for _, rawEntry := range entries {
			childPath := filepath.Join(path, rawEntry.Name())
			entry, err := classifyEntry(rawEntry)
			if err != nil {
				return fmt.Errorf("inspect entry %s: %w", childPath, err)
			}
			if entry.isDir && skippedDirectories[entry.name] {
				continue
			}
			if entry.isDir && depth >= maxDepth {
				return fmt.Errorf("directory depth limit (%d) exceeded at %s", maxDepth, childPath)
			}
			if err := visit(childPath, entry); err != nil {
				return err
			}
			if entry.isDir {
				if err := walkSourceTree(childPath, depth+1, maxDepth, readDir, visit); err != nil {
					return err
				}
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func retainSourceFile(node *directory, file sourceFile) {
	// Keep only the top 50 while scanning, including for huge flat folders.
	index := sort.Search(len(node.FileList), func(i int) bool {
		other := node.FileList[i]
		return file.Lines > other.Lines || (file.Lines == other.Lines && file.Name < other.Name)
	})
	if index >= 50 {
		return
	}
	if len(node.FileList) < 50 {
		node.FileList = append(node.FileList, sourceFile{})
	}
	copy(node.FileList[index+1:], node.FileList[index:len(node.FileList)-1])
	node.FileList[index] = file
}

func countNonblankLines(path string) (int, error) {
	return countNonblankLinesLimit(path, maxSourceBytes)
}

func countNonblankLinesLimit(path string, limit int64) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return 0, err
	}
	if info.Size() > limit {
		return 0, fmt.Errorf("source file size limit (%d bytes) exceeded: %s", limit, path)
	}
	count, err := countLinesBounded(file, limit)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	return count, nil
}

func countLinesBounded(input io.Reader, limit int64) (int, error) {
	// ReadRune uses a fixed-size buffer and preserves Unicode whitespace handling
	// even when a rune spans a buffer boundary. LimitReader also catches growth.
	reader := bufio.NewReader(io.LimitReader(input, limit+1))
	var consumed int64
	count, nonblank := 0, false
	for {
		char, size, err := reader.ReadRune()
		consumed += int64(size)
		if consumed > limit {
			return 0, fmt.Errorf("source file size limit (%d bytes) exceeded", limit)
		}
		if err == io.EOF {
			if nonblank {
				count++
			}
			return count, nil
		}
		if err != nil {
			return 0, err
		}
		if char == '\n' {
			if nonblank {
				count++
			}
			nonblank = false
		} else if !unicode.IsSpace(char) {
			nonblank = true
		}
	}
}
