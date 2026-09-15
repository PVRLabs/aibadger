package scanner

import (
	"sync"
	"time"

	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/model"
)

// Scanner orchestrates the scanning process.
type Scanner struct {
	ProjectRoot          string
	MaxFilesPerDirectory int // 0 = unlimited
}

// NewScanner creates a new Scanner instance.
func NewScanner(root string) *Scanner {
	return &Scanner{ProjectRoot: root}
}

// Scan runs language-specific detectors in parallel, then falls back to the
// generic detector if no language detector finds modules.
func (s *Scanner) Scan() (*model.ProjectTopology, error) {
	start := time.Now()

	topology := &model.ProjectTopology{
		ProjectRoot: s.ProjectRoot,
		Modules:     []model.Module{},
	}

	externalContext, err := externalcontext.Load(s.ProjectRoot)
	if err != nil {
		return nil, err
	}
	topology.ExternalContext = externalContext

	var wg sync.WaitGroup
	var mu sync.Mutex

	nodeDetector := NewNodeDetector()
	nodeDetector.maxFilesPerDir = s.MaxFilesPerDirectory
	csharpDetector := NewCSharpDetector()
	csharpDetector.maxFilesPerDir = s.MaxFilesPerDirectory
	cppDetector := NewCppDetector()
	cppDetector.maxFilesPerDir = s.MaxFilesPerDirectory

	detectors := []func(string) ([]model.Module, error){
		NewGoDetector().Detect,
		NewJavaDetector().Detect,
		nodeDetector.Detect,
		NewPythonDetector().Detect,
		csharpDetector.Detect,
		cppDetector.Detect,
	}
	for _, detect := range detectors {
		wg.Add(1)
		go func(detect func(string) ([]model.Module, error)) {
			defer wg.Done()

			modules, detErr := detect(s.ProjectRoot)
			if detErr == nil {
				mu.Lock()
				topology.Modules = append(topology.Modules, modules...)
				mu.Unlock()
			}
		}(detect)
	}

	wg.Wait()

	// Preserve the full Generic detector as a fallback. When specialized
	// detectors succeed, add only recognized source they do not semantically
	// own so normal language weighting and finalization can consume it.
	usedGenericFallback := false
	var projectContext []projectContextCandidate
	if len(topology.Modules) == 0 {
		det := NewGenericDetector()
		if s.MaxFilesPerDirectory > 0 {
			det.maxFilesPerDir = s.MaxFilesPerDirectory
		}
		modules, detErr := det.Detect(s.ProjectRoot)
		if detErr == nil {
			topology.Modules = modules
			usedGenericFallback = true
		}
	} else {
		det := NewGenericDetector()
		if s.MaxFilesPerDirectory > 0 {
			det.maxFilesPerDir = s.MaxFilesPerDirectory
		}
		ownership := newSemanticSourceOwnership(s.ProjectRoot, topology.Modules)
		coverage, contextCandidates, coverageErr := det.collectGenericAugmentation(s.ProjectRoot, ownership)
		if coverageErr == nil {
			topology.Modules = append(topology.Modules, coverage...)
			projectContext = contextCandidates
		}
	}
	languageWeights := sourceLanguageWeightsFromModules(topology.Modules, s.ProjectRoot)
	if csharpDetector.languageSourceCount > 0 {
		languageWeights["C#"] += int64(csharpDetector.languageSourceCount)
	}
	if cppDetector.languageSourceCount > 0 {
		languageWeights["C++"] += int64(cppDetector.languageSourceCount)
	}

	if !usedGenericFallback {
		docs, docsErr := scanDocs(s.ProjectRoot)
		if docsErr != nil {
			docs = nil
		}
		attachDocsToTopology(topology, docs)

		webResources, webErr := scanWebResources(s.ProjectRoot)
		if webErr != nil {
			webResources = nil
		}
		attachWebResourcesToTopology(topology, webResources)
	}

	opsResources, opsErr := scanOpsResources(s.ProjectRoot)
	if opsErr != nil {
		opsResources = nil
	}
	attachOpsResourcesToTopology(topology, opsResources)

	resources, resErr := scanGenericResources(s.ProjectRoot)
	if resErr != nil {
		resources = nil
	}
	attachGenericResourcesToTopology(topology, resources)
	attachProjectContextToTopology(topology, projectContext)

	// Finalize topology
	topology.ScanTime = time.Since(start)
	s.finalizeTopologyWithLanguageWeights(topology, languageWeights)

	return topology, nil
}
