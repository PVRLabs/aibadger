// Package skills contains the official Agent Skills bundled with Badger.
package skills

import _ "embed"

// Definition is one official Agent Skill definition bundled with Badger.
type Definition struct {
	Name    string
	Content string
}

//go:embed handoff/SKILL.md
var handoff []byte

//go:embed badger-code-review/SKILL.md
var badgerCodeReview []byte

// Definitions returns the official definitions in deterministic installation
// order. The returned slice is independent of the package's internal state.
func Definitions() []Definition {
	return []Definition{
		{Name: "handoff", Content: string(handoff)},
		{Name: "badger-code-review", Content: string(badgerCodeReview)},
	}
}
