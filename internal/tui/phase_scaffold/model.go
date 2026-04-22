// Package phase_scaffold shows the scaffold progress screen while the
// scaffold engine writes the project. Wired in Step 10.
package phase_scaffold

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

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

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) { return m, nil }

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SCAFFOLD (Step 10 will wire the real generator)"))
	b.WriteString("\n\n")
	if m.stack.AppName != "" {
		b.WriteString("would write to: ./" + m.stack.AppName + "/\n\n")
	}
	for _, e := range m.stack.SelectedEntries(m.registry) {
		b.WriteString("  ✓ " + e.Name + "\n")
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim.Render("q · quit"))
	return b.String()
}
