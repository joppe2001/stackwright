// Package detect probes the host terminal for rendering capabilities.
//
// Three capabilities drive stackwright's rendering decisions:
//
//   - TrueColor: whether the terminal advertises 24-bit color. Required for the
//     diagram's tech-color palette; without it, we fall back to 256-color.
//   - Unicode: whether box-drawing and half-block glyphs render at one cell wide.
//     We verify this by writing one glyph and measuring cursor advancement.
//   - KittyGraphics: whether the terminal speaks the Kitty Graphics Protocol.
//     Required for visual mode (pixel-rendered logos and the live diagram).
//
// Detection runs synchronously before the TUI takes over the terminal. All
// probes put stdin briefly into raw mode, issue an escape sequence, and read
// the reply with a tight timeout so a non-responsive terminal never hangs the
// program — we simply conclude the capability is absent.
package detect

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// Capabilities is the result of probing the host terminal.
//
// Interactive records whether stdin was a TTY at probe time. When false, the
// Unicode and KittyGraphics probes can't run and those fields fall back to
// conservative defaults (Unicode=true, KittyGraphics=false). The report uses
// Interactive to be honest about which results came from a live probe vs a
// default guess.
type Capabilities struct {
	TrueColor     bool
	KittyGraphics bool
	KittyProbeMs  int64
	Unicode       bool
	Interactive   bool
	TermProgram   string
	TermVersion   string
}

// VisualMode reports whether visual (Kitty GFX) rendering is available.
// Honors the --no-kitty flag passed by the caller.
//
// Note: visual mode is currently OFF by default even when Kitty Graphics is
// available, because mixing Kitty GFX with bubbletea's cell-grid layout leads
// to image-positioning artifacts on resize. Users who want visual mode can
// explicitly opt in with --kitty (set opt-in via a separate flag; see cmd/root.go).
func (c Capabilities) VisualMode(forceOff bool) bool {
	return !forceOff && c.KittyGraphics
}

// Probe runs all detection checks synchronously and returns the result.
// Safe to call before the TUI starts; each probe restores the terminal
// to its previous mode regardless of outcome.
func Probe() Capabilities {
	prog, ver := detectTermProgram()
	interactive := isTTY(os.Stdin) && isTTY(os.Stdout)
	c := Capabilities{
		TrueColor:   detectTrueColor(),
		Interactive: interactive,
		TermProgram: prog,
		TermVersion: ver,
	}
	if !interactive {
		// Can't probe: assume Unicode (modern default), don't claim Kitty.
		c.Unicode = true
		return c
	}
	c.Unicode = detectUnicode()
	kittyOK, elapsed := detectKittyGraphics()
	c.KittyGraphics = kittyOK
	c.KittyProbeMs = elapsed.Milliseconds()
	return c
}

// detectTrueColor checks COLORTERM. Per the spec, "truecolor" or "24bit" enables it.
// This is the de-facto signal every modern terminal sets.
func detectTrueColor() bool {
	v := strings.ToLower(os.Getenv("COLORTERM"))
	return v == "truecolor" || v == "24bit"
}

// detectTermProgram identifies the host terminal via well-known env vars.
// Priority order: Ghostty, Kitty, then the generic TERM_PROGRAM.
// Returns (program, version) where version may be empty if unknown.
func detectTermProgram() (string, string) {
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" {
		return "ghostty", os.Getenv("GHOSTTY_VERSION")
	}
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return "kitty", os.Getenv("KITTY_VERSION")
	}
	if p := os.Getenv("TERM_PROGRAM"); p != "" {
		return strings.ToLower(p), os.Getenv("TERM_PROGRAM_VERSION")
	}
	return "unknown", ""
}

// detectUnicode writes a half-block glyph and measures how far the cursor
// advanced. A terminal that renders ▀ as a single cell advances by 1 column;
// one that renders it as "?" or two columns does not. We read the cursor
// position via the ESC[6n DSR reply and compute the delta.
//
// Falls back to true if the probe can't run (e.g., stdin is not a TTY) —
// modern terminals overwhelmingly support Unicode, so guessing "yes" is safer
// than refusing to draw box-drawing characters to a real TTY.
func detectUnicode() bool {
	if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		return true
	}
	restore, err := enterRaw(os.Stdin)
	if err != nil {
		return true
	}
	defer restore()

	before, err := cursorPosition(150 * time.Millisecond)
	if err != nil {
		return true
	}
	// Write the test glyph followed immediately by a position query.
	if _, err := os.Stdout.WriteString("▀"); err != nil {
		return true
	}
	after, err := cursorPosition(150 * time.Millisecond)
	// Erase the test glyph so it doesn't pollute the screen before the TUI starts.
	// \b backspace then space then \b again: works whether the glyph advanced 1 or 2 columns.
	_, _ = os.Stdout.WriteString("\b \b\b \b")
	if err != nil {
		return true
	}
	delta := after.col - before.col
	return delta == 1
}

// detectKittyGraphics sends a Kitty GFX query (a=q) and looks for a reply
// that begins with the APC Kitty prefix (\x1b_G). A real Kitty implementation
// answers either \x1b_Gi=31;OK\x1b\\ or an error response in the same format —
// either is proof of support. Terminals that don't speak the protocol simply
// don't reply, and the 50ms read window expires.
//
// Returns (supported, latency).
func detectKittyGraphics() (bool, time.Duration) {
	if !isTTY(os.Stdin) || !isTTY(os.Stdout) {
		return false, 0
	}
	restore, err := enterRaw(os.Stdin)
	if err != nil {
		return false, 0
	}
	defer restore()

	// a=q (query), i=31 (image id), s=1,v=1 (1x1 dimensions), t=d (direct), f=24 (RGB).
	// No payload: we only care whether the terminal recognizes the APC and answers.
	start := time.Now()
	if _, err := os.Stdout.WriteString("\x1b_Ga=q,i=31,s=1,v=1,t=d,f=24;\x1b\\"); err != nil {
		return false, 0
	}
	reply, err := readUntilST(50 * time.Millisecond)
	elapsed := time.Since(start)
	if err != nil || len(reply) == 0 {
		return false, elapsed
	}
	return strings.HasPrefix(reply, "\x1b_G"), elapsed
}

// cursorPos is (row, col), 1-indexed per DEC convention.
type cursorPos struct {
	row int
	col int
}

// cursorPosition queries the terminal for the current cursor position using
// the standard DSR sequence (\x1b[6n) and parses the \x1b[<row>;<col>R reply.
//
// Caller must have stdin in raw mode already; otherwise the reply may be
// consumed by the shell's line discipline before we get a chance to read it.
func cursorPosition(timeout time.Duration) (cursorPos, error) {
	if _, err := os.Stdout.WriteString("\x1b[6n"); err != nil {
		return cursorPos{}, err
	}
	reply, err := readUntilByte('R', timeout)
	if err != nil {
		return cursorPos{}, err
	}
	// Expected format: ESC[<row>;<col>R
	i := strings.Index(reply, "\x1b[")
	if i < 0 {
		return cursorPos{}, fmt.Errorf("no CSI in reply")
	}
	body := strings.TrimSuffix(reply[i+2:], "R")
	var row, col int
	if _, err := fmt.Sscanf(body, "%d;%d", &row, &col); err != nil {
		return cursorPos{}, fmt.Errorf("unparsable DSR reply %q: %w", reply, err)
	}
	return cursorPos{row: row, col: col}, nil
}

// readUntilByte reads from stdin until the given sentinel byte or the timeout
// elapses. The sentinel is included in the returned string. Used to capture
// the fixed-terminator 'R' at the end of a DSR cursor-position reply.
func readUntilByte(sentinel byte, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 0, 32)
	one := make([]byte, 1)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return string(buf), fmt.Errorf("timeout")
		}
		if err := os.Stdin.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			// File doesn't support deadlines (unusual for a TTY); fall through and read.
		}
		n, err := os.Stdin.Read(one)
		if n > 0 {
			buf = append(buf, one[0])
			if one[0] == sentinel {
				return string(buf), nil
			}
		}
		if err != nil {
			if err == io.EOF {
				continue
			}
			return string(buf), err
		}
	}
}

// readUntilST reads until the ST terminator (\x1b\\) of an APC sequence or the
// timeout. Used for the Kitty Graphics reply which is framed by ESC_G…ESC\.
func readUntilST(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 0, 64)
	one := make([]byte, 1)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return string(buf), fmt.Errorf("timeout")
		}
		if err := os.Stdin.SetReadDeadline(time.Now().Add(remaining)); err != nil {
			// See readUntilByte: best-effort.
		}
		n, err := os.Stdin.Read(one)
		if n > 0 {
			buf = append(buf, one[0])
			// Look for the ST terminator: \x1b followed by \\ (0x5c).
			if len(buf) >= 2 && buf[len(buf)-2] == 0x1b && buf[len(buf)-1] == 0x5c {
				return string(buf), nil
			}
		}
		if err != nil {
			if err == io.EOF {
				continue
			}
			return string(buf), err
		}
	}
}

// enterRaw puts the file into raw mode and returns a cleanup that restores it.
// On any failure, returns an error and a no-op cleanup (safe to defer unconditionally).
func enterRaw(f *os.File) (func(), error) {
	fd := int(f.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return func() {}, err
	}
	return func() {
		_ = term.Restore(fd, oldState)
		// Clear any read deadline we set during probes.
		_ = f.SetReadDeadline(time.Time{})
	}, nil
}

// isTTY reports whether the file is a terminal. Non-TTY stdin/stdout (piped
// input, CI environments) skip interactive probes and use env-var signals only.
func isTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
