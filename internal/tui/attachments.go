package tui

import (
	"fmt"
	"strings"

	"github.com/PVRLabs/aibadger/internal/protocol"
)

type goalAttachmentType string

const (
	goalAttachmentGitDiff goalAttachmentType = "git diff"
	goalAttachmentText    goalAttachmentType = "text"
)

type goalAttachment struct {
	Type         goalAttachmentType
	Source       string
	Summary      string
	Text         string
	SizeBytes    int64
	Lines        int
	FilesChanged int
	Additions    int
	Deletions    int
}

func newGoalTextAttachment(source, text string) goalAttachment {
	return newGoalAttachment(goalAttachmentText, source, text, 0, 0, 0)
}

func newGoalGitDiffAttachment(source, diff string) goalAttachment {
	return newGoalAttachment(goalAttachmentGitDiff, source, diff, 0, 0, 0)
}

func newGoalGitDiffAttachmentWithStats(source, diff string, filesChanged, additions, deletions int) goalAttachment {
	return newGoalAttachment(goalAttachmentGitDiff, source, diff, filesChanged, additions, deletions)
}

func newGoalAttachment(kind goalAttachmentType, source, text string, filesChanged, additions, deletions int) goalAttachment {
	attachment := goalAttachment{
		Type:         kind,
		Source:       strings.TrimSpace(source),
		Text:         text,
		SizeBytes:    int64(len(text)),
		Lines:        countTextLines(text),
		FilesChanged: filesChanged,
		Additions:    additions,
		Deletions:    deletions,
	}
	attachment.Summary = formatGoalAttachmentSummary(attachment)
	return attachment
}

func formatGoalAttachmentSummary(attachment goalAttachment) string {
	switch attachment.Type {
	case goalAttachmentGitDiff:
		if attachment.FilesChanged > 0 || attachment.Additions > 0 || attachment.Deletions > 0 {
			return fmt.Sprintf("[git diff: %d files changed, +%d/-%d]", attachment.FilesChanged, attachment.Additions, attachment.Deletions)
		}
		return fmt.Sprintf("[git diff: %s, %d lines]", protocol.FormatFileSize(attachment.SizeBytes), attachment.Lines)
	case goalAttachmentText:
		fallthrough
	default:
		return fmt.Sprintf("[text: %s, %d lines]", protocol.FormatFileSize(attachment.SizeBytes), attachment.Lines)
	}
}

func assembleGoalSubmission(instruction string, attachments []goalAttachment) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" && len(attachments) == 0 {
		return ""
	}

	parts := make([]string, 0, 1+len(attachments)*2)
	if instruction != "" {
		parts = append(parts, instruction)
	}
	for _, attachment := range attachments {
		if attachment.Text == "" {
			continue
		}
		if len(parts) > 0 {
			parts = append(parts, "")
		}
		parts = append(parts, renderGoalAttachment(attachment))
	}
	return strings.Join(parts, "\n")
}

func renderGoalAttachment(attachment goalAttachment) string {
	return strings.Join([]string{
		fmt.Sprintf("Attached %s:", attachment.Type),
		renderGoalAttachmentBlock(attachment),
	}, "\n")
}

func renderGoalAttachmentBlock(attachment goalAttachment) string {
	fence := goalAttachmentFence(attachment.Text)
	lang := attachmentFenceLanguage(attachment.Type)
	if lang != "" {
		return strings.Join([]string{
			fence + lang,
			attachment.Text,
			fence,
		}, "\n")
	}
	return strings.Join([]string{
		fence,
		attachment.Text,
		fence,
	}, "\n")
}

func attachmentFenceLanguage(kind goalAttachmentType) string {
	switch kind {
	case goalAttachmentGitDiff:
		return "diff"
	case goalAttachmentText:
		return "text"
	default:
		return ""
	}
}

func goalAttachmentFence(text string) string {
	maxRun := 0
	run := 0
	for _, r := range text {
		if r == '`' {
			run++
			if run > maxRun {
				maxRun = run
			}
			continue
		}
		run = 0
	}
	fenceLen := maxRun + 1
	if fenceLen < 3 {
		fenceLen = 3
	}
	return strings.Repeat("`", fenceLen)
}

func removeGoalAttachmentAt(attachments []goalAttachment, index int) []goalAttachment {
	if index < 0 || index >= len(attachments) {
		return append([]goalAttachment(nil), attachments...)
	}
	next := make([]goalAttachment, 0, len(attachments)-1)
	next = append(next, attachments[:index]...)
	next = append(next, attachments[index+1:]...)
	return next
}

func countTextLines(text string) int {
	if text == "" {
		return 0
	}
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return 0
	}
	return strings.Count(text, "\n") + 1
}
