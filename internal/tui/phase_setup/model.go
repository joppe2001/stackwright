// Package phase_setup drives the setup wizard: CLI-presence checks, installs,
// account prompts, auth flows, and verification for every technology in the
// confirmed stack. Each technology runs through a small state machine
// (see techState below); the model renders the current tech in detail while
// showing a progress list of peers above it.
package phase_setup

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
	"github.com/joppe2001/stackwright/internal/tui/theme"
)

// techState tracks where in the setup state machine a given tech is.
// See the state diagram in the spec.
type techState int

const (
	stPending techState = iota
	stChecking
	stInstallPrompt
	stInstalling
	stInstallFailed
	stAccountPrompt
	stAuthPrompt
	stAuthRunning
	stVerifying
	stDone
	stSkipped
	stRemoved
)

// techProgress is the per-tech state slice. `output` keeps the last N lines
// of streaming stdout so the detail pane can render the live tail.
type techProgress struct {
	entry    registry.Entry
	state    techState
	version  string
	identity string
	output   []string
	errMsg   string
}

// maxOutputLines bounds how much streaming output we keep in memory per tech.
// Matches the 8-line display box from the spec plus a buffer for scroll.
const maxOutputLines = 128

// Model is the setup phase's tea.Model-ish object (Update/View called by root).
type Model struct {
	registry registry.Bundle
	stack    tui.Stack

	items []techProgress
	cur   int // index of the current in-progress tech

	// proc is the currently-running child process, if any. Nil between steps.
	proc *ProcessHandle

	width  int
	height int
}

// New builds a setup model from the confirmed stack.
// The selected entries are sorted into dependency order via SetupOrder.
func New(bundle registry.Bundle, stack tui.Stack) Model {
	entries := SetupOrder(stack.SelectedEntries(bundle))
	items := make([]techProgress, len(entries))
	for i, e := range entries {
		items[i] = techProgress{entry: e, state: stPending}
	}
	return Model{
		registry: bundle,
		stack:    stack,
		items:    items,
		cur:      0,
	}
}

func (m *Model) SetSize(w, h int) { m.width = w; m.height = h }

// Init kicks off the wizard by starting the first tech's CHECKING step.
func (m Model) Init() tea.Cmd {
	if len(m.items) == 0 {
		return nil
	}
	return startCheckCmd(m.cur, m.items[m.cur].entry)
}

// ── Messages ──────────────────────────────────────────────────────────────

type checkDoneMsg struct {
	idx     int
	version string
	ok      bool
}

type processLineMsg struct {
	idx  int
	line string
}

type processDoneMsg struct {
	idx      int
	exitCode int
	err      error
}

type verifyDoneMsg struct {
	idx      int
	ok       bool
	identity string
}

// ── Command factories ─────────────────────────────────────────────────────

// startCheckCmd probes whether the tech's CLI is present on PATH.
// For techs with no CLI (registry.CLI == nil), this treats the tech as
// "already installed" and proceeds to the account/auth steps.
func startCheckCmd(idx int, e registry.Entry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		v, ok := IsInstalled(ctx, e)
		return checkDoneMsg{idx: idx, version: v, ok: ok}
	}
}

// readLineCmd pulls the next line (or close) off a ProcessHandle. Re-scheduled
// from Update each time a lineMsg is received so the stream flows continuously.
func readLineCmd(idx int, h *ProcessHandle) tea.Cmd {
	if h == nil {
		return nil
	}
	return func() tea.Msg {
		line, ok := <-h.Lines
		if !ok {
			// Channel closed — wait for final exit result.
			res := <-h.Done
			return processDoneMsg{idx: idx, exitCode: res.ExitCode, err: res.Err}
		}
		return processLineMsg{idx: idx, line: line}
	}
}

// startProcessCmd starts a shell command under a PTY and returns the first
// lineMsg / doneMsg. The receiver re-schedules readLineCmd to continue reading.
func startProcessCmd(idx int, command string) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		h, err := Start(ctx, command)
		if err != nil {
			return processDoneMsg{idx: idx, exitCode: -1, err: err}
		}
		// Wrap in a starter message so Update can stash the handle and schedule reads.
		return processStartedMsg{idx: idx, handle: h}
	}
}

type processStartedMsg struct {
	idx    int
	handle *ProcessHandle
}

// verifyAuthCmd runs the tech's verify_cmd after a successful auth.
// The AuthTimeout is applied via context so a hung verify doesn't stall the wizard.
func verifyAuthCmd(idx int, e registry.Entry) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		id, ok := VerifyAuth(ctx, e)
		return verifyDoneMsg{idx: idx, ok: ok, identity: id}
	}
}

// ── Update ───────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {

	case checkDoneMsg:
		return m.onCheckDone(msg)

	case processStartedMsg:
		m.proc = msg.handle
		return m, readLineCmd(msg.idx, m.proc)

	case processLineMsg:
		if msg.idx == m.cur && m.cur < len(m.items) {
			m.items[m.cur].output = appendCapped(m.items[m.cur].output, msg.line, maxOutputLines)
		}
		return m, readLineCmd(msg.idx, m.proc)

	case processDoneMsg:
		return m.onProcessDone(msg)

	case verifyDoneMsg:
		return m.onVerifyDone(msg)

	case tea.KeyMsg:
		return m.onKey(msg)
	}
	return m, nil
}

// onCheckDone handles the CLI-presence probe result. Missing → prompt install.
// Present → jump to the next state (account or auth if required, else done).
func (m Model) onCheckDone(msg checkDoneMsg) (Model, tea.Cmd) {
	if msg.idx != m.cur {
		return m, nil
	}
	it := &m.items[m.cur]
	it.version = msg.version
	if msg.ok {
		return m.advanceAfterInstall()
	}
	it.state = stInstallPrompt
	return m, nil
}

// advanceAfterInstall decides the next state based on whether account / auth is required.
func (m Model) advanceAfterInstall() (Model, tea.Cmd) {
	it := &m.items[m.cur]
	if it.entry.Account != nil && it.entry.Account.Required {
		it.state = stAccountPrompt
		return m, nil
	}
	return m.advanceAfterAccount()
}

func (m Model) advanceAfterAccount() (Model, tea.Cmd) {
	it := &m.items[m.cur]
	if it.entry.Auth != nil && it.entry.Auth.Required {
		it.state = stAuthPrompt
		return m, nil
	}
	return m.markDoneAndAdvance()
}

// markDoneAndAdvance finishes the current tech and starts the next one.
func (m Model) markDoneAndAdvance() (Model, tea.Cmd) {
	m.items[m.cur].state = stDone
	return m.startNext()
}

// startNext bumps m.cur past any skipped/removed slots and kicks off its check.
func (m Model) startNext() (Model, tea.Cmd) {
	for m.cur+1 < len(m.items) {
		m.cur++
		if m.items[m.cur].state == stPending {
			return m, startCheckCmd(m.cur, m.items[m.cur].entry)
		}
	}
	// All done — emit a PhaseChangeMsg to move on to scaffold.
	return m, func() tea.Msg { return tui.PhaseChangeMsg{To: tui.PhaseScaffold} }
}

// onProcessDone handles the exit of an install or auth command.
func (m Model) onProcessDone(msg processDoneMsg) (Model, tea.Cmd) {
	if msg.idx != m.cur {
		return m, nil
	}
	m.proc = nil
	it := &m.items[m.cur]
	switch it.state {
	case stInstalling:
		if msg.exitCode != 0 {
			it.state = stInstallFailed
			if msg.err != nil {
				it.errMsg = msg.err.Error()
			} else {
				it.errMsg = fmt.Sprintf("install exited with code %d", msg.exitCode)
			}
			return m, nil
		}
		// Re-check PATH so the binary we just installed is picked up.
		return m, startCheckCmd(m.cur, it.entry)

	case stAuthRunning:
		if msg.exitCode != 0 {
			it.errMsg = fmt.Sprintf("auth command exited with code %d", msg.exitCode)
			if msg.err != nil {
				it.errMsg = msg.err.Error()
			}
			// Treat non-zero auth exit as auth failure — user can skip.
			it.state = stAuthPrompt
			return m, nil
		}
		it.state = stVerifying
		return m, verifyAuthCmd(m.cur, it.entry)
	}
	return m, nil
}

// onVerifyDone handles the verify-cmd result.
func (m Model) onVerifyDone(msg verifyDoneMsg) (Model, tea.Cmd) {
	if msg.idx != m.cur {
		return m, nil
	}
	it := &m.items[m.cur]
	if !msg.ok {
		it.errMsg = "auth verify failed — try again or skip"
		it.state = stAuthPrompt
		return m, nil
	}
	it.identity = msg.identity
	return m.markDoneAndAdvance()
}

// onKey dispatches keystrokes to whatever action is valid in the current state.
func (m Model) onKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.cur >= len(m.items) {
		return m, nil
	}
	it := &m.items[m.cur]
	key := msg.String()

	switch it.state {
	case stInstallPrompt:
		switch key {
		case "y", "enter":
			cmd := PickInstallCommand(it.entry)
			if cmd == "" {
				it.errMsg = fmt.Sprintf("no install command for %s", runtime.GOOS)
				it.state = stInstallFailed
				return m, nil
			}
			it.state = stInstalling
			it.output = nil
			it.errMsg = ""
			return m, startProcessCmd(m.cur, cmd)
		case "s":
			it.state = stSkipped
			return m.startNext()
		case "r":
			it.state = stRemoved
			return m.startNext()
		}
	case stInstallFailed:
		switch key {
		case "y", "enter":
			it.state = stInstallPrompt
			return m, nil
		case "s":
			it.state = stSkipped
			return m.startNext()
		case "r":
			it.state = stRemoved
			return m.startNext()
		}
	case stAccountPrompt:
		switch key {
		case "o":
			if it.entry.Account != nil {
				OpenURL(it.entry.Account.SignupURL)
			}
			return m, nil
		case "enter":
			return m.advanceAfterAccount()
		case "s":
			it.state = stSkipped
			return m.startNext()
		}
	case stAuthPrompt:
		switch key {
		case "y", "enter":
			if it.entry.Auth == nil || it.entry.Auth.Cmd == "" {
				return m.markDoneAndAdvance()
			}
			it.state = stAuthRunning
			it.output = nil
			it.errMsg = ""
			return m, startProcessCmd(m.cur, it.entry.Auth.Cmd)
		case "s":
			it.state = stSkipped
			return m.startNext()
		}
	}
	return m, nil
}

// ── View ─────────────────────────────────────────────────────────────────

func (m Model) View() string {
	if len(m.items) == 0 {
		return theme.Dim.Render("No technologies in the stack. Go back and pick some.")
	}
	var b strings.Builder
	b.WriteString(theme.Accent.Render("SETUP"))
	b.WriteString("   ")
	b.WriteString(m.progressBar())
	b.WriteString("\n\n")

	for i, it := range m.items {
		b.WriteString(m.renderItemRow(i, it))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", max(20, min(m.width, 80))))
	b.WriteString("\n")
	b.WriteString(m.renderDetail())
	return b.String()
}

func (m Model) progressBar() string {
	done := 0
	for _, it := range m.items {
		if it.state == stDone || it.state == stSkipped {
			done++
		}
	}
	return theme.Dim.Render(fmt.Sprintf("%d / %d complete", done, len(m.items)))
}

func (m Model) renderItemRow(i int, it techProgress) string {
	mark, status := " ", ""
	switch it.state {
	case stDone:
		mark = theme.Good.Render("✓")
		status = "installed · " + it.version
		if it.identity != "" {
			status += "  ·  " + it.identity
		}
	case stSkipped:
		mark = theme.Dim.Render("–")
		status = "skipped"
	case stRemoved:
		mark = theme.Dim.Render("×")
		status = "removed from stack"
	case stPending:
		mark = theme.Dim.Render("·")
		status = "waiting"
	default:
		if i == m.cur {
			mark = theme.Accent.Render("▶")
			status = stateLabel(it.state)
		} else {
			mark = theme.Dim.Render("·")
			status = "waiting"
		}
	}
	return fmt.Sprintf("  %s  %-18s  %s",
		mark,
		truncate(it.entry.Name, 18),
		theme.Dim.Render(status),
	)
}

// renderDetail is the lower "active step" pane — prompt + output for the current tech.
func (m Model) renderDetail() string {
	if m.cur >= len(m.items) {
		return theme.Good.Render("All done. Advancing to scaffold.")
	}
	it := m.items[m.cur]
	var b strings.Builder

	b.WriteString(theme.Accent.Render("▶ " + it.entry.Name))
	b.WriteString("  ")
	b.WriteString(theme.Dim.Render("·  " + stateLabel(it.state)))
	b.WriteString("\n\n")

	switch it.state {
	case stChecking:
		b.WriteString(theme.Dim.Render("Probing PATH…"))
	case stInstallPrompt:
		cmd := PickInstallCommand(it.entry)
		if it.entry.CLI != nil {
			b.WriteString(fmt.Sprintf("%s not found on PATH.\n\n", it.entry.CLI.Binary))
		}
		b.WriteString("Install command:\n  ")
		b.WriteString(theme.Accent.Render(cmd))
		b.WriteString("\n\n")
		b.WriteString(actionRow("y Install", "s Skip", "r Remove from stack"))
	case stInstalling:
		b.WriteString(theme.Dim.Render("Running install…"))
		b.WriteString("\n\n")
		b.WriteString(renderOutput(it.output))
	case stInstallFailed:
		b.WriteString(theme.Accent.Render("install failed"))
		if it.errMsg != "" {
			b.WriteString("  ")
			b.WriteString(theme.Dim.Render(it.errMsg))
		}
		b.WriteString("\n\n")
		b.WriteString(renderOutput(it.output))
		b.WriteString("\n")
		b.WriteString(actionRow("y Retry", "s Skip", "r Remove from stack"))
	case stAccountPrompt:
		if it.entry.Account != nil && it.entry.Account.Note != "" {
			b.WriteString(it.entry.Account.Note)
			b.WriteString("\n\n")
		}
		if it.entry.Account != nil && it.entry.Account.SignupURL != "" {
			b.WriteString("Signup: ")
			b.WriteString(theme.Accent.Render(it.entry.Account.SignupURL))
			b.WriteString("\n\n")
		}
		b.WriteString(actionRow("o Open signup", "enter I already have an account", "s Skip"))
	case stAuthPrompt:
		if it.entry.Auth != nil {
			if it.entry.Auth.Note != "" {
				b.WriteString(it.entry.Auth.Note)
				b.WriteString("\n\n")
			}
			b.WriteString("Auth command:\n  ")
			b.WriteString(theme.Accent.Render(it.entry.Auth.Cmd))
			b.WriteString("\n\n")
		}
		if it.errMsg != "" {
			b.WriteString(theme.Accent.Render(it.errMsg))
			b.WriteString("\n\n")
		}
		b.WriteString(actionRow("y Run auth", "s Skip"))
	case stAuthRunning:
		b.WriteString(theme.Dim.Render("Running auth — may open a browser…"))
		b.WriteString("\n\n")
		b.WriteString(renderOutput(it.output))
	case stVerifying:
		b.WriteString(theme.Dim.Render("Verifying auth…"))
	}
	return b.String()
}

func stateLabel(s techState) string {
	switch s {
	case stChecking:
		return "checking"
	case stInstallPrompt:
		return "install needed"
	case stInstalling:
		return "installing"
	case stInstallFailed:
		return "install failed"
	case stAccountPrompt:
		return "account required"
	case stAuthPrompt:
		return "auth needed"
	case stAuthRunning:
		return "authenticating"
	case stVerifying:
		return "verifying"
	case stDone:
		return "done"
	}
	return "pending"
}

func renderOutput(lines []string) string {
	start := 0
	if len(lines) > 8 {
		start = len(lines) - 8
	}
	var b strings.Builder
	for _, ln := range lines[start:] {
		b.WriteString(theme.Dim.Render("▸ "))
		b.WriteString(truncate(ln, 120))
		b.WriteString("\n")
	}
	return b.String()
}

func actionRow(actions ...string) string {
	parts := make([]string, 0, len(actions))
	for _, a := range actions {
		parts = append(parts, theme.Accent.Render("["+a+"]"))
	}
	return strings.Join(parts, "   ")
}

// ── Small helpers ─────────────────────────────────────────────────────────

func appendCapped(list []string, s string, cap int) []string {
	list = append(list, s)
	if len(list) > cap {
		list = list[len(list)-cap:]
	}
	return list
}

func truncate(s string, maxLen int) string {
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
