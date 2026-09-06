package startup

// Context groups the launch-time seed data passed from CLI preparation into
// the TUI and headless runners.
type Context struct {
	Goal string
	// LiteralGoal makes the seeded goal bypass slash-command completion and
	// dispatch for its first submission.
	LiteralGoal bool
	Attachments []Attachment
	Status      Status
	// ReviewMode, ReviewRef, and ReviewExtraFocus preserve the review scope
	// used to prepare this context so an interactive session can refresh it.
	// They are optional metadata and do not affect non-review startup flows.
	ReviewMode          string
	ReviewRef           string
	ReviewExtraFocus    string
	ReviewSelectedPaths []string
}

// Attachment is a goal attachment prepared before the TUI starts.
type Attachment struct {
	Type         string
	Source       string
	Text         string
	SizeBytes    int64
	Lines        int
	FilesChanged int
	Additions    int
	Deletions    int
	// ReviewGenerated identifies context owned by the review builder. The TUI
	// may replace these attachments when the user refreshes a review.
	ReviewGenerated bool
	// SensitivePaths lists tracked repository-relative paths whose diffs may
	// contain secrets. It is metadata for interactive delivery only; the
	// attachment text remains complete and authoritative.
	SensitivePaths []string
}

// Status is the user-facing message shown after startup preparation.
type Status struct {
	Text     string
	Severity string
}
