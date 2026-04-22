// Package phase_setup drives the setup wizard: CLI-presence checks,
// installs, account prompts, auth flows, and verification for every
// technology in the confirmed stack. Wired in Step 9.
//
// The Step 4 skeleton renders a placeholder so the root model's phase
// transitions are observable end-to-end. Step 9 replaces Model.View
// with the per-tech state-machine UI from the spec.
package phase_setup

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// Model is the setup-phase model.
type Model struct {
	registry registry.Bundle
	stack    tui.Stack
	width    int
	height   int
}

func New(bundle registry.Bundle, stack tui.Stack) Model {
	return Model{registry: bundle, stack: stack}
}

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		// 'b' goes back to design; real flow in Step 9 won't let users back
		// out once commands start running, but the skeleton keeps it simple.
		if k.String() == "b" {
			return m, func() tea.Msg { return tui.PhaseChangeMsg{To: tui.PhaseDesign} }
		}
		if k.String() == "g" {
			return m, func() tea.Msg { return tui.PhaseChangeMsg{To: tui.PhaseScaffold} }
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SETUP (Step 9 will wire the real wizard)"))
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("app: %s\n\n", m.stack.AppName))

	for _, e := range m.stack.SelectedEntries(m.registry) {
		cli := "no CLI"
		if e.CLI != nil {
			cli = e.CLI.Binary
		}
		auth := "no auth"
		if e.Auth != nil && e.Auth.Required {
			auth = "auth required"
		}
		b.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
			theme.Good.Render("○"),
			e.Name,
			theme.Dim.Render("cli="+cli),
			theme.Dim.Render(auth),
		))
	}

	b.WriteString("\n")
	b.WriteString(theme.Dim.Render("b · back to design    g · advance to scaffold (placeholder)"))
	return b.String()
}
