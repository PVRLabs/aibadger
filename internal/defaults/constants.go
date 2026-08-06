package defaults

// This file owns shared default values that must be usable by lower-level
// packages without importing higher-level workflow helpers.

const (
	ExitCommand = "/exit"
	// LargeProjectFileThreshold triggers the TUI continue/truncate/exit prompt.
	LargeProjectFileThreshold = 1000
	// LargePromptBytes is the threshold at which the expanded TUI
	// prompt-delivery menu is shown instead of the normal y/N prompt.
	LargePromptBytes = 128 * 1024
	// TruncatedMaxPackages caps Prompt 1 packages in large-project mode.
	TruncatedMaxPackages = 50
	// MaxContextFileBytes caps the amount of extracted file content retained
	// before adding truncation markers and Prompt 2 block framing.
	MaxContextFileBytes = 50 * 1024
	// MaxTopologyPromptBytes caps the complete serialized Prompt 1 target,
	// including topology, task content, and constraints.
	MaxTopologyPromptBytes = 512 * 1024
	// MaxPromptTwoBytes is the target maximum for the serialized Prompt 2.
	// Badger enforces it by dropping extracted context blocks. If the
	// non-droppable prompt framing, topology, task, and instructions alone
	// exceed the target, they remain intact and the final output may exceed it.
	MaxPromptTwoBytes = 192 * 1024
	// MaxFilesPerDirectory caps the number of files processed per directory in
	// the generic detector. Prevents hangs on directories like C:\Windows\System32.
	MaxFilesPerDirectory = 250
	// MaxTotalScanFiles caps the total number of files processed during a
	// generic-detector scan across all directories.
	MaxTotalScanFiles = 10000
	StepNames         = "scan, goal, topology (aliases: map, prompt1), extraction, context (alias: prompt2), write-plan (alias: write)"
)
