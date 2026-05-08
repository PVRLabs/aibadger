package workflow

// This file preserves the public workflow constants used by TUI and headless
// callers while delegating the raw defaults to a lower-level package.

import "github.com/PVRLabs/aibadger/internal/defaults"

const (
	ExitCommand               = defaults.ExitCommand
	LargeProjectFileThreshold = defaults.LargeProjectFileThreshold
	LargePromptBytes          = defaults.LargePromptBytes
	TruncatedMaxPackages      = defaults.TruncatedMaxPackages
	MaxContextFileBytes       = defaults.MaxContextFileBytes
	MaxTotalContextBytes      = defaults.MaxTotalContextBytes
	StepNames                 = defaults.StepNames
	TopologyPromptKind        = "Prompt 1: Topology"
	CodeContextPromptKind     = "Prompt 2: Code Context"
)
