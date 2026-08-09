package workflow

import (
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestContextReadyStatus(t *testing.T) {
	want := "Code context ready. Review the file list before copying Prompt 2: Code Context."
	if got := ContextReadyStatus(); got != want {
		t.Fatalf("ContextReadyStatus() = %q, want %q", got, want)
	}
}

func TestFocusDisplayName(t *testing.T) {
	cases := []struct {
		focus protocol.Focus
		want  string
	}{
		{focus: protocol.FocusCode, want: "Code"},
		{focus: protocol.FocusReview, want: "Review"},
		{focus: protocol.FocusDesign, want: "Design"},
		{focus: protocol.FocusFollowup, want: "Follow-up"},
		{focus: "unknown", want: "Code"},
	}

	for _, tc := range cases {
		if got := FocusDisplayName(tc.focus); got != tc.want {
			t.Fatalf("FocusDisplayName(%q) = %q, want %q", tc.focus, got, tc.want)
		}
	}
}
