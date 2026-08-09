package badger

import (
	"fmt"
	"io"
	"strings"

	"github.com/PVRLabs/aibadger/internal/engine"
	"github.com/PVRLabs/aibadger/internal/protocol"
	"github.com/PVRLabs/aibadger/internal/workflow"
)

type scanOutputMode int

const (
	scanOutputStable scanOutputMode = iota
	scanOutputSilent
)

func apiEngineOptions(cfg Config, focus protocol.Focus) workflow.EngineOptions {
	return workflow.EngineOptions{
		MaxContextFileBytes:    cfg.MaxContextFileBytes,
		MaxTopologyPromptBytes: cfg.MaxTopologyPromptBytes,
		MaxPromptTwoBytes:      cfg.MaxPromptTwoBytes,
		SchemaAConstraint:      cfg.SchemaAConstraint,
		SchemaBConstraint:      cfg.SchemaBConstraint,
		Focus:                  focus,
	}
}

func scanProject(w io.Writer, root string, output scanOutputMode, maxFilesPerDir int) (*engine.Engine, error) {
	if output != scanOutputSilent {
		fmt.Fprint(w, "Scanning project... ")
	}
	eng, err := engine.New(root, maxFilesPerDir)
	if err != nil {
		return nil, err
	}
	if output == scanOutputSilent {
		return eng, nil
	}
	topology := eng.Topology
	fmt.Fprint(w, "Done\n\n")
	fmt.Fprintf(w, "Project: %s", topology.Name)
	if len(topology.Languages) > 0 {
		fmt.Fprintf(w, " (%s)", strings.Join(topology.Languages, ", "))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Found %d modules\n\n", len(topology.Modules))
	return eng, nil
}
