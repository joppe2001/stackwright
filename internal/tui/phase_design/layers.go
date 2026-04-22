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
	case "a":
		// Request the design model to open the unknown-tech modal for this layer.
		return openModalCmd(m.subLayer), true
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

// closeSub returns to the top-level layer list (used after the unknown-tech
// modal finishes; the design model calls this on the layers component).
func (m *layersModel) closeSub() { m.mode = modeLayerList }

// openModalMsg asks the parent design model to open the unknown-tech modal
// for the given layer. We route via a message instead of a direct call so
// the design model owns all modal state.
type openModalMsg struct{ layer tui.Layer }

func openModalCmd(l tui.Layer) tea.Cmd {
	return func() tea.Msg { return openModalMsg{layer: l} }
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

// viewLayerList renders the two-line-per-layer left-pane navigator.
// Layout per layer (pane width w, no trailing border):
//
//	●  Frontend                      ✓     <- dot + bold name + right-aligned tick
//	   Next.js 14                         <- indented muted selected value
//
// The selected row's name line is highlighted by swapping the name style
// to accent-bold; a leading ▎ bar gives the row a visible cursor.
func (m layersModel) viewLayerList(w, h int) string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("stack layers"))
	b.WriteString("\n\n")

	for i, l := range tui.AllLayers {
		b.WriteString(m.renderLayerRow(i, l, w))
		b.WriteString("\n")
	}

	// Leave a blank line, then the confirm button + key hints.
	b.WriteString("\n")
	if m.canGenerate() {
		b.WriteString(theme.Good.Render("  ↓ confirm & generate  "))
		b.WriteString("\n\n")
	}
	b.WriteString(theme.Dim.Render("space open  ·  del clear"))
	return fitToHeight(b.String(), h)
}

// renderLayerRow draws one two-line layer entry.
// w is the pane inner width in cells; we right-align the tick and truncate
// the selected value so nothing wraps.
func (m layersModel) renderLayerRow(i int, l tui.Layer, w int) string {
	selected := m.cursor == i
	isSet := m.stack.IsSet(l)

	// Left gutter: "▎ " for the selected row, "  " otherwise. Width 2.
	gutter := "  "
	if selected {
		gutter = theme.Accent.Render("▎ ")
	}

	// Colored dot — tech color when set, muted when not.
	dotColor := theme.TextMuted
	if isSet {
		dotColor = m.layerDotColor(l)
	}
	dot := lipgloss.NewStyle().Foreground(lipgloss.Color(dotColor)).Render("●")

	// Name: bold + accent when the row is focused; primary otherwise.
	name := tui.LayerTitle(l)
	nameStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.TextPrimary))
	if selected {
		nameStyle = nameStyle.Foreground(lipgloss.Color(theme.AccentPurple)).Bold(true)
	}

	// Tick or em-dash on the right edge.
	tick := theme.Dim.Render(" ")
	if isSet {
		tick = theme.Good.Render("✓")
	}

	// Compute the padding that places the tick at column w-2.
	// Layout is:  "gutter(2) dot(1) space(1) name  ...  tick(1)"
	// Spaces between name and tick = w - 2 - 1 - 1 - nameLen - 1.
	nameLen := lipgloss.Width(name)
	pad := w - 2 - 1 - 1 - nameLen - 1
	if pad < 1 {
		pad = 1
	}

	line1 := gutter + dot + " " + nameStyle.Render(name) + strings.Repeat(" ", pad) + tick

	// Second line: indented muted value, hard-truncated to w-4.
	value := m.selectedValueText(l)
	maxVal := w - 4
	if maxVal < 4 {
		maxVal = 4
	}
	if lipgloss.Width(value) > maxVal {
		runes := []rune(value)
		if len(runes) > maxVal-1 {
			value = string(runes[:maxVal-1]) + "…"
		}
	}
	line2 := "    " + theme.LayerValueMuted.Render(value)

	return line1 + "\n" + line2
}

// selectedValueText returns the muted second-line text for a layer.
// Returns "not set" when empty and the human name of the selected tech otherwise.
func (m layersModel) selectedValueText(l tui.Layer) string {
	if l == tui.LayerAppType {
		if m.stack.AppName == "" {
			return "not set"
		}
		return m.stack.AppName
	}
	slug := m.stack.Slug(l)
	if slug == "" {
		return "not set"
	}
	if e, ok := m.registry.BySlug(slug); ok {
		return e.Name
	}
	return slug
}

// layerDotColor returns the hex color to use for the row's leading dot.
// When the layer has a selected tech, use that tech's diagram_color; otherwise
// fall back to a per-layer default so the dot column doesn't look monochrome.
func (m layersModel) layerDotColor(l tui.Layer) string {
	if l == tui.LayerAppType {
		return theme.AccentPurple
	}
	if slug := m.stack.Slug(l); slug != "" {
		if e, ok := m.registry.BySlug(slug); ok && e.DiagramColor != "" {
			return e.DiagramColor
		}
	}
	// Per-layer fallback palette roughly matching the mockup.
	switch l {
	case tui.LayerFrontend:
		return "#7c6ff7"
	case tui.LayerBackend:
		return "#2dd4a0"
	case tui.LayerAuth:
		return "#a78bfa"
	case tui.LayerDatabase:
		return "#6aa2ce"
	case tui.LayerCache:
		return "#f59e0b"
	case tui.LayerPayments:
		return "#ec4899"
	case tui.LayerInfra:
		return "#22c55e"
	case tui.LayerCICD:
		return "#2088ff"
	}
	return theme.TextMuted
}

// canGenerate mirrors the readyToAdvance check on the parent Model — the
// confirm-button is only shown when pressing 'g' would actually advance.
func (m layersModel) canGenerate() bool {
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
		b.WriteString(theme.Dim.Render("press 'a' to add a new technology"))
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
		b.WriteString(theme.Dim.Render("↑↓ nav   / search   a add new   enter select   esc back"))
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
