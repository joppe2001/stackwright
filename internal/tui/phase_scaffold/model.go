// Package phase_scaffold shows the scaffold progress screen — the files
// written, progress bar, and a final summary — while internal/scaffold
// writes the project. Takes the confirmed stack from the root model.
package phase_scaffold

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/scaffold"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// phase is the scaffold's internal state — slightly finer-grained than
// "generating" so we can show the output-dir prompt before writing begins.
type phase int

const (
	phaseDirPrompt phase = iota
	phaseGenerating
	phaseWritingPNG
	phaseDone
	phaseError
)

// Model is the scaffold phase's tea.Model (Update/View driven by root).
type Model struct {
	registry registry.Bundle
	stack    tui.Stack

	phase    phase
	dirInput textinput.Model
	outDir   string

	// ch is the live stream of generated files. Kept on the model so
	// re-scheduling reads across many Update() cycles can share it.
	ch <-chan scaffold.FileResult

	// File progress — streamed via FileResult messages.
	total   int
	written []scaffold.FileResult

	pngPath string
	pngErr  error

	// Top-level error when something fatal happens (e.g., output dir unwritable).
	fatal error

	width  int
	height int
}

func New(bundle registry.Bundle, stack tui.Stack) Model {
	ti := textinput.New()
	ti.Prompt = "▸ "
	ti.CharLimit = 200
	ti.Width = 48
	defaultDir := "./" + defaultDirName(stack.AppName)
	ti.SetValue(defaultDir)
	ti.Focus()
	return Model{
		registry: bundle,
		stack:    stack,
		phase:    phaseDirPrompt,
		dirInput: ti,
	}
}

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

// Init only displays the prompt — work starts when the user confirms the directory.
func (m Model) Init() tea.Cmd { return textinput.Blink }

// ── messages ──────────────────────────────────────────────────────────────

type fileResultMsg struct{ r scaffold.FileResult }

type pngDoneMsg struct{ err error }

// streamFilesCmd drains the FileResult channel from scaffold.Generate and
// re-schedules itself until the channel closes, mirroring the setup wizard's
// line-by-line streaming pattern.
func streamFilesCmd(ch <-chan scaffold.FileResult) tea.Cmd {
	return func() tea.Msg {
		r, ok := <-ch
		if !ok {
			return fileResultMsg{r: scaffold.FileResult{Err: nil, Path: ""}} // sentinel: done
		}
		return fileResultMsg{r: r}
	}
}

// writePNGCmd writes the architecture PNG sibling file. Runs after files
// so the progress UI can show it as a named step.
func writePNGCmd(outDir string, bundle registry.Bundle, stack tui.Stack) tea.Cmd {
	return func() tea.Msg {
		out := outDir + "-architecture.png"
		err := scaffold.WriteArchitecturePNG(out, bundle, stack)
		return pngDoneMsg{err: err}
	}
}

// ── update ────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.onKey(msg)
	case fileResultMsg:
		return m.onFileResult(msg)
	case pngDoneMsg:
		return m.onPNGDone(msg)
	}
	// Forward other messages to the textinput when we're in the prompt.
	if m.phase == phaseDirPrompt {
		var cmd tea.Cmd
		m.dirInput, cmd = m.dirInput.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) onKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch m.phase {
	case phaseDirPrompt:
		if msg.Type == tea.KeyEnter {
			return m.startGenerating()
		}
		var cmd tea.Cmd
		m.dirInput, cmd = m.dirInput.Update(msg)
		return m, cmd
	case phaseDone, phaseError:
		if msg.String() == "enter" || msg.String() == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) startGenerating() (Model, tea.Cmd) {
	dir := strings.TrimSpace(m.dirInput.Value())
	if dir == "" {
		m.fatal = fmt.Errorf("output directory is required")
		m.phase = phaseError
		return m, nil
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		m.fatal = fmt.Errorf("invalid path: %w", err)
		m.phase = phaseError
		return m, nil
	}
	m.outDir = abs
	m.phase = phaseGenerating
	m.ch = scaffold.Generate(abs, m.registry, m.stack)
	return m, streamFilesCmd(m.ch)
}

func (m Model) onFileResult(msg fileResultMsg) (Model, tea.Cmd) {
	r := msg.r
	// Sentinel: first message carries the file count.
	if r.Total > 0 && r.Path == "" {
		m.total = r.Total
		return m, streamFilesCmd(m.ch)
	}
	// Sentinel: closed channel (path empty, err nil, total zero).
	if r.Path == "" && r.Err == nil && r.Total == 0 {
		m.phase = phaseWritingPNG
		return m, writePNGCmd(m.outDir, m.registry, m.stack)
	}
	m.written = append(m.written, r)
	return m, streamFilesCmd(m.ch)
}

func (m Model) onPNGDone(msg pngDoneMsg) (Model, tea.Cmd) {
	m.pngErr = msg.err
	m.pngPath = m.outDir + "-architecture.png"
	m.phase = phaseDone
	return m, nil
}

// ── view ──────────────────────────────────────────────────────────────────

func (m Model) View() string {
	switch m.phase {
	case phaseDirPrompt:
		return m.viewDirPrompt()
	case phaseError:
		return m.viewError()
	}
	return m.viewProgress()
}

func (m Model) viewDirPrompt() string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SCAFFOLD"))
	b.WriteString("\n\n")
	b.WriteString("Output directory:\n\n")
	b.WriteString(m.dirInput.View())
	b.WriteString("\n\n")
	b.WriteString(theme.Dim.Render("enter · generate    esc · back (TUI quits on ctrl+c)"))
	return b.String()
}

func (m Model) viewError() string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SCAFFOLD — error"))
	b.WriteString("\n\n")
	if m.fatal != nil {
		b.WriteString(m.fatal.Error())
	}
	b.WriteString("\n\n")
	b.WriteString(theme.Dim.Render("q · quit"))
	return b.String()
}

func (m Model) viewProgress() string {
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SCAFFOLD"))
	b.WriteString("  ")
	b.WriteString(theme.Dim.Render(fmt.Sprintf("writing %s", m.outDir)))
	b.WriteString("\n\n")

	countOK := 0
	for _, f := range m.written {
		if f.Err == nil {
			countOK++
		}
	}
	b.WriteString(theme.Dim.Render(fmt.Sprintf("%d / %d files", countOK, m.total)))
	b.WriteString("\n\n")

	// Show the last N written files, newest at the bottom for a scrolling feel.
	start := 0
	if len(m.written) > 14 {
		start = len(m.written) - 14
	}
	for _, f := range m.written[start:] {
		mark := theme.Good.Render("✓")
		if f.Err != nil {
			mark = theme.Accent.Render("✗")
		}
		p := f.Path
		if idx := strings.Index(p, m.outDir); idx == 0 && len(p) > len(m.outDir) {
			p = p[len(m.outDir)+1:]
		}
		line := fmt.Sprintf("  %s  %s", mark, p)
		if f.Err != nil {
			line += "  " + theme.Dim.Render(f.Err.Error())
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	switch m.phase {
	case phaseWritingPNG:
		b.WriteString("\n")
		b.WriteString(theme.Dim.Render("  writing architecture PNG…"))
	case phaseDone:
		b.WriteString("\n")
		if m.pngErr != nil {
			b.WriteString("  ")
			b.WriteString(theme.Dim.Render("PNG: " + m.pngErr.Error()))
			b.WriteString("\n")
		} else {
			b.WriteString("  ")
			b.WriteString(theme.Good.Render("✓ " + m.pngPath))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(theme.Good.Render("Done."))
		b.WriteString("  ")
		b.WriteString(theme.Dim.Render(fmt.Sprintf("output: %s", m.outDir)))
		b.WriteString("\n\n")
		b.WriteString(theme.Dim.Render("q · quit"))
	}
	return b.String()
}

// defaultDirName turns the user's app name into a relative directory name
// suitable for the default in the output-dir prompt.
func defaultDirName(appName string) string {
	name := strings.TrimSpace(appName)
	if name == "" {
		return "stackwright-app"
	}
	var b strings.Builder
	prev := byte(0)
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c + 32)
			prev = c + 32
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			b.WriteByte(c)
			prev = c
		default:
			if prev != '-' && b.Len() > 0 {
				b.WriteByte('-')
				prev = '-'
			}
		}
	}
	out := b.String()
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	if out == "" {
		return "stackwright-app"
	}
	return out
}
