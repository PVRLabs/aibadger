// This file canonicalizes module contents for deterministic topology identity
// and tie-breaking across context ownership, sorting, and deduplication.
package scanner

import (
	"encoding/json"
	"sort"

	"github.com/PVRLabs/aibadger/internal/model"
)

// stableModuleContentIdentity returns a canonical module identity for deterministic
// topology tie-breaking. Enrichment roots can be excluded when their contents
// should not affect a module's identity.
func stableModuleContentIdentity(module model.Module, excludeEnrichment bool) string {
	stable := module
	stable.SourceRoots = nil
	stable.TopFiles = append([]model.FileSummary(nil), module.TopFiles...)
	stable.AuxFiles = append([]model.FileSummary(nil), module.AuxFiles...)
	sortStableFileSummaries(stable.TopFiles)
	sortStableFileSummaries(stable.AuxFiles)
	for _, sourceRoot := range module.SourceRoots {
		if excludeEnrichment && isEnrichmentSourceRootRole(sourceRoot.Role) {
			continue
		}
		rootCopy := sourceRoot
		rootCopy.Packages = make([]model.Package, len(sourceRoot.Packages))
		for idx, pkg := range sourceRoot.Packages {
			pkgCopy := pkg
			pkgCopy.TopFiles = append([]model.FileSummary(nil), pkg.TopFiles...)
			pkgCopy.AuxFiles = append([]model.FileSummary(nil), pkg.AuxFiles...)
			sortStableFileSummaries(pkgCopy.TopFiles)
			sortStableFileSummaries(pkgCopy.AuxFiles)
			rootCopy.Packages[idx] = pkgCopy
		}
		sort.Slice(rootCopy.Packages, func(i, j int) bool {
			return packageSortKey(rootCopy.Packages[i]) < packageSortKey(rootCopy.Packages[j])
		})
		stable.SourceRoots = append(stable.SourceRoots, rootCopy)
	}
	sort.Slice(stable.SourceRoots, func(i, j int) bool {
		return sourceRootSortKey(stable.SourceRoots[i]) < sourceRootSortKey(stable.SourceRoots[j])
	})
	encoded, _ := json.Marshal(stable)
	return string(encoded)
}

func isEnrichmentSourceRootRole(role string) bool {
	switch role {
	case "Documentation", "Web Resources", "Ops/Deploy", "Resources", projectContextRole:
		return true
	default:
		return false
	}
}

func sortStableFileSummaries(files []model.FileSummary) {
	sort.Slice(files, func(i, j int) bool {
		if files[i].Path != files[j].Path {
			return files[i].Path < files[j].Path
		}
		if files[i].Name != files[j].Name {
			return files[i].Name < files[j].Name
		}
		if files[i].Kind != files[j].Kind {
			return files[i].Kind < files[j].Kind
		}
		return files[i].Size < files[j].Size
	})
}
