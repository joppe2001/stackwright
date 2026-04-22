package diagram

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// canvas is a lightweight 2-D rune grid with a per-cell ANSI color.
// ANSI escape sequences are emitted by render() only, so intermediate draws
// can run at raw-rune speed without string allocations per cell.
type canvas struct {
	w, h  int
	runes [][]rune
	color [][]string // "" = default foreground
	dim   [][]bool   // dim=true applies the muted-text color regardless of `color`
}

func newCanvas(w, h int) *canvas {
	runes := make([][]rune, h)
	color := make([][]string, h)
	dim := make([][]bool, h)
	for i := range runes {
		runes[i] = make([]rune, w)
		color[i] = make([]string, w)
		dim[i] = make([]bool, w)
		for j := range runes[i] {
			runes[i][j] = ' '
		}
	}
	return &canvas{w: w, h: h, runes: runes, color: color, dim: dim}
}

// set writes one cell without bounds checking. Callers use in().
func (c *canvas) set(x, y int, r rune, color string, dim bool) {
	if !c.in(x, y) {
		return
	}
	c.runes[y][x] = r
	c.color[y][x] = color
	c.dim[y][x] = dim
}

func (c *canvas) in(x, y int) bool { return x >= 0 && x < c.w && y >= 0 && y < c.h }

// render collapses the canvas into one ANSI-colored string. Runs of same-color
// cells share a single SGR prefix so the output stays compact.
func (c *canvas) render() string {
	var b strings.Builder
	for y := 0; y < c.h; y++ {
		curColor := ""
		for x := 0; x < c.w; x++ {
			col := c.color[y][x]
			if c.dim[y][x] {
				col = theme.TextMuted
			}
			if col != curColor {
				if curColor != "" {
					b.WriteString("\x1b[0m")
				}
				if col != "" {
					b.WriteString(sgrFG(col))
				}
				curColor = col
			}
			b.WriteRune(c.runes[y][x])
		}
		if curColor != "" {
			b.WriteString("\x1b[0m")
			curColor = ""
		}
		if y < c.h-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// sgrFG returns the SGR 24-bit foreground escape for a hex color like "#ab12ef".
// Invalid hex produces the terminal default (empty string).
func sgrFG(hex string) string {
	r, g, b, ok := parseHex(hex)
	if !ok {
		return ""
	}
	return "\x1b[38;2;" + strconv.Itoa(r) + ";" + strconv.Itoa(g) + ";" + strconv.Itoa(b) + "m"
}

func parseHex(hex string) (int, int, int, bool) {
	s := strings.TrimPrefix(hex, "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	r, err := strconv.ParseInt(s[0:2], 16, 0)
	if err != nil {
		return 0, 0, 0, false
	}
	g, err := strconv.ParseInt(s[2:4], 16, 0)
	if err != nil {
		return 0, 0, 0, false
	}
	bb, err := strconv.ParseInt(s[4:6], 16, 0)
	if err != nil {
		return 0, 0, 0, false
	}
	return int(r), int(g), int(bb), true
}

// RenderStandard produces an ANSI-rendered diagram for the given layout.
// frame is incremented by the tick loop in the design phase so particle
// positions advance over time. Pass frame=0 for a still render.
func RenderStandard(layout Layout, frame int) string {
	if layout.Width <= 0 || layout.Height <= 0 {
		return ""
	}
	c := newCanvas(layout.Width, layout.Height)

	drawDotGrid(c)
	for _, conn := range layout.Connections {
		drawConnection(c, layout.Nodes[conn.FromIdx], layout.Nodes[conn.ToIdx], conn, frame)
	}
	for _, n := range layout.Nodes {
		drawNode(c, n)
	}
	return c.render()
}

// drawDotGrid fills the background with dim dots every 4 cols / 2 rows.
// Nodes and connections are drawn over the top and erase the dot below.
func drawDotGrid(c *canvas) {
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			if x%4 == 0 && y%2 == 0 {
				c.set(x, y, '·', "", true)
			}
		}
	}
}

// drawNode renders a rounded-border box with the tech name and category pill.
// Tech color colors the border so the eye associates the box with the tech.
func drawNode(c *canvas, n Node) {
	// Fill the interior first with blanks so the dot grid doesn't bleed through.
	for y := n.Row + 1; y < n.Row+n.H-1; y++ {
		for x := n.Col + 1; x < n.Col+n.W-1; x++ {
			c.set(x, y, ' ', "", false)
		}
	}

	// Corners + sides.
	c.set(n.Col, n.Row, '╭', n.Entry.DiagramColor, false)
	c.set(n.Col+n.W-1, n.Row, '╮', n.Entry.DiagramColor, false)
	c.set(n.Col, n.Row+n.H-1, '╰', n.Entry.DiagramColor, false)
	c.set(n.Col+n.W-1, n.Row+n.H-1, '╯', n.Entry.DiagramColor, false)
	for x := n.Col + 1; x < n.Col+n.W-1; x++ {
		c.set(x, n.Row, '─', n.Entry.DiagramColor, false)
		c.set(x, n.Row+n.H-1, '─', n.Entry.DiagramColor, false)
	}
	for y := n.Row + 1; y < n.Row+n.H-1; y++ {
		c.set(n.Col, y, '│', n.Entry.DiagramColor, false)
		c.set(n.Col+n.W-1, y, '│', n.Entry.DiagramColor, false)
	}

	// Interior: title, fitting into available space minus a left-padding.
	title := n.Title
	maxTitle := n.W - 4
	if len(title) > maxTitle {
		title = title[:maxTitle]
	}
	for i, r := range title {
		c.set(n.Col+2+i, n.Row+1, r, theme.TextPrimary, false)
	}
}

// drawConnection traces a cubic bezier between two nodes' ports, draws a
// handful of particles along it, and highlights both endpoints.
func drawConnection(c *canvas, up, down Node, conn Connection, frame int) {
	x1, y1 := up.BottomPort()
	x2, y2 := down.TopPort()

	// Control points: vertical tangents at both ends, length = half the row delta.
	dy := (y2 - y1) / 2
	if dy < 2 {
		dy = 2
	}
	c1x, c1y := x1, y1+dy
	c2x, c2y := x2, y2-dy

	// Sample the curve. Dense sampling so multiple samples land in the same
	// cell — we overwrite, which is fine.
	samples := 40
	points := make([][2]int, 0, samples)
	for i := 0; i <= samples; i++ {
		t := float64(i) / float64(samples)
		x, y := cubicBezier(t, x1, y1, c1x, c1y, c2x, c2y, x2, y2)
		points = append(points, [2]int{x, y})
	}

	// Dashed connections skip every other sample group so gaps are visible.
	for i, p := range points {
		if conn.Dashed && (i/3)%2 == 1 {
			continue
		}
		glyph := glyphForDirection(points, i)
		// Don't overwrite a node cell.
		if isNodeCell(c, p[0], p[1]) {
			continue
		}
		c.set(p[0], p[1], glyph, conn.Color, false)
	}

	// Endpoint dots.
	c.set(x1, y1, '●', conn.Color, false)
	c.set(x2, y2, '●', conn.Color, false)

	// Particles — 3 per connection, drifting along the curve over time.
	for i := 0; i < 3; i++ {
		phase := (frame + i*33) % 100
		idx := int(float64(phase) / 100.0 * float64(len(points)))
		if idx < 0 || idx >= len(points) {
			continue
		}
		px, py := points[idx][0], points[idx][1]
		if isNodeCell(c, px, py) {
			continue
		}
		c.set(px, py, '●', conn.Color, false)
	}
}

// cubicBezier returns the integer cell position for parameter t.
func cubicBezier(t float64, x1, y1, c1x, c1y, c2x, c2y, x2, y2 int) (int, int) {
	u := 1 - t
	b0 := u * u * u
	b1 := 3 * u * u * t
	b2 := 3 * u * t * t
	b3 := t * t * t
	x := b0*float64(x1) + b1*float64(c1x) + b2*float64(c2x) + b3*float64(x2)
	y := b0*float64(y1) + b1*float64(c1y) + b2*float64(c2y) + b3*float64(y2)
	return int(x + 0.5), int(y + 0.5)
}

// glyphForDirection picks a single-cell glyph approximating the local slope
// of the curve at index i. Purely heuristic — nicer than plain │ since we
// get diagonals on the curved parts.
func glyphForDirection(points [][2]int, i int) rune {
	if i == 0 || i == len(points)-1 {
		return '│'
	}
	prev := points[i-1]
	next := points[i]
	dx := next[0] - prev[0]
	dy := next[1] - prev[1]
	switch {
	case dy == 0:
		return '─'
	case dx == 0:
		return '│'
	case dx > 0 && dy > 0, dx < 0 && dy < 0:
		return '╲'
	case dx > 0 && dy < 0, dx < 0 && dy > 0:
		return '╱'
	}
	return '·'
}

// isNodeCell lets connection drawing skip cells occupied by a node border or
// interior so the node stays legible.
func isNodeCell(c *canvas, x, y int) bool {
	// Heuristic: if the cell contains a box-drawing / letter glyph, leave it.
	if !c.in(x, y) {
		return true
	}
	r := c.runes[y][x]
	switch r {
	case '╭', '╮', '╰', '╯', '─', '│':
		return true
	}
	if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
		return true
	}
	return false
}

// Debug helpers — kept unexported; used by tests in Step 13 to snapshot-compare
// rendered layouts without the animation frame.
var _ = fmt.Sprintf
