package diagram

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Kitty Graphics Protocol constants. See: https://sw.kovidgoyal.net/kitty/graphics-protocol/
//
// We use f=100 (PNG payload) + a=T (transmit+display) so we don't have to
// send raw pixel data and the terminal handles decompression. Chunks are
// capped at 4 KiB per the Kitty spec.
const (
	kittyChunkSize    = 4096
	pxPerCellHoriz    = 10 // virtual cell width in pixels (tuned for ~10px wide monospace cells)
	pxPerCellVert     = 18 // virtual cell height in pixels
	diagramLabelInset = 14 // extra pixels reserved on the node's left for the accent bar + padding
)

// RenderPNG rasterizes a Layout to an image.RGBA for the given canvas size
// in cells. Used by both the live Kitty TUI path and the scaffold-phase
// architecture export — the code that positions nodes and draws connections
// is exactly the same in both cases.
//
// Returns (img, png_bytes). Callers that only want pixels can discard the
// second value; callers that only want the PNG can discard the first.
func RenderPNG(layout Layout, frame int) (*image.RGBA, []byte, error) {
	pxW := layout.Width * pxPerCellHoriz
	pxH := layout.Height * pxPerCellVert
	if pxW <= 0 || pxH <= 0 {
		return nil, nil, fmt.Errorf("invalid layout size")
	}

	img := image.NewRGBA(image.Rect(0, 0, pxW, pxH))

	// Background — the spec's "canvas" color.
	fillRect(img, 0, 0, pxW, pxH, parseHexRGBA("#0b0b0f"))

	// Dot grid: dim dots every 22 pixels horizontally and vertically.
	dotColor := parseHexRGBA("#1c1c2e")
	for y := 12; y < pxH; y += 22 {
		for x := 12; x < pxW; x += 22 {
			setPixel(img, x, y, dotColor)
			setPixel(img, x+1, y, dotColor)
			setPixel(img, x, y+1, dotColor)
			setPixel(img, x+1, y+1, dotColor)
		}
	}

	// Draw connections beneath the nodes so node cards sit on top.
	for _, conn := range layout.Connections {
		drawPNGConnection(img, layout.Nodes[conn.FromIdx], layout.Nodes[conn.ToIdx], conn, frame)
	}

	// Node cards on top.
	for _, n := range layout.Nodes {
		drawPNGNode(img, n)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return img, nil, err
	}
	return img, buf.Bytes(), nil
}

// KittyGFXTransmit wraps a PNG payload in the chunked APC escape sequence
// Kitty (and compatible terminals like Ghostty) interpret.
//
// The first chunk carries metadata (a=T, f=100, C=1); subsequent chunks carry
// m=1 (more coming) or m=0 (last). C=1 tells the terminal NOT to move the
// cursor after displaying the image — critical when the image is part of a
// bubbletea cell-grid view, otherwise following content gets pushed off-screen.
// We also prepend "delete all images" so each frame replaces the previous one
// instead of stacking.
func KittyGFXTransmit(pngBytes []byte) string {
	if len(pngBytes) == 0 {
		return ""
	}
	encoded := base64.StdEncoding.EncodeToString(pngBytes)

	var out bytes.Buffer

	// Clear any previously-transmitted image so we don't get stale frames
	// overlapping after resize or stack change. d=A means "delete all".
	out.WriteString("\x1b_Ga=d,d=A\x1b\\")

	// First chunk: include the metadata header.
	// q=2  = quiet (no response), C=1 = don't move cursor after display.
	header := "a=T,f=100,q=2,C=1"
	for i := 0; i < len(encoded); i += kittyChunkSize {
		end := i + kittyChunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]
		last := end == len(encoded)

		out.WriteString("\x1b_G")
		if i == 0 {
			out.WriteString(header)
			if !last {
				out.WriteString(",m=1")
			}
		} else {
			if last {
				out.WriteString("m=0")
			} else {
				out.WriteString("m=1")
			}
		}
		out.WriteString(";")
		out.WriteString(chunk)
		out.WriteString("\x1b\\")
	}
	return out.String()
}

// RenderKittyView returns a TUI-ready string for visual mode: the Kitty
// GFX transmission followed by enough blank lines to fill the pane height.
// The blanks preserve the pane's vertical occupancy in bubbletea's view
// string so the diagram area doesn't collapse to zero cells in the layout.
//
// frame is the animation counter — reserved for future particle motion;
// Step 8 passes it through to RenderPNG without animating.
func RenderKittyView(layout Layout, frame int, cellsW, cellsH int) (string, error) {
	_, pngBytes, err := RenderPNG(layout, frame)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	out.WriteString(KittyGFXTransmit(pngBytes))
	// Fill the pane height so bubbletea's diff doesn't reclaim rows under the image.
	for i := 0; i < cellsH; i++ {
		out.WriteString("\n")
	}
	return out.String(), nil
}

// ── pixel drawing primitives ───────────────────────────────────────────────

// fillRect paints a solid rectangle into img. Bounds-checked per pixel via setPixel.
func fillRect(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			setPixel(img, x, y, c)
		}
	}
}

// setPixel is the bounds-checked single-pixel write. No blending — opaque set.
func setPixel(img *image.RGBA, x, y int, c color.RGBA) {
	b := img.Bounds()
	if x < b.Min.X || x >= b.Max.X || y < b.Min.Y || y >= b.Max.Y {
		return
	}
	img.SetRGBA(x, y, c)
}

// parseHexRGBA accepts "#rrggbb" and returns an opaque RGBA. Invalid input
// returns opaque magenta so broken colors are visibly obvious.
func parseHexRGBA(hex string) color.RGBA {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return color.RGBA{R: 255, G: 0, B: 255, A: 255}
	}
	return color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
}

// drawPNGNode paints one node card: filled background, colored accent bar
// on the left, border, and the title rendered with the bitmap font.
//
// When the entry has a LogoURL and the logo is cached (see logos.go), a
// 32x32 resized logo is blitted at the start of the label area, pushing
// the title text right. Unreachable logos fall back to monogram-free text.
func drawPNGNode(img *image.RGBA, n Node) {
	x0 := n.Col * pxPerCellHoriz
	y0 := n.Row * pxPerCellVert
	x1 := (n.Col + n.W) * pxPerCellHoriz
	y1 := (n.Row + n.H) * pxPerCellVert

	fill := parseHexRGBA("#0f0f1a")
	border := parseHexRGBA(n.Entry.DiagramColor)

	// Card background.
	fillRect(img, x0, y0, x1, y1, fill)
	// Rectangular border, 1px wide.
	for x := x0; x < x1; x++ {
		setPixel(img, x, y0, border)
		setPixel(img, x, y1-1, border)
	}
	for y := y0; y < y1; y++ {
		setPixel(img, x0, y, border)
		setPixel(img, x1-1, y, border)
	}
	// 3px accent bar on the left (per the spec).
	fillRect(img, x0, y0, x0+3, y1, border)

	// Try to place a logo at the start of the label area. Logo is square
	// and fills the card's vertical padding area minus a 2px margin.
	textX := x0 + diagramLabelInset
	cardH := y1 - y0
	if cardH >= 36 {
		if logo := loadLogo(n.Entry.LogoURL); logo != nil {
			lx := x0 + 6
			ly := y0 + (cardH-logoPixelSize)/2
			drawImageAt(img, logo, lx, ly)
			textX = lx + logoPixelSize + 6
		}
	}

	// Title text using the basicfont 7x13 face — reliable, bundled in x/image/font.
	drawText(img, n.Title, textX, y0+cardH/2+4, parseHexRGBA("#c8c6e0"))
	// Subtitle (category) in muted color, one line below when space allows.
	if cardH >= 30 {
		drawText(img, string(n.Entry.Category), textX, y0+cardH/2+20, parseHexRGBA("#6060a0"))
	}
}

// drawImageAt blits src onto dst at (x, y). Keeps the call site in drawPNGNode
// clean and factors out the bounds-clipping math.
func drawImageAt(dst, src *image.RGBA, x, y int) {
	sb := src.Bounds()
	for yy := 0; yy < sb.Dy(); yy++ {
		for xx := 0; xx < sb.Dx(); xx++ {
			c := src.RGBAAt(xx, yy)
			if c.A == 0 {
				continue // preserve card background through transparent pixels
			}
			setPixel(dst, x+xx, y+yy, c)
		}
	}
}

// drawPNGConnection draws a cubic bezier between the two nodes' ports. Dashed
// connections skip every third pixel sample so the gaps are visible.
func drawPNGConnection(img *image.RGBA, up, down Node, conn Connection, frame int) {
	x1 := (up.Col + up.W/2) * pxPerCellHoriz
	y1 := (up.Row + up.H) * pxPerCellVert
	x2 := (down.Col + down.W/2) * pxPerCellHoriz
	y2 := down.Row * pxPerCellVert

	dy := (y2 - y1) / 2
	if dy < pxPerCellVert {
		dy = pxPerCellVert
	}
	c1x, c1y := x1, y1+dy
	c2x, c2y := x2, y2-dy

	col := parseHexRGBA(conn.Color)
	samples := 120
	for i := 0; i <= samples; i++ {
		if conn.Dashed && (i/6)%2 == 1 {
			continue
		}
		t := float64(i) / float64(samples)
		x, y := cubicBezierF(t, x1, y1, c1x, c1y, c2x, c2y, x2, y2)
		// Stamp a 2x2 block so the line reads as a clean stroke even at low DPI.
		setPixel(img, x, y, col)
		setPixel(img, x+1, y, col)
		setPixel(img, x, y+1, col)
		setPixel(img, x+1, y+1, col)
	}

	// Endpoint dots, 4px filled squares.
	for _, p := range [][2]int{{x1, y1}, {x2, y2}} {
		fillRect(img, p[0]-2, p[1]-2, p[0]+2, p[1]+2, col)
	}
}

// cubicBezierF is the floating-point variant used by the pixel renderer.
// (The ANSI renderer uses an integer version for cell coordinates.)
func cubicBezierF(t float64, x1, y1, c1x, c1y, c2x, c2y, x2, y2 int) (int, int) {
	u := 1 - t
	b0 := u * u * u
	b1 := 3 * u * u * t
	b2 := 3 * u * t * t
	b3 := t * t * t
	x := b0*float64(x1) + b1*float64(c1x) + b2*float64(c2x) + b3*float64(x2)
	y := b0*float64(y1) + b1*float64(c1y) + b2*float64(c2y) + b3*float64(y2)
	return int(x + 0.5), int(y + 0.5)
}

// drawText writes a text string onto the image using basicfont.Face7x13.
// Coordinates (x, y) specify the glyph baseline — callers should add the
// font's ascent if they want "top-of-text" coordinates.
func drawText(img *image.RGBA, s string, x, y int, c color.RGBA) {
	face := basicfont.Face7x13
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)},
	}
	drawer.DrawString(s)
}
