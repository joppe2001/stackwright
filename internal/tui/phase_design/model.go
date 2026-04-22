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
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/phase_design/diagram"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// tickMsg advances the diagram's animation frame.
type tickMsg struct{}

// tickInterval: 100ms = 10fps. Gentle on bubbletea's diffing and enough for
// visible particle motion on the connection traces.
const tickInterval = 100 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// Model is the design phase's tea.Model. Not literally a tea.Model because
// phases don't run their own tea.Program — the root model calls their
// Update/View directly with the shared window size.
type Model struct {
	registry registry.Bundle
	stack    tui.Stack

	layers layersModel
	modal  unknownModal

	// leftWidth is how wide the left pane gets. Rest is the diagram.
	leftWidth int
	// width/height are the design-phase content area (inside any parent chrome).
	width  int
	height int

	// frame is the animation counter incremented by tickMsg. Used by the
	// diagram renderer to advance particle positions.
	frame int

	// visualMode toggles the Kitty GFX renderer (true) vs the ANSI renderer (false).
	visualMode bool

	// narrowShowsDiagram toggles, in <80-col layouts, between the layer list
	// (false, default) and the diagram (true). Triggered with 'd' when no
	// modal is open.
	narrowShowsDiagram bool
}

// New returns a fresh design-phase model seeded with the registry and an empty stack.
// visualMode is set by the root model based on the capability probe + --no-kitty.
func New(bundle registry.Bundle, visualMode bool) Model {
	stack := tui.NewStack()
	return Model{
		registry:   bundle,
		stack:      stack,
		layers:     newLayersModel(bundle, stack),
		modal:      newUnknownModal(),
		leftWidth:  24,
		visualMode: visualMode,
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

// Init starts the animation tick loop.
func (m Model) Init() tea.Cmd { return tickCmd() }

// Update handles key events and the animation tick for the design phase.
// Unknown keys fall through so the root model can handle globals.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	// Modal takes priority when visible.
	if m.modal.visible {
		entry, saved, cancelled, cmd := m.modal.Update(msg)
		if saved {
			// Add to in-memory bundle so the layer navigator finds it immediately.
			m.registry.Entries = append(m.registry.Entries, entry)
			m.layers.registry = m.registry
			// Auto-select the new entry for the layer it was created under.
			m.stack = m.stack.WithSelection(m.modal.forLayer, entry.Slug)
			m.layers.syncStack(m.stack)
			m.layers.closeSub()
			return m, tea.Batch(cmd, emitStack(m.stack))
		}
		if cancelled {
			m.layers.closeSub()
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tickMsg:
		m.frame = (m.frame + 1) % 10000
		return m, tickCmd()
	case tui.StackUpdateMsg:
		m.stack = msg.Stack
		m.layers.syncStack(m.stack)
		return m, nil
	case openModalMsg:
		m.modal.Open(msg.layer)
		return m, nil
	case tea.KeyMsg:
		// 'g' advances to setup — but only if there's at least one selection.
		if msg.String() == "g" && m.layers.mode == modeLayerList {
			if m.readyToAdvance() {
				return m, func() tea.Msg { return tui.PhaseChangeMsg{To: tui.PhaseSetup} }
			}
		}
		// 'd' toggles the narrow layout between layer list and diagram —
		// only when the terminal is narrow enough that both panes don't fit.
		if msg.String() == "d" && m.width < 80 && m.layers.mode == modeLayerList {
			m.narrowShowsDiagram = !m.narrowShowsDiagram
			return m, nil
		}
		cmd, _ := m.layers.Update(msg)
		return m, cmd
	}
	return m, nil
}

// View renders the full design phase at the current window size.
// When the unknown-tech modal is visible, it's composited on top.
func (m Model) View() string {
	var body string
	if m.width < 80 {
		body = m.viewNarrow()
	} else {
		body = m.viewWide()
	}
	if m.modal.visible {
		// Place the modal over the composed body. lipgloss.Place + the body
		// height keeps the background visible as context behind the overlay.
		return m.modal.View(m.width, m.height)
	}
	return body
}

func (m Model) viewWide() string {
	// 2 cols lost to each pane's border + 2 cols padding inside.
	leftContentW := m.leftWidth - 4
	rightContentW := m.width - m.leftWidth - 4
	innerH := m.height - 2

	leftBody := m.layers.View(leftContentW, innerH-2)
	rightBody := m.renderDiagram(rightContentW, innerH-2)

	left := theme.Pane.
		Width(leftContentW).
		Height(innerH - 2).
		Render(leftBody)

	right := theme.Pane.
		Width(rightContentW).
		Height(innerH - 2).
		Render(rightBody)

	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m Model) viewNarrow() string {
	// Narrow layout: one pane at a time. 'd' toggles between layers and diagram.
	contentW := m.width - 4
	contentH := m.height - 4
	var body string
	if m.narrowShowsDiagram {
		hint := theme.Dim.Render("[d] toggle to layers")
		body = hint + "\n" + m.renderDiagram(contentW, contentH-1)
	} else {
		hint := theme.Dim.Render("[d] toggle to diagram")
		body = hint + "\n" + m.layers.View(contentW, contentH-1)
	}
	return theme.Pane.
		Width(contentW).
		Height(contentH).
		Render(body)
}

// renderDiagram produces the right-pane content. The dot-grid canvas is
// always rendered so the pane never looks empty; nodes fade in as layers
// get selections, and a tiny overlay message guides the user until then.
func (m Model) renderDiagram(w, h int) string {
	// Reserve 1 line for the header and 1 for the footer.
	bodyH := h - 2
	if bodyH < 8 {
		bodyH = 8
	}

	layout := diagram.Compute(m.stack, m.registry, w, bodyH)

	var body string
	if m.visualMode {
		// Visual (Kitty) mode emits an APC sequence the terminal displays
		// out-of-band of the cell grid; it's opt-in behind --kitty because
		// bubbletea's cell-layout diff can't reclaim image pixels on resize.
		view, err := diagram.RenderKittyView(layout, m.frame, w, bodyH)
		if err != nil {
			body = diagram.RenderStandard(layout, m.frame)
		} else {
			body = view
		}
	} else {
		body = diagram.RenderStandard(layout, m.frame)
	}

	// Header: "live diagram" + badge for data-flow animation state.
	header := theme.Accent.Render("live diagram")
	if m.stack.AppName != "" {
		header += "   " + theme.Good.Render("● "+m.stack.AppName)
	}

	// Footer hint.
	var footer string
	switch {
	case m.readyToAdvance():
		footer = theme.Good.Render("press 'g' to continue to setup")
	case m.stack.AppName == "":
		footer = theme.Dim.Render("set an app name, then pick a technology for a layer")
	case len(layout.Nodes) == 0:
		footer = theme.Dim.Render("pick a technology for a layer to see it appear")
	default:
		footer = theme.Dim.Render("keep selecting layers…")
	}

	return header + "\n" + body + "\n" + footer
}

// countSet reports how many layers in the stack are populated.
// Kept for future use by the setup/scaffold phases' stack previews.
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

// (sideBySide removed — the design phase now uses lipgloss.JoinHorizontal
// after the rounded-pane redesign.)
var _ = strings.Split // keep strings imported for other uses
