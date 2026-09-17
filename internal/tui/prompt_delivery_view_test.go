package tui

import (
	"strings"
	"testing"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/model"
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

func TestScanCompleteRetainsReviewAttachmentSummaryAcrossDeliveryBranches(t *testing.T) {
	cases := []struct {
		name           string
		largeProject   bool
		largePrompt    bool
		wantAdditional string
	}{
		{name: "normal"},
		{name: "large prompt", largePrompt: true, wantAdditional: "This prompt is large"},
		{name: "large project", largeProject: true, wantAdditional: "Large project detected"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Focus = protocol.FocusDesign
			if tc.largePrompt {
				cfg.LargePromptByteThreshold = 8
			}
			m := NewModel("/tmp/project", cfg)
			m.state = stateScanComplete
			m.schemaA = "small topology"
			if tc.largePrompt {
				m.schemaA = strings.Repeat("x", 64)
			}
			m.largeProjectPending = tc.largeProject
			m.goalAttachments = []goalAttachment{newGoalReviewAttachment(
				"review context", "review payload", 3, 9, 2, nil,
			)}
			m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{
				Modules: []model.Module{{Name: "app", FileCount: 3}},
			})

			view := m.viewScanComplete()
			for _, want := range []string{
				"Review context: 3 changed files, +9/-2, 14B, 1 line",
				tc.wantAdditional,
			} {
				if want != "" && !strings.Contains(view, want) {
					t.Fatalf("scan-complete view missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestScanCompleteWithoutReviewAttachmentDoesNotShowReviewSummary(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.state = stateScanComplete
	m.schemaA = "small topology"
	m.goalAttachments = []goalAttachment{newGoalTextAttachment("notes", "review notes")}
	m.eng = engine.FromTopology("/tmp/project", &model.ProjectTopology{})

	view := m.viewScanComplete()
	if strings.Contains(view, "Review context:") {
		t.Fatalf("non-review scan-complete view showed review summary:\n%s", view)
	}
}
