// Package browser opens URLs in the user's default browser across macOS,
// Windows, Linux (including WSL), and any environment that sets $BROWSER.
//
// Callers always get a concrete error on failure so the TUI can show an
// honest "couldn't open — copy this URL" fallback. Firing and forgetting
// was the previous behavior and it's why users reported silent no-ops.
package browser

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Open spawns the platform-appropriate helper to open url in the user's
// default browser. Returns a non-nil error if no opener succeeded.
//
// The function returns as soon as the opener process is spawned (Start),
// not when the browser window actually appears. That's the right contract
// for a TUI — we only need to know the handoff succeeded.
func Open(url string) error {
	if strings.TrimSpace(url) == "" {
		return errors.New("empty URL")
	}

	cmds := candidates(url)
	if len(cmds) == 0 {
		return fmt.Errorf("no browser opener available for %s", runtime.GOOS)
	}

	var lastErr error
	for _, args := range cmds {
		// Skip candidates whose binary isn't on PATH so we don't waste a
		// failing exec on every probe.
		if _, err := exec.LookPath(args[0]); err != nil {
			lastErr = err
			continue
		}
		cmd := exec.Command(args[0], args[1:]...)
		// Detach stdio so openers that fork-and-exit don't leave file
		// descriptors connected to our TUI.
		cmd.Stdin = nil
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		// The opener started; release it so its process doesn't become a zombie.
		_ = cmd.Process.Release()
		return nil
	}

	if lastErr == nil {
		lastErr = errors.New("no opener succeeded")
	}
	return fmt.Errorf("open %q: %w", url, lastErr)
}

// candidates returns an ordered list of argv candidates to try for the
// current OS. Earlier entries are preferred.
func candidates(url string) [][]string {
	switch runtime.GOOS {
	case "darwin":
		return [][]string{{"open", url}}

	case "windows":
		// `cmd /C start "" <url>` — the empty "" is the window title,
		// required when the URL might contain quotes or spaces.
		return [][]string{{"cmd", "/C", "start", "", url}}

	default: // linux, freebsd, openbsd, etc.
		var out [][]string

		// WSL: delegate to the Windows side when available. wslview is the
		// canonical helper (from wslu); cmd.exe via /mnt/c is the fallback.
		if isWSL() {
			out = append(out, []string{"wslview", url})
			// Running cmd.exe from WSL takes the same "start" pattern.
			out = append(out, []string{"cmd.exe", "/C", "start", "", url})
		}

		// $BROWSER wins when set — users on bespoke setups (WM scripts,
		// remote forwarding helpers) rely on this.
		if b := strings.TrimSpace(os.Getenv("BROWSER")); b != "" {
			// $BROWSER can contain multiple colon-separated commands. Try each.
			for _, part := range strings.Split(b, ":") {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				// Respect %s replacement if the user used it; otherwise append url.
				if strings.Contains(part, "%s") {
					expanded := strings.ReplaceAll(part, "%s", url)
					fields := strings.Fields(expanded)
					if len(fields) > 0 {
						out = append(out, fields)
					}
				} else {
					fields := append(strings.Fields(part), url)
					out = append(out, fields)
				}
			}
		}

		// Standard Linux desktop openers, in priority order.
		out = append(out,
			[]string{"xdg-open", url},
			[]string{"sensible-browser", url},
			[]string{"x-www-browser", url},
			[]string{"gnome-open", url},
			[]string{"kde-open5", url},
			[]string{"kde-open", url},
		)
		return out
	}
}

// isWSL detects Windows Subsystem for Linux by sniffing well-known
// indicators. No false positives against native Linux have been reported
// because /proc/version on WSL contains the literal string "microsoft".
func isWSL() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
		return true
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}
