package reviewtask

import "github.com/PVRLabs/aibadger/internal/pathmatch"

// ValidateExcludePatterns checks the shared review and Code Map grammar.
func ValidateExcludePatterns(patterns []string) error {
	return pathmatch.Validate(patterns)
}

func matchesExclusions(patterns []string, name string) bool {
	return pathmatch.Matches(patterns, name)
}
