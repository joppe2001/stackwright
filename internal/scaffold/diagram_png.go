package scaffold

import (
	"image/png"
	"os"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/phase_design/diagram"
)

// diagramCanvas sizes (in diagram *cells* before the pixel multiplier in
// kitty.go). 120x60 cells ends up around 1200x1080 pixels, which matches
// the spec's 1200×900 target closely enough.
const (
	diagramCellsW = 120
	diagramCellsH = 60
)

// WriteArchitecturePNG renders the confirmed stack as an architecture image
// and writes it to outPath. Uses the same geometry as the live TUI diagram,
// so the exported image matches what the user saw while designing.
func WriteArchitecturePNG(outPath string, bundle registry.Bundle, stack tui.Stack) error {
	layout := diagram.Compute(stack, bundle, diagramCellsW, diagramCellsH)
	img, _, err := diagram.RenderPNG(layout, 0)
	if err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
