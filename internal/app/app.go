// Package app is the bubbletea root model that wires every phase sub-model
// into one tea.Program. Lives outside internal/tui so the tui package can
// own the shared types (Stack, Layer, messages) without creating an import
// cycle between tui and its phase subpackages.
package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/joppe2001/stackwright/internal/detect"
	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/phase_design"
	"github.com/joppe2001/stackwright/internal/tui/phase_scaffold"
	"github.com/joppe2001/stackwright/internal/tui/phase_setup"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// Opts carries everything Run needs to start the TUI.
type Opts struct {
	Capabilities detect.Capabilities
	Registry     registry.LoadResult
	NoKitty      bool
	Offline      bool
}

// Run builds the bubbletea.Program and blocks until the user exits.
// Any program error (fatal panic, terminal lost, etc.) is returned.
func Run(opts Opts) error {
	m := newRootModel(opts)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

// rootModel owns the phase state machine and window dimensions. Phase
// sub-models are stored by value so Update can return an updated rootModel
// without shared-mutation concerns.
type rootModel struct {
	opts Opts

	phase    tui.Phase
	design   phase_design.Model
	setup    phase_setup.Model
	scaffold phase_scaffold.Model

	width  int
	height int

	// Snapshot of the user's stack. Phases emit tui.StackUpdateMsg; the root
	// stores the latest snapshot here so it can hand it to later phases on
	// transition.
	stack tui.Stack
}

func newRootModel(opts Opts) rootModel {
	visual := opts.Capabilities.VisualMode(opts.NoKitty)
	return rootModel{
		opts:   opts,
		phase:  tui.PhaseDesign,
		design: phase_design.New(opts.Registry.Bundle, visual),
		stack:  tui.NewStack(),
	}
}

func (m rootModel) Init() tea.Cmd { return nil }

func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		w, h := m.phaseArea()
		m.design.SetSize(w, h)
		m.setup.SetSize(w, h)
		m.scaffold.SetSize(w, h)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			// Tear down any running child process so brew / flyctl / aws
			// don't keep running after the TUI exits.
			if m.phase == tui.PhaseSetup {
				m.setup.Cancel()
			}
			return m, tea.Quit
		}

	case tui.StackUpdateMsg:
		m.stack = msg.Stack
		// Keep the design model's internal snapshot in sync. Ignore the
		// emitted Cmd (it would be nil — design just updates state).
		m.design, _ = m.design.Update(msg)
		return m, nil

	case tui.PhaseChangeMsg:
		next, cmd := m.transition(msg.To)
		return next, cmd
	}

	// Route everything else to the active phase.
	var cmd tea.Cmd
	switch m.phase {
	case tui.PhaseDesign:
		m.design, cmd = m.design.Update(msg)
	case tui.PhaseSetup:
		m.setup, cmd = m.setup.Update(msg)
	case tui.PhaseScaffold:
		m.scaffold, cmd = m.scaffold.Update(msg)
	}
	return m, cmd
}

// transition advances to a new phase, copying the current stack forward.
// Later phases are lazily constructed so they always see the latest stack.
// Returns the new root model plus the phase's Init command so the child
// can kick off any initial work (e.g., setup runs a CLI check immediately).
func (m rootModel) transition(to tui.Phase) (rootModel, tea.Cmd) {
	// Defensive: grab the design model's stack in case we somehow missed a
	// StackUpdateMsg before the user pressed 'g'.
	if ds := m.design.Stack(); ds.AppName != "" && (len(ds.Selections) > len(m.stack.Selections) || ds.AppName != m.stack.AppName) {
		m.stack = ds
	}

	m.phase = to
	w, h := m.phaseArea()
	var cmd tea.Cmd
	switch to {
	case tui.PhaseSetup:
		m.setup = phase_setup.New(m.opts.Registry.Bundle, m.stack)
		m.setup.SetSize(w, h)
		cmd = m.setup.Init()
	case tui.PhaseScaffold:
		m.scaffold = phase_scaffold.New(m.opts.Registry.Bundle, m.stack)
		m.scaffold.SetSize(w, h)
		cmd = m.scaffold.Init()
	}
	return m, cmd
}

func (m rootModel) View() string {
	if m.width == 0 {
		return ""
	}
	var body string
	switch m.phase {
	case tui.PhaseDesign:
		body = m.design.View()
	case tui.PhaseSetup:
		body = m.setup.View()
	case tui.PhaseScaffold:
		body = m.scaffold.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderTitleBar(),
		body,
		m.renderStatusBar(),
	)
}

// phaseArea returns the inner area available to phase content after
// subtracting the title and status bars.
func (m rootModel) phaseArea() (int, int) {
	return m.width, m.height - 2
}

func (m rootModel) renderTitleBar() string {
	name := m.stack.AppName
	if name == "" {
		name = "— untitled stack"
	}
	var src string
	switch m.opts.Registry.Source {
	case registry.SourceNetwork:
		src = theme.Good.Render("● live")
	case registry.SourceCache:
		src = theme.Dim.Render("● cached")
	default:
		src = theme.Dim.Render("● bundled")
	}
	mode := "standard"
	if m.opts.Capabilities.VisualMode(m.opts.NoKitty) {
		mode = "visual"
	}
	offline := ""
	if m.opts.Offline {
		offline = "  ·  " + theme.Dim.Render("offline")
	}
	title := fmt.Sprintf("stackwright  ·  %s  ·  phase: %s  ·  mode: %s  ·  %s%s",
		name, m.phase, mode, src, offline)
	return theme.TitleBar.Width(m.width).Render(title)
}

func (m rootModel) renderStatusBar() string {
	var hint string
	switch m.phase {
	case tui.PhaseDesign:
		hint = "↑↓ nav  ·  space/enter open  ·  / search  ·  del clear  ·  g generate  ·  q quit"
	case tui.PhaseSetup:
		hint = "b back  ·  g advance  ·  q quit"
	case tui.PhaseScaffold:
		hint = "q quit"
	}
	return theme.StatusBar.Width(m.width).Render(hint)
}
