package tui

import (
	"strings"
	"testing"
)

func TestCountTextLines(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "empty", text: "", want: 0},
		{name: "single line", text: "hello", want: 1},
		{name: "multiple lines", text: "one\ntwo\nthree", want: 3},
		{name: "trailing newline ignored", text: "one\ntwo\n", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countTextLines(tt.text); got != tt.want {
				t.Fatalf("countTextLines(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestNewGoalTextAttachmentSummarizesAndPreservesPayload(t *testing.T) {
	attachment := newGoalTextAttachment("clipboard", "line 1\nline 2\n")

	if attachment.Type != goalAttachmentText {
		t.Fatalf("Type = %q, want %q", attachment.Type, goalAttachmentText)
	}
	if attachment.Source != "clipboard" {
		t.Fatalf("Source = %q, want clipboard", attachment.Source)
	}
	if attachment.Text != "line 1\nline 2\n" {
		t.Fatalf("Text = %q, want exact payload preservation", attachment.Text)
	}
	if attachment.Lines != 2 {
		t.Fatalf("Lines = %d, want 2", attachment.Lines)
	}
	if attachment.SizeBytes != int64(len("line 1\nline 2\n")) {
		t.Fatalf("SizeBytes = %d, want %d", attachment.SizeBytes, len("line 1\nline 2\n"))
	}
	if !strings.Contains(attachment.Summary, "[text:") || !strings.Contains(attachment.Summary, "2 lines") {
		t.Fatalf("Summary = %q, want compact text summary", attachment.Summary)
	}
}

func TestNewGoalGitDiffAttachmentUsesDiffSummaryWhenStatsPresent(t *testing.T) {
	attachment := newGoalAttachment(goalAttachmentGitDiff, "badger review", "diff --git a/file.go b/file.go\n", 5, 12, 7)

	if attachment.Type != goalAttachmentGitDiff {
		t.Fatalf("Type = %q, want %q", attachment.Type, goalAttachmentGitDiff)
	}
	if attachment.Source != "badger review" {
		t.Fatalf("Source = %q, want badger review", attachment.Source)
	}
	if attachment.Summary != "[git diff: 5 files changed, +12/-7]" {
		t.Fatalf("Summary = %q, want diff stats summary", attachment.Summary)
	}
}

func TestRemoveGoalAttachmentAt(t *testing.T) {
	attachments := []goalAttachment{
		newGoalTextAttachment("paste", "first"),
		newGoalTextAttachment("paste", "second"),
		newGoalTextAttachment("paste", "third"),
	}

	got := removeGoalAttachmentAt(attachments, 1)

	if len(got) != 2 {
		t.Fatalf("len(removeGoalAttachmentAt) = %d, want 2", len(got))
	}
	if got[0].Text != "first" || got[1].Text != "third" {
		t.Fatalf("removeGoalAttachmentAt preserved wrong entries: %#v", got)
	}

	unchanged := removeGoalAttachmentAt(attachments, 99)
	if len(unchanged) != len(attachments) {
		t.Fatalf("out-of-range removal changed length: got %d want %d", len(unchanged), len(attachments))
	}
}

func TestAssembleGoalSubmissionIncludesAttachments(t *testing.T) {
	attachments := []goalAttachment{
		newGoalTextAttachment("paste", "first line\nsecond line"),
		newGoalAttachment(goalAttachmentGitDiff, "badger review", "diff --git a/a.go b/a.go\n@@ -1 +1 @@\n-a\n+b\n", 0, 0, 0),
	}

	got := assembleGoalSubmission("Review this change.", attachments)

	for _, want := range []string{
		"Review this change.",
		"Attached text:",
		"```text",
		"first line\nsecond line",
		"Attached git diff:",
		"```diff",
		"diff --git a/a.go b/a.go",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("assembled goal missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "attachment markers") {
		t.Fatalf("assembled goal unexpectedly contained placeholder marker text:\n%s", got)
	}
}

func TestSubmitGoalAssemblesAttachments(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this change.")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "attached context"),
	}

	next, _ := m.submitGoal()
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("submitGoal returned %T, want tui.Model", next)
	}
	if got.state != stateScanning {
		t.Fatalf("state = %v, want %v", got.state, stateScanning)
	}
	if !strings.Contains(got.goal, "Review this change.") {
		t.Fatalf("assembled goal missing instruction:\n%s", got.goal)
	}
	if !strings.Contains(got.goal, "Attached text:") || !strings.Contains(got.goal, "attached context") {
		t.Fatalf("assembled goal missing attachment payload:\n%s", got.goal)
	}
}
