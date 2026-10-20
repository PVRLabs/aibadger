package reviewtask

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// ValidateExcludePatterns checks the small review glob language. Slashless
// patterns match basenames; other patterns are rooted at the repository.
// A whole ** component matches zero or more path components. Other components
// use path.Match (*, ?, character classes, and backslash escapes).
func ValidateExcludePatterns(patterns []string) error {
	for _, pattern := range patterns {
		invalid := pattern == "" || strings.HasPrefix(pattern, "/") || strings.ContainsFunc(pattern, unicode.IsControl)
		for _, component := range strings.Split(pattern, "/") {
			if component == "**" {
				continue
			}
			_, err := path.Match(component, "")
			invalid = invalid || component == "" || component == "." || component == ".." || strings.Contains(component, "**") || err != nil
		}
		if invalid {
			return fmt.Errorf("invalid --exclude pattern %q: use repository-relative globs with ** only as a whole path component", pattern)
		}
	}
	return nil
}

func matchesExclusions(patterns []string, name string) bool {
	for _, pattern := range patterns {
		if !strings.Contains(pattern, "/") {
			if pattern == "**" {
				return true
			}
			if matched, _ := path.Match(pattern, path.Base(name)); matched {
				return true
			}
			continue
		}
		if matchExcludeComponents(strings.Split(pattern, "/"), strings.Split(name, "/")) {
			return true
		}
	}
	return false
}

func matchExcludeComponents(pattern, name []string) bool {
	// Dynamic programming bounds repeated ** matching to O(pattern * path).
	matched := make([]bool, len(name)+1)
	matched[0] = true
	for _, component := range pattern {
		next := make([]bool, len(name)+1)
		if component == "**" {
			next[0] = matched[0]
			for j := 1; j <= len(name); j++ {
				next[j] = matched[j] || next[j-1]
			}
		} else {
			for j := 1; j <= len(name); j++ {
				if matched[j-1] {
					next[j], _ = path.Match(component, name[j-1])
				}
			}
		}
		matched = next
	}
	return matched[len(name)]
}
