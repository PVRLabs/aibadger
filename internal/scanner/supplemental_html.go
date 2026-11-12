package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/defaults"
	"github.com/PVRLabs/aibadger/internal/dirpolicy"
	"github.com/PVRLabs/aibadger/internal/model"
)

func htmlEntrypoint(name string) bool {
	return strings.EqualFold(name, "index.html") || strings.EqualFold(name, "index.htm")
}

func sortHTMLFiles(files []model.FileSummary) {
	sort.Slice(files, func(i, j int) bool {
		if htmlEntrypoint(files[i].Name) != htmlEntrypoint(files[j].Name) {
			return htmlEntrypoint(files[i].Name)
		}
		return filepath.ToSlash(files[i].Path) < filepath.ToSlash(files[j].Path)
	})
}

func addHTMLFile(files []model.FileSummary, file model.FileSummary, limit int) []model.FileSummary {
	files = append(files, file)
	sortHTMLFiles(files)
	if len(files) > limit {
		files = files[:limit]
	}
	return files
}

// collectSupplementalHTML has independent traversal counters. Pruned subtrees
// never spend its budget; ordinary regular files do, irrespective of extension.
func (d *GenericDetector) collectSupplementalHTML(root string, ownership semanticSourceOwnership) ([]model.SourceRoot, error) {
	packages := make(map[string]*model.Package)
	reached := make(map[string]bool)
	perDir := make(map[string]int)
	total, maxTotal := 0, d.maxTotalFiles
	if maxTotal <= 0 {
		maxTotal = defaults.MaxTotalScanFiles
	}
	err := walkDiscoveryDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel := relativePath(root, path)
		if entry.IsDir() && path != root {

			if shouldSkipDir(entry.Name(), d.Exclusions, d.directoryPolicy) || isSharedWebResourcePath(rel) || ownership.ownsWebResource(path) || equivalentConventionalWebDir(root, path, ownership) {
				return filepath.SkipDir
			}
		}
		if d.maxFilesPerDir > 0 {
			dir := filepath.Dir(path)
			perDir[dir]++
			if perDir[dir] > d.maxFilesPerDir {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		total++
		if total > maxTotal {
			return filepath.SkipAll
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if (ext != ".html" && ext != ".htm") || shouldOmitFile(root, path, entry.Name(), d.directoryPolicy) {
			return nil
		}
		dir := normalizeRelativeDir(filepath.Dir(rel))
		pkg := packages[dir]
		if pkg == nil {
			pkg = &model.Package{Name: webPackageName(dir, dir), Path: dir, SupplementalHTML: true}
			packages[dir] = pkg
		}
		reached[normalizeTopologyFilePath(rel)] = true
		pkg.FileCount++
		pkg.TopFiles = addHTMLFile(pkg.TopFiles, model.FileSummary{Name: entry.Name(), Path: rel, Size: info.Size()}, packageTopFileLimit(dir, effectiveTopFileLimit(d.maxTopFiles)))
		return nil
	}, d.directoryPolicy, root)
	sr := model.SourceRoot{Role: "Web Resources", SupplementalHTML: true, HTMLPaths: reached}
	for _, pkg := range packages {
		sr.Packages = append(sr.Packages, *pkg)
		sr.FileCount += pkg.FileCount
	}
	sort.Slice(sr.Packages, func(i, j int) bool { return sr.Packages[i].Path < sr.Packages[j].Path })
	if sr.FileCount == 0 {
		return nil, err
	}
	return []model.SourceRoot{sr}, err
}

// Transfer Node overview accounting even when its representative cap hid HTML.
func transferNodeHTML(topology *model.ProjectTopology, root string, reached map[string]bool, directoryPolicies ...dirpolicy.Policy) {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	for mi := range topology.Modules {
		m := &topology.Modules[mi]
		if m.Coverage || (m.Language != "JavaScript" && m.Language != "TypeScript") {
			continue
		}
		full := filepath.Join(root, m.Path)
		detector := NewNodeDetector(directoryPolicy)
		pkg, ok := detector.readPackageJSON(filepath.Join(full, "package.json"))
		if !ok {
			continue
		}
		seen := make(map[string]bool)
		for _, candidate := range nodeOverviewCandidates(full, pkg, directoryPolicy) {
			path := normalizeTopologyFilePath(relativePath(root, filepath.Join(full, candidate)))
			if !reached[path] {
				candidateInfo, err := os.Stat(filepath.Join(root, path))
				if err != nil {
					continue
				}
				for discovered := range reached {
					if !strings.EqualFold(discovered, path) {
						continue
					}
					info, err := os.Stat(filepath.Join(root, discovered))
					if err == nil && os.SameFile(candidateInfo, info) {
						path = discovered
						break
					}
				}
			}
			if !reached[path] || seen[path] {
				continue
			}
			seen[path] = true
			for si := range m.SourceRoots {
				sr := &m.SourceRoots[si]
				// addRootOverviewPackage attaches the overview to the first
				// structural source root, before shared context is appended.
				if si != 0 || !isLanguageOwnedSourceRoot(sr.Role) {
					continue
				}
				for pi := range sr.Packages {
					p := &sr.Packages[pi]
					if p.Path != "" {
						continue
					}
					p.FileCount--
					sr.FileCount--
					files := p.TopFiles[:0]
					for _, file := range p.TopFiles {
						if normalized := normalizeTopologyFilePath(file.Path); normalized != path && normalized != normalizeTopologyFilePath(relativePath(root, filepath.Join(full, candidate))) {
							files = append(files, file)
						}
					}
					p.TopFiles = files
				}
			}
		}
	}
}

// Directory aliases count as delegated coverage only when the filesystem agrees.
// EqualFold alone would hide distinct Public/ and public/ directories on Linux.
func equivalentConventionalWebDir(root, path string, ownership semanticSourceOwnership) bool {
	rel := relativePath(root, path)
	candidates := append(standaloneWebResourceDirs(), ownership.webRoots...)
	var actual os.FileInfo
	for _, candidate := range candidates {
		if !strings.EqualFold(rel, candidate) {
			continue
		}
		if actual == nil {
			var err error
			actual, err = os.Stat(path)
			if err != nil {
				return false
			}
		}
		expected, err := os.Stat(filepath.Join(root, candidate))
		if err == nil && os.SameFile(actual, expected) {
			return true
		}
	}
	return false
}
