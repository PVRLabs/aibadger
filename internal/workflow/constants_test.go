package workflow

import (
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestContextReadyStatus(t *testing.T) {
	want := "Code context ready. Review the file list before copying Prompt 2: Code Context."
	for _, focus := range []protocol.Focus{protocol.FocusCode, protocol.FocusReview, protocol.FocusDesign} {
		if got := ContextReadyStatus(focus); got != want {
			t.Fatalf("ContextReadyStatus(%v) = %q, want %q", focus, got, want)
		}
	}
}
