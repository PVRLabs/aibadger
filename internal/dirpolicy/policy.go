package dirpolicy

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

// Policy is immutable after construction. Its zero value enables built-ins.
type Policy struct {
	disabled   bool
	additional map[string]bool
}

func New(enabled *bool, additional []string) Policy {
	p := Policy{disabled: enabled != nil && !*enabled, additional: make(map[string]bool)}
	for _, name := range additional {
		p.additional[strings.ToLower(name)] = true
	}
	return p
}

func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." || strings.TrimSpace(name) != name || strings.ContainsAny(name, "/\\*?[]!:") || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return fmt.Errorf("directory exclusion %q must be a single directory name without patterns or path separators", name)
	}
	return nil
}

func (p Policy) Skip(name string) bool {
	return p.additional[strings.ToLower(name)] || (!p.disabled && SkipGenerated(name))
}

// SkipLegacy retains independent exclusions while allowing the toggle to
// disable generated names that older detectors also listed locally.
func (p Policy) SkipLegacy(name string, legacy map[string]bool) bool {
	return p.Skip(name) || (legacy[strings.ToLower(name)] && !(p.disabled && SkipGenerated(name)))
}

func (p Policy) SkipPath(path string) bool {
	return p.SkipDirectoryPath(filepath.Dir(path))
}

func (p Policy) SkipDirectoryPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if p.Skip(part) {
			return true
		}
	}
	return false
}

func Resolve(policies ...Policy) Policy {
	if len(policies) > 0 {
		return policies[0]
	}
	return Policy{}
}
