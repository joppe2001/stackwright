package phase_design

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// layerMode is what the left pane is currently showing.
type layerMode int

const (
	modeLayerList   layerMode = iota // top-level layer list
	modeSubList                      // entries for the selected layer's category
	modeAppNameEdit                  // text input for the app name (App type layer)
)

// layersModel is the left-pane component. It owns:
//   - the currently focused layer row (cursor in the top-level list)
//   - the sub-list of registry entries when the user opens a layer
//   - a text input used by the App type layer
//   - a fuzzy-search buffer used while browsing a sub-list
//
// Selections are written back to the parent via StackUpdateMsg so the parent
// remains the single source of truth for the user's Stack.
type layersModel struct {
	registry registry.Bundle
	stack    tui.Stack

	mode        layerMode
	cursor      int // row index in the current list
	subCursor   int // row index within the active sub-list
	subLayer    tui.Layer
	subQuery    string // fuzzy-search buffer (populated after pressing /)
	subSearching bool
	nameInput   textinput.Model

	width  int
	height int
}

func newLayersModel(bundle registry.Bundle, stack tui.Stack) layersModel {
	ti := textinput.New()
	ti.Placeholder = "my-saas"
	ti.CharLimit = 40
	ti.Prompt = "▸ "
	return layersModel{
		registry:  bundle,
		stack:     stack,
		mode:      modeLayerList,
		nameInput: ti,
	}
}

// setSize is called by the parent whenever the window is resized.
func (m *layersModel) setSize(w, h int) {
	m.width = w
	m.height = h
	m.nameInput.Width = w - 4
}

// syncStack replaces the local stack snapshot (called by parent when design
// receives a StackUpdateMsg that originated elsewhere, e.g., unknown-tech modal).
func (m *layersModel) syncStack(s tui.Stack) { m.stack = s }

// Update handles key events. Returns a command that may include a
// StackUpdateMsg for the parent to route.
func (m *layersModel) Update(msg tea.Msg) (tea.Cmd, bool /*consumed*/) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.mode {
		case modeLayerList:
			return m.updateLayerList(msg)
		case modeSubList:
			return m.updateSubList(msg)
		case modeAppNameEdit:
			return m.updateAppNameEdit(msg)
		}
	}
	return nil, false
}

func (m *layersModel) updateLayerList(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return nil, true
	case "down", "j":
		if m.cursor < len(tui.AllLayers)-1 {
			m.cursor++
		}
		return nil, true
	case "enter", " ":
		return m.enterLayer(tui.AllLayers[m.cursor])
	case "backspace", "delete":
		// Clear the selected layer.
		return m.clearLayer(tui.AllLayers[m.cursor])
	}
	return nil, false
}

func (m *layersModel) enterLayer(l tui.Layer) (tea.Cmd, bool) {
	if l == tui.LayerAppType {
		m.mode = modeAppNameEdit
		m.nameInput.SetValue(m.stack.AppName)
		m.nameInput.Focus()
		return textinput.Blink, true
	}
	if _, ok := tui.LayerCategory(l); !ok {
		return nil, true
	}
	m.mode = modeSubList
	m.subLayer = l
	m.subCursor = 0
	m.subQuery = ""
	m.subSearching = false
	// If the layer already has a selection, pre-position cursor on it.
	current := m.stack.Slug(l)
	if current != "" {
		list := m.currentSubList()
		for i, r := range list {
			if r.Entry.Slug == current {
				m.subCursor = i
				break
			}
		}
	}
	return nil, true
}

func (m *layersModel) clearLayer(l tui.Layer) (tea.Cmd, bool) {
	var next tui.Stack
	if l == tui.LayerAppType {
		next = m.stack.WithAppName("")
	} else {
		next = m.stack.WithSelection(l, "")
	}
	m.stack = next
	return emitStack(next), true
}

func (m *layersModel) updateSubList(msg tea.KeyMsg) (tea.Cmd, bool) {
	// Search-input mode: most keys go into the query buffer.
	if m.subSearching {
		switch msg.Type {
		case tea.KeyEsc:
			m.subSearching = false
			m.subQuery = ""
			m.subCursor = 0
			return nil, true
		case tea.KeyEnter:
			m.subSearching = false
			return nil, true
		case tea.KeyBackspace:
			if len(m.subQuery) > 0 {
				m.subQuery = m.subQuery[:len(m.subQuery)-1]
				m.subCursor = 0
			}
			return nil, true
		case tea.KeyRunes:
			m.subQuery += string(msg.Runes)
			m.subCursor = 0
			return nil, true
		}
		return nil, true
	}

	switch msg.String() {
	case "esc":
		m.mode = modeLayerList
		return nil, true
	case "up", "k":
		if m.subCursor > 0 {
			m.subCursor--
		}
		return nil, true
	case "down", "j":
		list := m.currentSubList()
		if m.subCursor < len(list)-1 {
			m.subCursor++
		}
		return nil, true
	case "/":
		m.subSearching = true
		m.subQuery = ""
		return nil, true
	case "enter", " ":
		list := m.currentSubList()
		if len(list) == 0 {
			return nil, true
		}
		selected := list[m.subCursor].Entry
		m.stack = m.stack.WithSelection(m.subLayer, selected.Slug)
		// Advance to the next unset layer for smoother flow.
		m.cursor = nextUnsetLayerAfter(m.stack, m.cursor)
		m.mode = modeLayerList
		return emitStack(m.stack), true
	}
	return nil, false
}

func (m *layersModel) updateAppNameEdit(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = modeLayerList
		m.nameInput.Blur()
		return nil, true
	case tea.KeyEnter:
		m.stack = m.stack.WithAppName(strings.TrimSpace(m.nameInput.Value()))
		m.mode = modeLayerList
		m.nameInput.Blur()
		// Move to the next layer automatically.
		if m.cursor == 0 && m.stack.AppName != "" {
			m.cursor = 1
		}
		return emitStack(m.stack), true
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return cmd, true
}

// currentSubList returns the entries visible in the sub-list, with the active
// fuzzy-search query applied.
func (m layersModel) currentSubList() []registry.SearchResult {
	cat, _ := tui.LayerCategory(m.subLayer)
	return m.registry.Search(m.subQuery, cat)
}

// View renders the left pane at the given inner size (w x h). The caller is
// responsible for the surrounding border.
func (m layersModel) View(w, h int) string {
	switch m.mode {
	case modeAppNameEdit:
		return m.viewAppNameEdit(w, h)
	case modeSubList:
		return m.viewSubList(w, h)
	}
	return m.viewLayerList(w, h)
}

func (m layersModel) viewLayerList(w, h int) string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("STACK LAYERS"))
	b.WriteString("\n\n")
	for i, l := range tui.AllLayers {
		marker := " "
		if m.cursor == i {
			marker = "▶"
		}
		name := tui.LayerTitle(l)
		value := m.layerValue(l)
		tick := " "
		if m.stack.IsSet(l) {
			tick = theme.Good.Render("✓")
		} else {
			tick = theme.Dim.Render("–")
		}
		row := fmt.Sprintf("%s %-12s %s  %s", marker, name, tick, value)
		if m.cursor == i {
			row = theme.LayerRowSelected.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(theme.Dim.Render("space/enter · open   del · clear"))
	return fitToHeight(b.String(), h)
}

func (m layersModel) viewSubList(w, h int) string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render(strings.ToUpper(tui.LayerTitle(m.subLayer))))
	b.WriteString("\n")
	if m.subSearching || m.subQuery != "" {
		prompt := "/" + m.subQuery
		if m.subSearching {
			prompt += "▎"
		}
		b.WriteString(theme.Dim.Render(prompt))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	list := m.currentSubList()
	if len(list) == 0 {
		b.WriteString(theme.Dim.Render("no matches — press esc to go back"))
		b.WriteString("\n")
		b.WriteString(theme.Dim.Render("(Step 7 will add: press 'a' to add new tech)"))
		return fitToHeight(b.String(), h)
	}
	maxRows := h - 6
	if maxRows < 4 {
		maxRows = 4
	}
	start := 0
	if m.subCursor >= maxRows {
		start = m.subCursor - maxRows + 1
	}
	end := start + maxRows
	if end > len(list) {
		end = len(list)
	}
	for i := start; i < end; i++ {
		r := list[i]
		marker := " "
		if i == m.subCursor {
			marker = "▶"
		}
		row := fmt.Sprintf("%s %s", marker, r.Entry.Name)
		if i == m.subCursor {
			row = theme.LayerRowSelected.Render(row)
		}
		b.WriteString(row)
		b.WriteString("\n")
	}
	b.WriteString("\n")
	if m.subSearching {
		b.WriteString(theme.Dim.Render("typing · searching   enter · accept   esc · cancel"))
	} else {
		b.WriteString(theme.Dim.Render("↑↓ nav   / search   enter select   esc back"))
	}
	return fitToHeight(b.String(), h)
}

func (m layersModel) viewAppNameEdit(w, h int) string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("APP NAME"))
	b.WriteString("\n\n")
	b.WriteString(m.nameInput.View())
	b.WriteString("\n\n")
	b.WriteString(theme.Dim.Render("enter · accept   esc · cancel"))
	return fitToHeight(b.String(), h)
}

// layerValue renders the muted "selected value" text shown next to each row.
func (m layersModel) layerValue(l tui.Layer) string {
	if l == tui.LayerAppType {
		if m.stack.AppName == "" {
			return theme.LayerValueMuted.Render("not set")
		}
		return theme.LayerValueMuted.Render(m.stack.AppName)
	}
	slug := m.stack.Slug(l)
	if slug == "" {
		return theme.LayerValueMuted.Render("not set")
	}
	if e, ok := m.registry.BySlug(slug); ok {
		return theme.LayerValueMuted.Render(e.Name)
	}
	return theme.LayerValueMuted.Render(slug + " (unknown)")
}

func nextUnsetLayerAfter(s tui.Stack, from int) int {
	for i := from + 1; i < len(tui.AllLayers); i++ {
		if !s.IsSet(tui.AllLayers[i]) {
			return i
		}
	}
	// Wrap: find any unset.
	for i := 0; i < len(tui.AllLayers); i++ {
		if !s.IsSet(tui.AllLayers[i]) {
			return i
		}
	}
	return from
}

// fitToHeight truncates the output to h lines so we never overflow the pane.
// Callers who need to pad to a fixed height can wrap in a lipgloss Height().
func fitToHeight(s string, h int) string {
	if h <= 0 {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// emitStack is a small helper so update methods stay one-line for the message-emit path.
func emitStack(s tui.Stack) tea.Cmd {
	return func() tea.Msg { return tui.StackUpdateMsg{Stack: s} }
}
