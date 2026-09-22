// Package sessionimport defines the small, source-neutral boundary for importing
// conversation text into a Badger startup attachment.
package sessionimport

import (
	"errors"
	"time"
)

var (
	ErrNotFound         = errors.New("session not found")
	ErrLookupIncomplete = errors.New("session lookup stopped at work limit")
	ErrNoConversation   = errors.New("session has no usable conversation text")
)

type Summary struct {
	ID               string
	StartedAt        time.Time
	Label            string
	WorkingDirectory string
	ProjectMatch     bool
}

type Limits struct {
	// MaxOutputBytes includes the header and any truncation marker. Zero uses
	// the conservative default, below the 50 KiB extracted-file reference budget.
	MaxOutputBytes int
}

const DefaultMaxOutputBytes = 48 * 1024

type Conversation struct {
	Text          string
	SourceBytes   int64
	ImportedBytes int
	Truncated     bool
}
