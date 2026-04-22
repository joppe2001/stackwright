// Package phase_design implements the split-pane design phase.
// Left pane: layer navigator (layers.go). Right pane: live architecture
// diagram (diagram/standard.go or diagram/kitty.go — wired in Steps 6/8).
//
// This model is "dumb" in the sense that it holds no registry data of its own —
// it receives the bundle from the root and emits StackUpdateMsg when the user
// changes anything. The root model routes those updates back out to the Setup
// and Scaffold phases when the user presses 'g' to advance.
package phase_design

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// Model is the design phase's tea.Model. Not literally a tea.Model because
// phases don't run their own tea.Program — the root model calls their
// Update/View directly with the shared window size.
type Model struct {
	registry registry.Bundle
	stack    tui.Stack

	layers layersModel

	// leftWidth is how wide the left pane gets. Rest is the diagram.
	leftWidth int
	// width/height are the design-phase content area (inside any parent chrome).
	width  int
	height int
}

// New returns a fresh design-phase model seeded with the registry and an empty stack.
func New(bundle registry.Bundle) Model {
	stack := tui.NewStack()
	return Model{
		registry:  bundle,
		stack:     stack,
		layers:    newLayersModel(bundle, stack),
		leftWidth: 24,
	}
}

// Stack returns the current in-progress stack.
// Used by the root model when the user presses 'g' to hand it to Setup.
func (m Model) Stack() tui.Stack { return m.stack }

// SetSize is called by the root model on every tea.WindowSizeMsg.
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.leftWidth = leftPaneWidth(w)
	m.layers.setSize(m.leftWidth-2, h-2) // inner size, minus the border
}

// Init satisfies the tea.Model contract even though we don't own the program.
func (m Model) Init() tea.Cmd { return nil }

// Update handles key events for the design phase.
// Unknown keys fall through so the root model can handle globals.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tui.StackUpdateMsg:
		m.stack = msg.Stack
		m.layers.syncStack(m.stack)
		return m, nil
	case tea.KeyMsg:
		// 'g' advances to setup — but only if there's at least one selection.
		if msg.String() == "g" && m.layers.mode == modeLayerList {
			if m.readyToAdvance() {
				return m, func() tea.Msg { return tui.PhaseChangeMsg{To: tui.PhaseSetup} }
			}
		}
		cmd, _ := m.layers.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the full design phase at the current window size.
func (m Model) View() string {
	if m.width < 80 {
		return m.viewNarrow()
	}
	return m.viewWide()
}

func (m Model) viewWide() string {
	leftInnerW := m.leftWidth - 2
	rightInnerW := m.width - m.leftWidth - 2
	innerH := m.height - 2

	left := theme.PaneBorder.
		Width(leftInnerW).
		Height(innerH).
		Render(m.layers.View(leftInnerW, innerH))

	right := theme.PaneBorder.
		Width(rightInnerW).
		Height(innerH).
		Render(m.rightPlaceholder(rightInnerW, innerH))

	return sideBySide(left, right)
}

func (m Model) viewNarrow() string {
	// Left pane only. Step 13 adds a [d] diagram toggle.
	return theme.PaneBorder.
		Width(m.width - 2).
		Height(m.height - 2).
		Render(m.layers.View(m.width-4, m.height-4))
}

// rightPlaceholder is shown until Step 6 wires the real diagram.
func (m Model) rightPlaceholder(w, h int) string {
	entries := m.stack.SelectedEntries(m.registry)
	var b strings.Builder
	b.WriteString(theme.Accent.Render("LIVE DIAGRAM"))
	b.WriteString("  ")
	b.WriteString(theme.Dim.Render("(standard/Kitty renderer lands in Steps 6 & 8)"))
	b.WriteString("\n\n")

	if m.stack.AppName == "" {
		b.WriteString(theme.Dim.Render("Set an app name to begin."))
		return b.String()
	}
	b.WriteString("app: ")
	b.WriteString(m.stack.AppName)
	b.WriteString("\n\n")
	if len(entries) == 0 {
		b.WriteString(theme.Dim.Render("No technologies selected yet."))
		return b.String()
	}

	// Render a super simple "stacked node" preview until the real diagram arrives.
	for i, e := range entries {
		arrow := " "
		if i > 0 {
			arrow = "│"
		}
		b.WriteString(theme.Dim.Render(arrow))
		b.WriteString("\n")
		b.WriteString(renderNode(e))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim.Render(fmt.Sprintf("%d / %d layers set", countSet(m.stack), len(tui.AllLayers))))
	b.WriteString("\n")
	if m.readyToAdvance() {
		b.WriteString(theme.Good.Render("Press 'g' to continue to setup."))
	}
	return b.String()
}

func renderNode(e registry.Entry) string {
	// Compact one-line node card for the placeholder diagram.
	return fmt.Sprintf("  ▪ %s  %s",
		theme.Accent.Render(e.Name),
		theme.Dim.Render("("+string(e.Category)+")"))
}

func countSet(s tui.Stack) int {
	n := 0
	for _, l := range tui.AllLayers {
		if s.IsSet(l) {
			n++
		}
	}
	return n
}

// readyToAdvance is our minimum-viable-stack check for moving to setup.
// At least: an app name plus one technology selection.
func (m Model) readyToAdvance() bool {
	if m.stack.AppName == "" {
		return false
	}
	for _, l := range tui.AllLayers {
		if l == tui.LayerAppType {
			continue
		}
		if m.stack.IsSet(l) {
			return true
		}
	}
	return false
}

// leftPaneWidth is the spec's 22–28-column scaling rule.
func leftPaneWidth(total int) int {
	if total >= 160 {
		return 28
	}
	if total >= 120 {
		return 26
	}
	if total >= 100 {
		return 24
	}
	return 22
}

// sideBySide horizontally joins two already-rendered blocks. Rolls our own
// instead of using lipgloss.JoinHorizontal so we can add a 1-col gutter
// without having to re-measure each pane's rendered width.
func sideBySide(a, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	n := len(aLines)
	if len(bLines) > n {
		n = len(bLines)
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		var l, r string
		if i < len(aLines) {
			l = aLines[i]
		}
		if i < len(bLines) {
			r = bLines[i]
		}
		out[i] = l + r
	}
	return strings.Join(out, "\n")
}
