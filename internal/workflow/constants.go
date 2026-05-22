package workflow

// This file preserves the public workflow constants used by TUI and headless
// callers while delegating the raw defaults to a lower-level package.

import (
	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/protocol"
)

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

// PromptTwoKind returns the user-facing Prompt 2 label for the active focus.
func PromptTwoKind(focus protocol.Focus) string {
	if protocol.NormalizeFocus(focus) == protocol.FocusCode {
		return CodeContextPromptKind
	}
	return "Prompt 2: Respond"
}

// PipelineFinalLabel returns the final pipeline stage label for the focus.
func PipelineFinalLabel(focus protocol.Focus) string {
	if protocol.NormalizeFocus(focus) == protocol.FocusCode {
		return "Apply"
	}
	return "Respond"
}

// FocusDisplayName returns the capitalized focus label used in status text.
func FocusDisplayName(focus protocol.Focus) string {
	switch protocol.NormalizeFocus(focus) {
	case protocol.FocusReview:
		return "Review"
	case protocol.FocusDesign:
		return "Design"
	default:
		return "Code"
	}
}
