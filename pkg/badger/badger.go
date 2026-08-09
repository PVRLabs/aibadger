package badger

import (
	"github.com/PVRLabs/aibadger/internal/brand"
	"github.com/PVRLabs/aibadger/internal/tui"
	"github.com/PVRLabs/aibadger/internal/version"
)

const (
	// Name is the public product name printed by command wrappers.
	Name = brand.Name
	// Version is the initial public OSS release version.
	Version = version.Version
)

// Run starts the interactive BYOL TUI.
func Run(cfg Config) error {
	cfg = cfg.withDefaults()
	return tui.RunWithConfig(cfg.Root, cfg.tuiConfig())
}
