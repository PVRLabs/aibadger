package diagnose

import (
	"regexp"
	"strings"
)

var (
	versionPattern       = regexp.MustCompile(`\d+(?:\.\d+)+(?:[-+._][0-9A-Za-z]+)*`)
	simpleVersionPattern = regexp.MustCompile(`^v?(\d+(?:\.\d+)+(?:[-+._][0-9A-Za-z]+)*)$`)
	goVersionPattern     = regexp.MustCompile(`\bgo(\d+(?:\.\d+){1,2}(?:(?:beta|rc)\d+)?)\b`)
	dotnetSDKPattern     = regexp.MustCompile(`^\s*(\d+(?:\.\d+)+(?:[-+._][0-9A-Za-z]+)*)\s+\[[^\]]+\]\s*$`)
)

func parseGit(output string) (string, bool)    { return afterPrefix(output, "git version") }
func parsePython(output string) (string, bool) { return afterPrefix(output, "Python") }

func parseGo(output string) (string, bool) {
	match := goVersionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func parseJava(output string) (string, bool) {
	for _, line := range lines(output) {
		if strings.Contains(strings.ToLower(line), "version") {
			if value := versionPattern.FindString(line); value != "" {
				return value, true
			}
		}
	}
	return "", false
}

func parseSimpleVersion(output string) (string, bool) {
	match := simpleVersionPattern.FindStringSubmatch(strings.TrimSpace(output))
	if len(match) != 2 {
		return "", false
	}
	return match[1], true
}

func parsePip(output string) (string, bool) { return afterPrefix(output, "pip") }

func parseDotnetSDKs(output string) (string, bool) {
	var versions []string
	seen := make(map[string]bool)
	for _, line := range lines(output) {
		match := dotnetSDKPattern.FindStringSubmatch(line)
		if len(match) != 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		versions = append(versions, match[1])
	}
	if len(versions) == 0 {
		return "", false
	}
	return strings.Join(versions, ", "), true
}

func parseCompiler(output string) (string, bool) {
	first := ""
	for _, line := range lines(output) {
		if line != "" {
			first = line
			break
		}
	}
	version := versionPattern.FindString(first)
	if version == "" {
		return "", false
	}
	lower := strings.ToLower(first)
	switch {
	case strings.Contains(lower, "clang"):
		return "Clang " + version, true
	case strings.Contains(lower, "gcc") || strings.Contains(lower, "free software foundation"):
		return "GCC " + version, true
	case strings.Contains(lower, "microsoft") || strings.Contains(lower, "compiler version"):
		return "MSVC " + version, true
	default:
		return "Compiler " + version, true
	}
}

func prefixedVersion(prefix string) func(string) (string, bool) {
	return func(output string) (string, bool) { return afterPrefix(output, prefix) }
}

func afterPrefix(output, prefix string) (string, bool) {
	for _, line := range lines(output) {
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(prefix)) {
			value := versionPattern.FindString(strings.TrimPrefix(line, prefix))
			return value, value != ""
		}
	}
	return "", false
}

func lines(output string) []string {
	return strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
}
