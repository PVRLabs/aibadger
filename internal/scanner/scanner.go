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

	detectors := []func(string) ([]model.Module, error){
		NewGoDetector().Detect,
		NewJavaDetector().Detect,
		NewNodeDetector().Detect,
		NewPythonDetector().Detect,
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

	// Fallback to GenericDetector if no modules found
	usedGenericFallback := false
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

	// Finalize topology
	topology.ScanTime = time.Since(start)
	s.finalizeTopology(topology)

	return topology, nil
}
