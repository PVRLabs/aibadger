// Package pathmatch implements the shared review and Code Map exclusion grammar.
package pathmatch

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// Validate checks relative slash-separated patterns without normalizing user text.
// Only * and whole-component ** are wildcards; all other accepted punctuation
// is literal. Validation is OS-independent, including Windows path rejection.
func Validate(patterns []string) error {
	for _, pattern := range patterns {
		drive := len(pattern) >= 2 && ((pattern[0] >= 'a' && pattern[0] <= 'z') || (pattern[0] >= 'A' && pattern[0] <= 'Z')) && pattern[1] == ':'
		invalid := pattern == "" || strings.HasPrefix(pattern, "/") || drive ||
			strings.ContainsAny(pattern, "?[]\\") || strings.ContainsFunc(pattern, unicode.IsControl)
		for _, component := range strings.Split(pattern, "/") {
			invalid = invalid || component == "" || component == "." || component == ".." ||
				(component != "**" && strings.Contains(component, "**"))
		}
		if invalid {
			return fmt.Errorf("invalid --exclude pattern %q: use relative /-separated paths, * within components and ** as a whole component; ?, [], backslashes and absolute paths are not supported", pattern)
		}
	}
	return nil
}

// Matches accepts validated patterns and a relative slash-separated entry path.
// Matching an ancestor excludes descendants, giving review paths the same
// directory semantics as a filesystem walk that prunes matching directories.
func Matches(patterns []string, name string) bool {
	if len(patterns) == 0 {
		return false
	}
	components := strings.Split(name, "/")
	for _, pattern := range patterns {
		if !strings.Contains(pattern, "/") {
			if pattern == "**" {
				return true
			}
			for _, component := range components {
				if matched, _ := path.Match(pattern, component); matched {
					return true
				}
			}
		} else if matchExcludeComponents(strings.Split(pattern, "/"), components) {
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
	for _, prefix := range matched[1:] {
		if prefix {
			return true
		}
	}
	return false
}
