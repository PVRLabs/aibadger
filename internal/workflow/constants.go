package workflow

import (
	"fmt"

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
	MaxFilesPerDirectory      = defaults.MaxFilesPerDirectory
	StepNames                 = defaults.StepNames
	TopologyPromptKind        = "Prompt 1: Topology"
	CodeContextPromptKind     = "Prompt 2: Code Context"
)

func PromptTwoKind(_ protocol.Focus) string {
	return CodeContextPromptKind
}

func PipelineFinalLabel(_ protocol.Focus) string {
	return "Apply"
}

func ContextReadyStatus(focus protocol.Focus) string {
	return fmt.Sprintf("Code context ready. Review the file list before copying %s.", PromptTwoKind(focus))
}

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
