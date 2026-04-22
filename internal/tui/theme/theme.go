// Package theme centralizes every lipgloss style and color token used by the
// TUI. It exists as its own package (rather than a styles.go in tui/) so the
// root tui package can import phase packages AND phase packages can import
// theme — two otherwise-conflicting imports would create a cycle if the
// styles lived in tui itself.
package theme

import "github.com/charmbracelet/lipgloss"

// Design-system color tokens (see the color-palette table in the spec).
// Tech-specific colors live on registry entries, not here.
const (
	CanvasBG       = "#0b0b0f"
	Surface        = "#0f0f1a"
	SurfaceRaised  = "#13131a"
	BorderDefault  = "#1c1c2e"
	BorderHover    = "#2a2a40"
	BorderActive   = "#3a3a55"
	TextPrimary    = "#c8c6e0"
	TextSecondary  = "#6060a0"
	TextMuted      = "#303050"
	AccentPurple   = "#7c6ff7"
	Teal           = "#2dd4a0"
)

// Colors: pre-resolved lipgloss.Color values. Using Color() everywhere inline
// is noisy — these let call sites read like `theme.CPrimary`.
var (
	CCanvasBG      = lipgloss.Color(CanvasBG)
	CSurface       = lipgloss.Color(Surface)
	CSurfaceRaised = lipgloss.Color(SurfaceRaised)
	CBorderDefault = lipgloss.Color(BorderDefault)
	CBorderActive  = lipgloss.Color(BorderActive)
	CPrimary       = lipgloss.Color(TextPrimary)
	CSecondary     = lipgloss.Color(TextSecondary)
	CMuted         = lipgloss.Color(TextMuted)
	CAccent        = lipgloss.Color(AccentPurple)
	CTeal          = lipgloss.Color(Teal)
)

// TitleBar is the top strip that shows "stackwright — <app name>", phase
// counter, and the live-indicator dot.
var TitleBar = lipgloss.NewStyle().
	Foreground(CPrimary).
	Background(CSurfaceRaised).
	Padding(0, 2).
	Bold(true)

// StatusBar is the bottom keybinding hints row.
var StatusBar = lipgloss.NewStyle().
	Foreground(CSecondary).
	Background(CSurfaceRaised).
	Padding(0, 2)

// PaneBorder frames both panes (left navigator, right diagram).
var PaneBorder = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(CBorderDefault)

// PaneBorderActive highlights the currently-focused pane.
var PaneBorderActive = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(CBorderActive)

// LayerRow styles one row in the left-pane layer list.
var (
	LayerRowIdle     = lipgloss.NewStyle().Foreground(CPrimary)
	LayerRowSelected = lipgloss.NewStyle().Foreground(CAccent).Bold(true)
	LayerRowDone     = lipgloss.NewStyle().Foreground(CTeal)
	LayerValueMuted  = lipgloss.NewStyle().Foreground(CSecondary).Italic(true)
)

// NodeCard is the default right-pane node rendering (used by both standard
// and Kitty modes where text labels are drawn).
var NodeCard = lipgloss.NewStyle().
	Border(lipgloss.NormalBorder()).
	BorderForeground(CBorderDefault).
	Background(CSurface).
	Foreground(CPrimary).
	Padding(0, 1)

// Modal frames the unknown-tech overlay (Step 7).
var Modal = lipgloss.NewStyle().
	Border(lipgloss.ThickBorder()).
	BorderForeground(CAccent).
	Background(CSurfaceRaised).
	Padding(1, 2)

// Dim is for secondary/helper text.
var Dim = lipgloss.NewStyle().Foreground(CSecondary)

// Accent is for highlighted numbers / counters.
var Accent = lipgloss.NewStyle().Foreground(CAccent).Bold(true)

// Good is for success/✓ marks (teal).
var Good = lipgloss.NewStyle().Foreground(CTeal)

// KeyPill is the rounded-border key hint used in the status bar.
// Renders the key in accent-bold inside a subtle surface pill.
var KeyPill = lipgloss.NewStyle().
	Foreground(CAccent).
	Background(CSurface).
	Padding(0, 1).
	Bold(true)

// Pane is a rounded thick border used as the outer pane frame — gives the
// TUI the "app window" feel from the mockup.
var Pane = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(CBorderDefault).
	Padding(0, 1)

// PaneFocused mirrors Pane but with the active-border color.
var PaneFocused = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(CBorderActive).
	Padding(0, 1)
