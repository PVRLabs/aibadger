package model

import "time"

const (
	FileKindSource = "source"
	FileKindAsset  = "asset"
	FileKindBinary = "binary"
)

// ProjectTopology represents the complete structure of the project.
type ProjectTopology struct {
	Name            string            `json:"name"`
	Languages       []string          `json:"languages"`        // e.g. ["Java", "JavaScript"]
	PrimaryLanguage string            `json:"primary_language"` // e.g. "Java"
	Stack           []string          `json:"stack"`            // e.g. ["Maven", "Go Modules", "Node.js"]
	Structure       string            `json:"structure"`        // e.g. "Single Module", "Multi-Module"
	Modules         []Module          `json:"modules"`
	ExternalContext []ExternalContext `json:"external_context,omitempty"`
	ScanTime        time.Duration     `json:"scan_time"`
	ProjectRoot     string            `json:"project_root"`
}

// ExternalContext represents an explicitly configured read-only context
// directory outside, or separate from, the normal project source topology.
type ExternalContext struct {
	Path    string                `json:"path"`     // As listed in .badger-context, cleaned for display
	AbsPath string                `json:"abs_path"` // Absolute path used for local validation only
	Top     []ExternalContextItem `json:"top,omitempty"`
}

// ExternalContextItem stores a compact top-level summary entry for Prompt 1.
type ExternalContextItem struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir,omitempty"`
}

// Module represents a logical unit of code (e.g., a Maven module or a Go package).
type Module struct {
	Name        string        `json:"name"`
	Path        string        `json:"path"` // Relative to project root
	FileCount   int           `json:"file_count"`
	TotalBytes  int64         `json:"total_bytes"`
	Heaviest    HeaviestFile  `json:"heaviest"`
	TopFiles    []FileSummary `json:"top_files"`
	AuxFiles    []FileSummary `json:"aux_files,omitempty"`
	SourceRoots []SourceRoot  `json:"source_roots"`
	Language    string        `json:"language"` // e.g. "Java", "Go"
}

// SourceRoot represents a directory containing source files (e.g., src/main/java).
type SourceRoot struct {
	Path      string    `json:"path"` // Relative to project root
	Role      string    `json:"role"` // e.g. "Main Source", "Test Source"
	FileCount int       `json:"file_count"`
	Packages  []Package `json:"packages"`
}

// Package represents a directory that contains source files (e.g. com.example.service).
type Package struct {
	Name      string        `json:"name"` // e.g. "com.example.service"
	Path      string        `json:"path"` // Relative to project root
	FileCount int           `json:"file_count"`
	Heaviest  HeaviestFile  `json:"heaviest"`
	TopFiles  []FileSummary `json:"top_files"`
	AuxFiles  []FileSummary `json:"aux_files,omitempty"`
}

// HeaviestFile stores details about the largest primary source file in a module.
type HeaviestFile struct {
	Name string `json:"name"`
	Path string `json:"path"` // Relative to project root
	Size int64  `json:"size"`
	Kind string `json:"kind,omitempty"`
}

// FileSummary stores compact metadata for source files surfaced in Schema A.
type FileSummary struct {
	Name string `json:"name"`
	Path string `json:"path"` // Relative to project root
	Size int64  `json:"size"`
	Kind string `json:"kind,omitempty"`
}
