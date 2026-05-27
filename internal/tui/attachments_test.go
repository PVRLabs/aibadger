package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
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
		{name: "windows newlines", text: "one\r\ntwo\r\nthree", want: 3},
		{name: "carriage return newlines", text: "one\rtwo\rthree", want: 3},
		{name: "trailing carriage return ignored", text: "one\rtwo\r", want: 2},
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

func TestViewGoalAttachmentsHidesEmptyState(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())

	if got := m.viewGoalAttachments(); got != "" {
		t.Fatalf("viewGoalAttachments() = %q, want empty", got)
	}
	if strings.Contains(m.viewHome(), "Attachments:") {
		t.Fatalf("home view showed empty attachment section:\n%s", m.viewHome())
	}
}

func TestViewGoalAttachmentsRendersSelectedRow(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("clipboard", "first attachment"),
		newGoalGitDiffAttachmentWithStats("git diff", "diff --git a/a.go b/a.go\n", 1, 2, 1),
	}
	m.goalFocus = goalFocusAttachments
	m.goalAttachmentSelected = 1

	view := m.viewHome()

	if !strings.Contains(view, "Attachments:") {
		t.Fatalf("home view missing attachments header:\n%s", view)
	}
	if !strings.Contains(view, "clipboard · [text:") {
		t.Fatalf("home view missing first compact attachment row:\n%s", view)
	}
	if !strings.Contains(view, "> [git diff: 1 files changed, +2/-1]") {
		t.Fatalf("home view missing selected attachment row:\n%s", view)
	}
}

func TestViewGoalAttachmentsDoesNotShowSelectionWhileEditingGoal(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("clipboard", "first attachment"),
	}
	m.goalFocus = goalFocusEditor
	m.goalAttachmentSelected = 0

	view := m.viewHome()

	if strings.Contains(view, "> [text]") {
		t.Fatalf("home view showed attachment selection while editing goal:\n%s", view)
	}
}

func TestRenderGoalAttachmentRowTruncatesSafely(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.width = 28
	attachment := newGoalTextAttachment("clip🔧board", strings.Repeat("世界", 10))

	got := m.renderGoalAttachmentRow(attachment, false)

	if !utf8.ValidString(got) {
		t.Fatal("rendered attachment row is not valid UTF-8")
	}
	if runewidth.StringWidth(got) > m.goalAttachmentRowWidth() {
		t.Fatalf("rendered row width = %d, want <= %d\n%s", runewidth.StringWidth(got), m.goalAttachmentRowWidth(), got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("rendered row did not truncate long text:\n%s", got)
	}
}

func TestGoalAttachmentFocusNavigationAndDeletion(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this change.")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "first"),
		newGoalTextAttachment("paste", "second"),
		newGoalTextAttachment("paste", "third"),
	}
	m.goalAttachmentSelected = 1
	m.goalFocus = goalFocusEditor
	m.goalInput.Focus()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Tab) returned %T, want tui.Model", next)
	}
	if got.goalInput.Focused() {
		t.Fatal("goal input remained focused after moving to attachments")
	}
	if got.goalFocus != goalFocusAttachments {
		t.Fatalf("goalFocus = %v, want %v", got.goalFocus, goalFocusAttachments)
	}
	if got.goalAttachmentSelected != 1 {
		t.Fatalf("goalAttachmentSelected = %d, want 1", got.goalAttachmentSelected)
	}
	if cmd == nil {
		t.Fatal("tab into attachments did not return a blink command")
	}

	next, cmd = got.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKey(Down) returned %T, want tui.Model", next)
	}
	if got.goalAttachmentSelected != 2 {
		t.Fatalf("goalAttachmentSelected after down = %d, want 2", got.goalAttachmentSelected)
	}
	if cmd == nil {
		t.Fatal("attachment navigation did not return a blink command")
	}

	next, _ = got.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKey(Down) returned %T, want tui.Model", next)
	}
	if got.goalAttachmentSelected != 2 {
		t.Fatalf("goalAttachmentSelected after clamped down = %d, want 2", got.goalAttachmentSelected)
	}

	next, _ = got.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKey(Up) returned %T, want tui.Model", next)
	}
	if got.goalAttachmentSelected != 1 {
		t.Fatalf("goalAttachmentSelected after up = %d, want 1", got.goalAttachmentSelected)
	}

	next, _ = got.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	got, ok = next.(Model)
	if !ok {
		t.Fatalf("handleKey(Esc) returned %T, want tui.Model", next)
	}
	if got.goalFocus != goalFocusEditor {
		t.Fatalf("goalFocus after esc = %v, want %v", got.goalFocus, goalFocusEditor)
	}
	if !got.goalInput.Focused() {
		t.Fatal("goal input was not refocused after esc")
	}
}

func TestGoalAttachmentUpOnFirstItemReturnsToEditor(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this change.")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "first"),
		newGoalTextAttachment("paste", "second"),
	}
	m.goalAttachmentSelected = 0
	m.goalFocus = goalFocusAttachments
	m.goalInput.Blur()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Up) returned %T, want tui.Model", next)
	}
	if got.goalFocus != goalFocusEditor {
		t.Fatalf("goalFocus after up on first attachment = %v, want %v", got.goalFocus, goalFocusEditor)
	}
	if !got.goalInput.Focused() {
		t.Fatal("goal input was not refocused after leaving the first attachment")
	}
	if got.goalAttachmentSelected != 0 {
		t.Fatalf("goalAttachmentSelected = %d, want 0", got.goalAttachmentSelected)
	}
	if cmd == nil {
		t.Fatal("up on first attachment did not return a blink command")
	}
}

func TestGoalAttachmentDeletionKeepsNearestSelectionAndSubmitsRemainingAttachments(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this change.")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "first"),
		newGoalTextAttachment("paste", "second"),
		newGoalTextAttachment("paste", "third"),
	}
	m.goalAttachmentSelected = 2
	m.goalFocus = goalFocusAttachments
	m.goalInput.Blur()

	next, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Backspace) returned %T, want tui.Model", next)
	}
	if len(got.goalAttachments) != 2 {
		t.Fatalf("goalAttachments length after delete = %d, want 2", len(got.goalAttachments))
	}
	if got.goalAttachmentSelected != 1 {
		t.Fatalf("goalAttachmentSelected after delete = %d, want 1", got.goalAttachmentSelected)
	}
	if got.goalAttachments[1].Text != "second" {
		t.Fatalf("nearest selection fallback picked %q, want second", got.goalAttachments[1].Text)
	}
	if got.goalFocus != goalFocusAttachments {
		t.Fatalf("goalFocus after delete = %v, want %v", got.goalFocus, goalFocusAttachments)
	}
	if got.goalInput.Focused() {
		t.Fatal("goal input should stay blurred while attachments remain")
	}
	if cmd == nil {
		t.Fatal("attachment delete did not return a blink command")
	}

	next, _ = got.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	submitted, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Enter) returned %T, want tui.Model", next)
	}
	if submitted.state != stateScanning {
		t.Fatalf("state after submit = %v, want %v", submitted.state, stateScanning)
	}
	if strings.Contains(submitted.goal, "third") {
		t.Fatalf("submitted goal still contained removed attachment:\n%s", submitted.goal)
	}
	if !strings.Contains(submitted.goal, "Attached text:") || !strings.Contains(submitted.goal, "first") || !strings.Contains(submitted.goal, "second") {
		t.Fatalf("submitted goal missing remaining attachments:\n%s", submitted.goal)
	}
}

func TestDownOnlyEntersAttachmentsFromLastTextareaLine(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("First line\nSecond line")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "attached context"),
	}
	m.goalFocus = goalFocusEditor
	m.goalInput.Focus()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	afterUp, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Up) returned %T, want tui.Model", next)
	}

	next, _ = afterUp.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Down) returned %T, want tui.Model", next)
	}
	if got.goalFocus != goalFocusEditor {
		t.Fatalf("goalFocus = %v, want %v when cursor is not on last line", got.goalFocus, goalFocusEditor)
	}
}

func TestDeletingLastAttachmentReturnsFocusToEditor(t *testing.T) {
	m := NewModel("/tmp/project", DefaultConfig())
	m.goalInput.SetValue("Review this change.")
	m.goalAttachments = []goalAttachment{
		newGoalTextAttachment("paste", "only attachment"),
	}
	m.goalAttachmentSelected = 0
	m.goalFocus = goalFocusAttachments
	m.goalInput.Blur()

	next, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDelete})
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("handleKey(Delete) returned %T, want tui.Model", next)
	}
	if len(got.goalAttachments) != 0 {
		t.Fatalf("goalAttachments length = %d, want 0", len(got.goalAttachments))
	}
	if got.goalFocus != goalFocusEditor {
		t.Fatalf("goalFocus after deleting last attachment = %v, want %v", got.goalFocus, goalFocusEditor)
	}
	if !got.goalInput.Focused() {
		t.Fatal("goal input was not refocused after removing last attachment")
	}
	if strings.Contains(got.viewHome(), "Attachments:") {
		t.Fatalf("home view still showed attachment section after deleting last item:\n%s", got.viewHome())
	}
}
