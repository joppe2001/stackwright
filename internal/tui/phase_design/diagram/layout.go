// Package diagram computes and renders the architecture diagram shown in
// the right pane of the design phase. Two renderers share the same
// geometry module (layout.go): standard.go draws with ANSI/Unicode glyphs,
// kitty.go (Step 8) emits a PNG over the Kitty Graphics Protocol.
package diagram

import (
	"sort"
	"strings"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

// Layout is the pre-computed positions for one frame of the diagram.
// Both the standard and Kitty renderers consume this struct; only the
// drawing backends differ.
type Layout struct {
	Width, Height int

	Nodes       []Node
	Connections []Connection
}

// Node is one rendered tech box.
type Node struct {
	Entry       registry.Entry
	Col         int // x of the top-left corner in cell coordinates
	Row         int // y of the top-left corner in cell coordinates
	LogicalRow  int // 0-based row index in the layer-row mapping (see nodeRow)
	W, H        int // box size in cells, including border
	Title       string
	SubTag      string // category label shown in muted text
}

// Ports returns the top and bottom port (cell coordinates).
// The top port is where an incoming connection terminates; the bottom
// port is where an outgoing connection originates.
func (n Node) TopPort() (int, int)    { return n.Col + n.W/2, n.Row }
func (n Node) BottomPort() (int, int) { return n.Col + n.W/2, n.Row + n.H - 1 }

// Connection is a drawn line between two Nodes.
// Dashed indicates an "on-demand" data-plane connection (DB/cache); false
// means a "live" HTTP/RPC connection.
type Connection struct {
	FromIdx, ToIdx int
	Color          string // tech color to render in; from the source node
	Dashed         bool
}

// nodeRow assigns a vertical slot to each layer. Lower numbers render higher
// on screen. The mapping keeps related layers on the same row so the
// diagram reads like the spec's example (frontend up top, infra at the bottom).
func nodeRow(l tui.Layer) int {
	switch l {
	case tui.LayerFrontend:
		return 0
	case tui.LayerBackend, tui.LayerAuth, tui.LayerPayments:
		return 1
	case tui.LayerDatabase, tui.LayerCache:
		return 2
	case tui.LayerInfra, tui.LayerCICD:
		return 3
	}
	return 4 // unknown / service-tier falls through to its own row
}

// Compute positions nodes within a canvas of (w, h) cells and decides which
// connections to draw. Nodes are spaced on a virtual grid (see nodeRow) then
// centered horizontally within each row.
//
// Connections are emitted between adjacent rows when the upper node declares
// the lower node in compatible_with (or vice versa). We don't try to draw
// every possible edge — the compat list is the design-time hint.
func Compute(stack tui.Stack, bundle registry.Bundle, w, h int) Layout {
	sel := selectedByLayer(stack, bundle)
	if len(sel) == 0 || w < 20 || h < 10 {
		return Layout{Width: w, Height: h}
	}

	const (
		nodeW = 22
		nodeH = 4
	)

	// Group by row.
	rows := map[int][]layerEntry{}
	for _, le := range sel {
		r := nodeRow(le.layer)
		rows[r] = append(rows[r], le)
	}
	rowIdx := make([]int, 0, len(rows))
	for r := range rows {
		rowIdx = append(rowIdx, r)
	}
	sort.Ints(rowIdx)

	// Vertical placement: distribute rows across the canvas.
	rowY := map[int]int{}
	rowStep := (h - nodeH) / max(1, len(rowIdx))
	y := 1
	for _, r := range rowIdx {
		rowY[r] = y
		y += rowStep
	}

	// Horizontal placement: center each row's nodes.
	nodes := make([]Node, 0, len(sel))
	indexByLayer := map[tui.Layer]int{}
	for _, r := range rowIdx {
		list := rows[r]
		// Stable-sort the row so auth/payments consistently right of backend, etc.
		sort.SliceStable(list, func(i, j int) bool { return list[i].layer < list[j].layer })
		total := len(list)*nodeW + (len(list)-1)*4
		startX := (w - total) / 2
		if startX < 1 {
			startX = 1
		}
		for i, le := range list {
			x := startX + i*(nodeW+4)
			node := Node{
				Entry:      le.entry,
				Col:        x,
				Row:        rowY[r],
				LogicalRow: r,
				W:          nodeW,
				H:          nodeH,
				Title:      le.entry.Name,
				SubTag:     string(le.entry.Category),
			}
			indexByLayer[le.layer] = len(nodes)
			nodes = append(nodes, node)
		}
	}

	// Connections: only adjacent logical rows (delta == 1), and cap the
	// number of outgoing edges per upper node so dense stacks don't produce
	// tangled lines. Prefer edges where the lower node is in the directly
	// following row and the pair is mutually compatible (listed in both
	// compatible_with arrays) — those read as "primary" relationships.
	const maxOutPerNode = 2
	outCount := make(map[int]int, len(nodes))
	var conns []Connection

	// Primary pass: adjacent-row, mutually compatible pairs.
	for i, a := range nodes {
		for j, b := range nodes {
			if i >= j {
				continue
			}
			upper, lower := a, b
			upperIdx, lowerIdx := i, j
			if a.LogicalRow > b.LogicalRow {
				upper, lower = b, a
				upperIdx, lowerIdx = j, i
			}
			if lower.LogicalRow-upper.LogicalRow != 1 {
				continue
			}
			if !mutuallyCompatible(upper.Entry, lower.Entry) {
				continue
			}
			if outCount[upperIdx] >= maxOutPerNode {
				continue
			}
			conns = append(conns, Connection{
				FromIdx: upperIdx,
				ToIdx:   lowerIdx,
				Color:   upper.Entry.DiagramColor,
				Dashed:  isDataConnection(upper.Entry, lower.Entry),
			})
			outCount[upperIdx]++
		}
	}

	// Secondary pass: nodes left with zero outgoing edges get one single-
	// direction compatibility edge to an adjacent lower row, so every node
	// visibly connects somewhere.
	for i := range nodes {
		if outCount[i] > 0 {
			continue
		}
		for j, b := range nodes {
			if j == i {
				continue
			}
			if b.LogicalRow-nodes[i].LogicalRow != 1 {
				continue
			}
			if !compatible(nodes[i].Entry, b.Entry) {
				continue
			}
			conns = append(conns, Connection{
				FromIdx: i,
				ToIdx:   j,
				Color:   nodes[i].Entry.DiagramColor,
				Dashed:  isDataConnection(nodes[i].Entry, b.Entry),
			})
			outCount[i]++
			break
		}
	}

	return Layout{Width: w, Height: h, Nodes: nodes, Connections: conns}
}

// isDataConnection flags edges that represent an on-demand data query
// (app server → database or cache). Rendered dashed in both renderers.
func isDataConnection(a, b registry.Entry) bool {
	isStorage := func(c registry.Category) bool {
		return c == registry.CategoryDatabase || c == registry.CategoryCache
	}
	return isStorage(a.Category) || isStorage(b.Category)
}

// compatible reports whether either entry lists the other in its compatible_with.
func compatible(a, b registry.Entry) bool {
	if contains(a.CompatibleWith, b.Slug) {
		return true
	}
	return contains(b.CompatibleWith, a.Slug)
}

// mutuallyCompatible reports whether BOTH entries list each other. These
// bidirectional pairs are the strongest "primary" relationships and get
// priority in the connection layout.
func mutuallyCompatible(a, b registry.Entry) bool {
	return contains(a.CompatibleWith, b.Slug) && contains(b.CompatibleWith, a.Slug)
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// layerEntry pairs a selected layer with its resolved registry entry.
type layerEntry struct {
	layer tui.Layer
	entry registry.Entry
}

func selectedByLayer(stack tui.Stack, bundle registry.Bundle) []layerEntry {
	out := make([]layerEntry, 0, len(tui.AllLayers))
	for _, l := range tui.AllLayers {
		if l == tui.LayerAppType {
			continue
		}
		slug := stack.Slug(l)
		if slug == "" {
			continue
		}
		e, ok := bundle.BySlug(slug)
		if !ok {
			continue
		}
		out = append(out, layerEntry{layer: l, entry: e})
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// monogram extracts a 1-2 character glyph from an entry name for icon-free
// node rendering. Picks initials: "Next.js 14" → "N", "Go + chi" → "Gc".
func monogram(name string) string {
	fields := strings.Fields(strings.ReplaceAll(name, "+", " "))
	if len(fields) == 0 {
		return "?"
	}
	if len(fields) == 1 {
		r := []rune(fields[0])
		if len(r) >= 1 {
			return string(r[0])
		}
		return "?"
	}
	var b strings.Builder
	for i := 0; i < 2 && i < len(fields); i++ {
		r := []rune(fields[i])
		if len(r) > 0 {
			b.WriteRune(r[0])
		}
	}
	return b.String()
}
