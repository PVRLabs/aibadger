package defaults

// This file owns shared default values that must be usable by lower-level
// packages without importing higher-level workflow helpers.

const (
	ExitCommand = "/exit"
	// LargeProjectFileThreshold triggers the TUI continue/truncate/exit prompt.
	LargeProjectFileThreshold = 1000
	// LargePromptBytes triggers the expanded TUI prompt-delivery menu.
	LargePromptBytes = 50 * 1024
	// TruncatedMaxPackages caps Prompt 1 packages in large-project mode.
	TruncatedMaxPackages = 50
	// MaxContextFileBytes caps each extracted file block in Prompt 2.
	MaxContextFileBytes = 50 * 1024
	// MaxTotalContextBytes caps the full Prompt 2 payload after per-file trimming.
	MaxTotalContextBytes = 100000
	// MaxFilesPerDirectory caps the number of files processed per directory in
	// the generic detector. Prevents hangs on directories like C:\Windows\System32.
	MaxFilesPerDirectory = 250
	// MaxTotalScanFiles caps the total number of files processed during a
	// generic-detector scan across all directories.
	MaxTotalScanFiles = 10000
	StepNames         = "scan, goal, topology (aliases: map, prompt1), extraction, context (alias: prompt2), write-plan (alias: write)"
)
