package scanner

// This file owns the final topology-level fields and finalization pipeline.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/model"
)

func (s *Scanner) finalizeTopology(t *model.ProjectTopology) {
	if len(t.Modules) == 0 {
		t.Name = projectName(s.ProjectRoot)
		t.Languages = []string{"Unknown"}
		t.PrimaryLanguage = "Unknown"
		t.Structure = "Unknown"
		return
	}

	if t.Name == "" {
		t.Name = projectName(s.ProjectRoot)
	}

	weights := make(map[string]int64)
	languageSet := make(map[string]bool)
	for _, lang := range t.Languages {
		if lang != "" {
			languageSet[lang] = true
		}
	}
	for _, m := range t.Modules {
		if m.Language == "" {
			continue
		}
		languageSet[m.Language] = true
		weights[m.Language] += int64(m.FileCount)
	}

	primary := dominantLanguage(weights)
	if primary == "" && len(languageSet) == 1 {
		primary = sortedLanguages(languageSet, "")[0]
	}
	if primary == "" {
		primary = "Unknown"
	}

	t.Languages = sortedLanguages(languageSet, primary)
	t.PrimaryLanguage = primary
	if len(t.Modules) > 1 || isMavenMultiModule(s.ProjectRoot) {
		t.Structure = "Multi-Module"
	} else {
		t.Structure = "Single Module"
	}
	if len(t.Stack) == 0 {
		t.Stack = detectedStack(s.ProjectRoot)
	}
	deduplicateTopologyFiles(t)
	sortTopology(t)
}

func sortedLanguages(languageSet map[string]bool, primary string) []string {
	if len(languageSet) == 0 {
		return []string{primary}
	}
	languages := make([]string, 0, len(languageSet))
	for lang := range languageSet {
		languages = append(languages, lang)
	}
	sort.Strings(languages)
	return languages
}

func dominantLanguage(weights map[string]int64) string {
	if len(weights) == 0 {
		return ""
	}
	languages := make([]string, 0, len(weights))
	for lang := range weights {
		languages = append(languages, lang)
	}
	sort.Strings(languages)

	primary := ""
	var maxWeight int64
	for _, lang := range languages {
		if weights[lang] > maxWeight {
			maxWeight = weights[lang]
			primary = lang
		}
	}
	return primary
}

func projectName(root string) string {
	name := filepath.Base(root)
	if name == "." || name == string(filepath.Separator) {
		return ""
	}
	return name
}

func isMavenMultiModule(root string) bool {
	content, err := os.ReadFile(filepath.Join(root, "pom.xml"))
	if err != nil {
		return false
	}
	text := string(content)
	return strings.Contains(text, "<packaging>pom</packaging>") && strings.Contains(text, "<modules>")
}
