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
	MaxTopologyPromptBytes    = defaults.MaxTopologyPromptBytes
	MaxPromptTwoBytes         = defaults.MaxPromptTwoBytes
	MaxFilesPerDirectory      = defaults.MaxFilesPerDirectory
	StepNames                 = defaults.StepNames
	TopologyPromptKind        = "Prompt 1: Topology"
	CodeContextPromptKind     = "Prompt 2: Code Context"
	PipelineFinalLabel        = "Apply"
)

func ContextReadyStatus() string {
	return fmt.Sprintf("Code context ready. Review the file list before copying %s.", CodeContextPromptKind)
}

func FocusDisplayName(focus protocol.Focus) string {
	switch protocol.NormalizeFocus(focus) {
	case protocol.FocusReview:
		return "Review"
	case protocol.FocusDesign:
		return "Design"
	case protocol.FocusFollowup:
		return "Follow-up"
	default:
		return "Code"
	}
}
