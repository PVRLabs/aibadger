package startup

// Context groups the launch-time seed data passed from CLI preparation into
// the TUI and headless runners.
type Context struct {
	Goal        string
	Attachments []Attachment
	Status      Status
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
}

// Status is the user-facing message shown after startup preparation.
type Status struct {
	Text     string
	Severity string
}
