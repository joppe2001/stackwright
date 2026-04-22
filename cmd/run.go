package cmd

import (
	"github.com/joppe2001/stackwright/internal/app"
	"github.com/joppe2001/stackwright/internal/detect"
	"github.com/joppe2001/stackwright/internal/registry"
)

// runTUI is the main interactive entrypoint. It probes the terminal, loads
// the registry (network → cache → bundled fallback), and hands everything
// off to internal/app.Run which builds the bubbletea.Program.
func runTUI(noKitty, offline bool) error {
	caps := detect.Probe()
	reg := registry.Load(registry.LoadOptions{Offline: offline})

	return app.Run(app.Opts{
		Capabilities: caps,
		Registry:     reg,
		NoKitty:      noKitty,
		Offline:      offline,
	})
}
