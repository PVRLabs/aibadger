package reviewtask

import (
	"fmt"
	"sort"
	"strings"
)

// PayloadOverflow describes the rendered payload at a failed size check.
// Patch bytes describe only the already collected tracked patches.
type PayloadOverflow struct {
	PayloadBytes   int
	LimitBytes     int
	LargestPatches []PatchContributor
}

type PatchContributor struct {
	Path  string
	Bytes int
}

func mandatoryOverflow(set ChangeSet, payloadBytes, limitBytes int) InitialReviewResult {
	patches := make([]PatchContributor, 0, len(set.Changes))
	for _, change := range set.Changes {
		patches = append(patches, PatchContributor{Path: change.Path, Bytes: len(change.Patch)})
	}
	sort.Slice(patches, func(i, j int) bool {
		if patches[i].Bytes != patches[j].Bytes {
			return patches[i].Bytes > patches[j].Bytes
		}
		return patches[i].Path < patches[j].Path
	})
	if len(patches) > 5 {
		patches = patches[:5]
	}
	return InitialReviewResult{Failure: PayloadFailureMandatoryOverflow, Overflow: &PayloadOverflow{
		PayloadBytes: payloadBytes, LimitBytes: limitBytes, LargestPatches: patches,
	}}
}

func (overflow *PayloadOverflow) Message(opts Options) string {
	var out strings.Builder
	out.WriteString("Cannot prepare review: the complete Git changes exceed the review payload limit.\n\n")
	fmt.Fprintf(&out, "Payload size at failure:    %d bytes\nEffective payload limit:    %d bytes\n", overflow.PayloadBytes, overflow.LimitBytes)
	if len(overflow.LargestPatches) > 0 {
		out.WriteString("\nLargest file patches (patch bytes only):\n")
		for _, patch := range overflow.LargestPatches {
			fmt.Fprintf(&out, "  %d bytes  %s\n", patch.Bytes, escapeReviewPath(patch.Path))
		}
	}
	command := "badger review"
	switch opts.Mode {
	case ModeStaged:
		command += " --staged"
	case ModeBranch:
		command += " --branch " + quoteReviewShellArg(opts.Ref)
	case ModeCommit:
		command += " --commit " + quoteReviewShellArg(opts.Ref)
	}
	for _, pattern := range opts.ExcludePatterns {
		command += " --exclude " + quoteReviewShellArg(pattern)
	}
	out.WriteString("\nRetry with a narrower review, for example:\n  " + command + " --exclude '<pattern>'\n\nExcluded changes will not be reviewed.")
	return out.String()
}

func quoteReviewShellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
