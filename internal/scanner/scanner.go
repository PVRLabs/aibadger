package scanner

import (
	"sync"
	"time"

	"github.com/PVRLabs/aibadger/internal/dirpolicy"
	"github.com/PVRLabs/aibadger/internal/externalcontext"
	"github.com/PVRLabs/aibadger/internal/model"
)

// Scanner orchestrates the scanning process.
type Scanner struct {
	MaxTopFilesPerPackage int
	ProjectRoot           string
	MaxFilesPerDirectory  int // 0 = unlimited
	genericFallback       bool
	directoryPolicy       dirpolicy.Policy
}

// NewScanner creates a new Scanner instance.
func NewScanner(root string, directoryPolicies ...dirpolicy.Policy) *Scanner {
	directoryPolicy := dirpolicy.Resolve(directoryPolicies...)

	return &Scanner{ProjectRoot: root, directoryPolicy: directoryPolicy}
}

// Scan runs language-specific detectors in parallel, then falls back to the
// generic detector if no language detector finds modules.
func (s *Scanner) Scan() (*model.ProjectTopology, error) {
	start := time.Now()

	topology := &model.ProjectTopology{
		ProjectRoot: s.ProjectRoot,
		Modules:     []model.Module{},
	}

	externalContext, err := externalcontext.Load(s.ProjectRoot, s.directoryPolicy)
	if err != nil {
		return nil, err
	}
	topology.ExternalContext = externalContext

	var wg sync.WaitGroup
	var mu sync.Mutex

	nodeDetector := NewNodeDetector(s.directoryPolicy)
	nodeDetector.maxFilesPerDir = s.MaxFilesPerDirectory
	nodeDetector.maxTopFiles = s.MaxTopFilesPerPackage
	csharpDetector := NewCSharpDetector(s.directoryPolicy)
	csharpDetector.maxFilesPerDir = s.MaxFilesPerDirectory
	csharpDetector.maxTopFiles = s.MaxTopFilesPerPackage
	cppDetector := NewCppDetector(s.directoryPolicy)
	cppDetector.maxFilesPerDir = s.MaxFilesPerDirectory
	cppDetector.maxTopFiles = s.MaxTopFilesPerPackage

	detectors := []func(string) ([]model.Module, error){
		func(root string) ([]model.Module, error) {
			d := NewGoDetector(s.directoryPolicy)
			d.maxTopFiles = s.MaxTopFilesPerPackage
			return d.Detect(root)
		},
		func(root string) ([]model.Module, error) {
			d := NewJavaDetector(s.directoryPolicy)
			d.maxTopFiles = s.MaxTopFilesPerPackage
			return d.Detect(root)
		},
		nodeDetector.Detect,
		func(root string) ([]model.Module, error) {
			d := NewPythonDetector(s.directoryPolicy)
			d.maxTopFiles = s.MaxTopFilesPerPackage
			return d.Detect(root)
		},
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
	var supplementalHTML []model.SourceRoot
	if len(topology.Modules) == 0 {
		det := NewGenericDetector(s.directoryPolicy)
		det.maxTopFiles = s.MaxTopFilesPerPackage
		if s.MaxFilesPerDirectory > 0 {
			det.maxFilesPerDir = s.MaxFilesPerDirectory
		}
		modules, detErr := det.Detect(s.ProjectRoot)
		if detErr == nil {
			topology.Modules = modules
			usedGenericFallback = true
		}
	} else {
		det := NewGenericDetector(s.directoryPolicy)
		det.maxTopFiles = s.MaxTopFilesPerPackage
		if s.MaxFilesPerDirectory > 0 {
			det.maxFilesPerDir = s.MaxFilesPerDirectory
		}
		ownership := newSemanticSourceOwnership(s.ProjectRoot, topology.Modules, s.directoryPolicy)
		supplementalHTML, _ = det.collectSupplementalHTML(s.ProjectRoot, ownership)
		coverage, contextCandidates, coverageErr := det.collectGenericAugmentation(s.ProjectRoot, ownership)
		if coverageErr == nil {
			topology.Modules = append(topology.Modules, coverage...)
			projectContext = contextCandidates
		}
	}
	languageWeights := sourceLanguageWeightsFromModules(topology.Modules, s.ProjectRoot, s.directoryPolicy)
	if csharpDetector.languageSourceCount > 0 {
		languageWeights["C#"] += int64(csharpDetector.languageSourceCount)
	}
	if cppDetector.languageSourceCount > 0 {
		languageWeights["C++"] += int64(cppDetector.languageSourceCount)
	}

	if !usedGenericFallback {
		docs, docsErr := scanDocs(s.ProjectRoot, s.directoryPolicy)
		if docsErr != nil {
			docs = nil
		}
		attachDocsToTopology(topology, docs)

		webResources, webErr := scanWebResources(s.ProjectRoot, s.directoryPolicy)
		if webErr != nil {
			webResources = nil
		}
		attachWebResourcesToTopology(topology, webResources)
	}

	opsResources, opsErr := scanOpsResources(s.ProjectRoot, s.directoryPolicy)
	if opsErr != nil {
		opsResources = nil
	}
	attachOpsResourcesToTopology(topology, opsResources)

	resources, resErr := scanGenericResources(s.ProjectRoot, s.directoryPolicy)
	if resErr != nil {
		resources = nil
	}
	attachGenericResourcesToTopology(topology, resources)
	attachProjectContextToTopology(topology, projectContext)

	if len(supplementalHTML) > 0 {
		transferNodeHTML(topology, s.ProjectRoot, supplementalHTML[0].HTMLPaths, s.directoryPolicy)
		module := docsTargetModule(topology.Modules)
		if module != nil {
			module.SourceRoots = append(module.SourceRoots, supplementalHTML...)
		}
	}

	// Finalize topology
	topology.ScanTime = time.Since(start)
	s.genericFallback = usedGenericFallback
	s.finalizeTopologyWithLanguageWeights(topology, languageWeights)

	return topology, nil
}
