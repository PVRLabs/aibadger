package tui

import (
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

func TestPromptOnePrivacyTextWithReviewAttachmentUnderDesign(t *testing.T) {
	got := promptOnePrivacyTextWithAttachment(protocol.FocusDesign, nil, true)
	if !strings.Contains(got, "Privacy: Includes Git changes") {
		t.Fatalf("privacy text = %q, want Git-change warning", got)
	}
	if got == "Privacy: Structure only - no source code." {
		t.Fatal("privacy text retained structure-only message for review attachment")
	}
}

func TestPromptOnePrivacyTextWithoutReviewAttachmentUnderDesign(t *testing.T) {
	got := promptOnePrivacyTextWithAttachment(protocol.FocusDesign, nil, false)
	if got != "Privacy: Structure only - no source code." {
		t.Fatalf("privacy text = %q, want structure-only message", got)
	}
}
