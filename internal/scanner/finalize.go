package scanner

// This file owns the final topology-level fields and finalization pipeline.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

func (s *Scanner) finalizeTopology(t *model.ProjectTopology) {
	s.finalizeTopologyWithLanguageWeights(t, nil)
}

func (s *Scanner) finalizeTopologyWithLanguageWeights(t *model.ProjectTopology, languageWeights map[string]int64) {
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
		if languageWeights != nil {
			continue
		}
		weights[m.Language] += int64(m.FileCount)
	}
	if languageWeights != nil {
		for lang, weight := range languageWeights {
			if lang == "" {
				continue
			}
			languageSet[lang] = true
			weights[lang] = weight
		}
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

func isLanguageOwnedSourceRoot(role string) bool {
	switch role {
	case "Documentation", "Web Resources", "Ops/Deploy", "Resources":
		return false
	default:
		return true
	}
}

func sourceLanguageWeightsFromModules(modules []model.Module, projectRoot string) map[string]int64 {
	weights := make(map[string]int64)
	for _, module := range modules {
		if module.Language == "" {
			continue
		}
		weights[module.Language] += int64(countModuleLanguageSourceFiles(module, projectRoot))
	}
	return weights
}

func countModuleLanguageSourceFiles(module model.Module, projectRoot string) int {
	seen := make(map[string]bool)
	for _, sourceRoot := range module.SourceRoots {
		if !isLanguageOwnedSourceRoot(sourceRoot.Role) {
			continue
		}
		for _, pkg := range sourceRoot.Packages {
			countLanguageSourceFilesInDir(module.Language, projectRoot, pkg.Path, seen)
		}
	}
	return len(seen)
}

func countLanguageSourceFilesInDir(language, projectRoot, relDir string, seen map[string]bool) {
	fullDir := filepath.Join(projectRoot, relDir)
	entries, err := os.ReadDir(fullDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !isLanguageSourceFile(language, filepath.Join(relDir, entry.Name())) {
			continue
		}
		seen[normalizeTopologyFilePath(filepath.Join(relDir, entry.Name()))] = true
	}
}

func isLanguageSourceFile(language, path string) bool {
	name := filepath.Base(path)
	switch language {
	case "Go":
		return isGoSourceFile(name)
	case "Java":
		return isJavaSourceFile(name)
	case "Python":
		return isPythonSourceFile(name)
	case "JavaScript":
		return isNodeSourceFile(name) && !isTypeScriptSourceFile(name)
	case "TypeScript":
		return isTypeScriptSourceFile(name)
	case "Generic":
		kind := filekind.Classify(path)
		return kind != model.FileKindAsset && kind != model.FileKindBinary
	default:
		return false
	}
}

func isTypeScriptSourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".tsx", ".cts", ".mts":
		return true
	default:
		return false
	}
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
