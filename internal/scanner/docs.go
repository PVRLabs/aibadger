package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/PVRLabs/aibadger/internal/filekind"
	"github.com/PVRLabs/aibadger/internal/model"
)

// scanDocs finds shallow Markdown documentation files.
func scanDocs(root string) ([]model.SourceRoot, error) {
	docs := make(map[string][]model.FileSummary)

	// Scan root for *.md
	entries, err := os.ReadDir(root)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".md") {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if shouldOmitFile(root, path, entry.Name()) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			docs[""] = append(docs[""], docFileSummary(root, path, entry.Name(), info.Size()))
		}
	}

	docDirs := []string{"docs", "doc"}
	for _, dirName := range docDirs {
		dirPath := filepath.Join(root, dirName)
		info, err := os.Stat(dirPath)
		if err != nil || !info.IsDir() {
			continue
		}

		entries, err := os.ReadDir(dirPath)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".md") {
				continue
			}
			path := filepath.Join(dirPath, name)
			if shouldOmitFile(root, path, name) {
				continue
			}
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			docs[dirName] = append(docs[dirName], docFileSummary(root, path, name, info.Size()))
		}
	}

	sourceRoots := make([]model.SourceRoot, 0, len(docs))
	for sourceRootPath, files := range docs {
		if len(files) == 0 {
			continue
		}
		sort.Slice(files, func(i, j int) bool {
			return files[i].Path < files[j].Path
		})
		pkg := model.Package{
			Name:      sourceRootPath,
			Path:      sourceRootPath,
			FileCount: len(files),
		}
		for _, file := range files {
			pkg.TopFiles = addTopFile(pkg.TopFiles, file, 5) // Increased limit for docs
		}
		if len(pkg.TopFiles) > 0 {
			pkg.Heaviest = heaviestFromSummary(pkg.TopFiles[0])
		}
		sourceRoots = append(sourceRoots, model.SourceRoot{
			Path:      sourceRootPath,
			Role:      "Documentation",
			FileCount: len(files),
			Packages:  []model.Package{pkg},
		})
	}
	sort.Slice(sourceRoots, func(i, j int) bool {
		return sourceRoots[i].Path < sourceRoots[j].Path
	})
	return sourceRoots, nil
}

func docFileSummary(root, path, name string, size int64) model.FileSummary {
	return model.FileSummary{
		Name: name,
		Path: relativePath(root, path),
		Size: size,
		Kind: filekind.Classify(path),
	}
}

func attachDocsToTopology(topology *model.ProjectTopology, docs []model.SourceRoot) {
	if len(docs) == 0 || len(topology.Modules) == 0 {
		return
	}
	module := docsTargetModule(topology.Modules)
	if module == nil {
		return
	}
	for _, sourceRoot := range docs {
		mergeDocsSourceRoot(module, sourceRoot)
	}
}

func docsTargetModule(modules []model.Module) *model.Module {
	targetIdx := -1
	for idx := range modules {
		if modules[idx].Path == "" {
			return &modules[idx]
		}
		if targetIdx == -1 || modules[idx].Path < modules[targetIdx].Path {
			targetIdx = idx
		}
	}
	if targetIdx == -1 {
		return nil
	}
	return &modules[targetIdx]
}

func mergeDocsSourceRoot(module *model.Module, docsRoot model.SourceRoot) {
	for idx := range module.SourceRoots {
		if module.SourceRoots[idx].Path == docsRoot.Path {
			mergeDocsPackage(&module.SourceRoots[idx], docsRoot.Packages[0])
			return
		}
	}
	module.SourceRoots = append(module.SourceRoots, docsRoot)
}

func mergeDocsPackage(sourceRoot *model.SourceRoot, docsPackage model.Package) {
	sourceRoot.FileCount += docsPackage.FileCount
	for idx := range sourceRoot.Packages {
		if sourceRoot.Packages[idx].Path == docsPackage.Path {
			sourceRoot.Packages[idx].FileCount += docsPackage.FileCount
			for _, file := range docsPackage.TopFiles {
				sourceRoot.Packages[idx].TopFiles = addTopFile(sourceRoot.Packages[idx].TopFiles, file, 5)
			}
			if len(sourceRoot.Packages[idx].TopFiles) > 0 {
				sourceRoot.Packages[idx].Heaviest = heaviestFromSummary(sourceRoot.Packages[idx].TopFiles[0])
			}
			return
		}
	}
	sourceRoot.Packages = append(sourceRoot.Packages, docsPackage)
}
