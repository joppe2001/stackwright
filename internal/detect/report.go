package detect

import (
	"fmt"
	"io"
)

// PrintReport writes the human-readable capability report to w.
// Matches the format documented in the spec.
func PrintReport(w io.Writer, c Capabilities, noKittyFlag bool) {
	fmt.Fprintln(w, "▶  stackwright — terminal capability check")
	fmt.Fprintln(w)

	line(w, c.TrueColor, "24-bit true colour", colorDetail(c))
	line(w, c.Unicode, "Unicode support", unicodeDetail(c))
	line(w, c.KittyGraphics, "Kitty Graphics", kittyDetail(c))
	fmt.Fprintf(w, "     └─ terminal: %s", c.TermProgram)
	if c.TermVersion != "" {
		fmt.Fprintf(w, " %s", c.TermVersion)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	if !c.Interactive {
		fmt.Fprintln(w, "  Render mode will be chosen at TUI launch (probes need an interactive TTY).")
		return
	}

	if c.VisualMode(noKittyFlag) {
		fmt.Fprintln(w, "  Visual mode available.")
		fmt.Fprintln(w, "  Tech logos and the live diagram will render as pixel graphics.")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "  Launching in visual mode. Pass --no-kitty to force standard mode.")
		return
	}

	fmt.Fprintln(w, "  Standard mode will be used (ANSI/Unicode rendering).")
	if c.KittyGraphics && noKittyFlag {
		fmt.Fprintln(w, "  --no-kitty was passed; Kitty Graphics is available but disabled.")
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "  To enable visual mode, try a terminal with Kitty Graphics support:")
	fmt.Fprintln(w, "    brew install kitty            # Kitty (macOS/Linux)")
	fmt.Fprintln(w, "    brew install --cask ghostty   # Ghostty (macOS/Linux)")
}

func line(w io.Writer, ok bool, label, detail string) {
	mark := "✗"
	if ok {
		mark = "✓"
	}
	fmt.Fprintf(w, "  %s  %-22s %s\n", mark, label, detail)
}

func colorDetail(c Capabilities) string {
	if c.TrueColor {
		return "COLORTERM=truecolor"
	}
	return "COLORTERM not set — falling back to 256-color"
}

func unicodeDetail(c Capabilities) string {
	if !c.Interactive {
		return "not probed (non-interactive stdin) — assuming supported"
	}
	if c.Unicode {
		return "half-blocks, box-drawing OK"
	}
	return "glyph advanced > 1 cell — fallback ASCII will be used"
}

func kittyDetail(c Capabilities) string {
	if !c.Interactive {
		return "not probed (non-interactive stdin)"
	}
	if c.KittyGraphics {
		if c.KittyProbeMs <= 0 {
			return "probe responded in <1ms"
		}
		return fmt.Sprintf("probe responded in %dms", c.KittyProbeMs)
	}
	return "no response within 50ms"
}
