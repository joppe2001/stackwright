package phase_setup

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"

	"github.com/joppe2001/stackwright/internal/browser"
	"github.com/joppe2001/stackwright/internal/registry"
)

// A ProcessHandle is a live command streamed over channels. bubbletea reads
// from Lines and Done via tea.Cmds and renders state accordingly.
//
// The lifecycle is:
//  1. Start(...) forks a shell via PTY so the child sees a "real" terminal
//     (required for auth CLIs that open browsers and display device codes).
//  2. A goroutine scans stdout/stderr line-by-line and sends each line on
//     the Lines channel.
//  3. When the child exits, the goroutine closes Lines and sends the exit
//     code on Done.
//  4. Cancel() kills the whole process group — CLIs that fork helpers (e.g.,
//     brew) are stopped in full.
type ProcessHandle struct {
	Lines chan string
	Done  chan ExitResult

	pty    *ptyHandle
	cancel context.CancelFunc
	once   sync.Once
}

// ExitResult carries the final status of a finished process.
type ExitResult struct {
	ExitCode int
	Err      error
}

// ptyHandle is a thin wrapper so we can cleanly close both the cmd and its
// PTY file from Cancel() without duplicating the logic.
type ptyHandle struct {
	cmd *exec.Cmd
	f   io.ReadWriteCloser
}

// Start spawns a new command under a pseudo-terminal and returns a handle.
// `command` is passed to `sh -c` on Unix and `cmd /C` on Windows.
//
// stdout and stderr are merged (PTY combines them naturally). Line breaks
// are normalized to \n so downstream consumers don't have to care about
// \r\n coming from Windows tools or raw \r from progress-bar CLIs.
//
// Important: we do NOT set SysProcAttr.Setpgid. creack/pty's Start() sets
// Setsid=true and Setctty=true internally to give the child its own session
// and controlling TTY; layering Setpgid on top of that triggered EPERM
// ("operation not permitted") on macOS when the kernel tried to apply
// both. Setsid already creates a fresh process group, so we have a clean
// tree to kill via Cancel without needing Setpgid.
func Start(ctx context.Context, command string) (*ProcessHandle, error) {
	if command == "" {
		return nil, errors.New("empty command")
	}

	shell, shellArg := shellFor(runtime.GOOS)
	subCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(subCtx, shell, shellArg, command)
	// Inherit parent environment (PATH, HOMEBREW_PREFIX, CARGO_HOME, etc.).
	cmd.Env = nil

	ptmx, err := pty.Start(cmd)
	if err != nil {
		cancel()
		return nil, err
	}

	h := &ProcessHandle{
		Lines:  make(chan string, 64),
		Done:   make(chan ExitResult, 1),
		pty:    &ptyHandle{cmd: cmd, f: ptmx},
		cancel: cancel,
	}

	go h.pump(ptmx, cmd)
	return h, nil
}

// Cancel terminates the command.
// Safe to call multiple times — each subsequent call is a no-op.
//
// Since we start the child under pty.Start() with Setsid=true, the child
// leads its own session. SIGKILL on the direct child is enough on macOS/Linux
// for the common case; if a misbehaving CLI double-forks a detached helper,
// that helper may outlive us, but that's a tradeoff vs. the EPERM from
// manual Setpgid. Closing the PTY also triggers SIGHUP on the session.
func (h *ProcessHandle) Cancel() {
	h.once.Do(func() {
		if h.pty != nil {
			if h.pty.f != nil {
				_ = h.pty.f.Close()
			}
			if h.pty.cmd != nil && h.pty.cmd.Process != nil {
				_ = h.pty.cmd.Process.Kill()
			}
		}
		if h.cancel != nil {
			h.cancel()
		}
	})
}

// pump reads from the PTY and emits lines. On EOF or error, waits for the
// command to exit and reports the exit status on Done.
func (h *ProcessHandle) pump(r io.ReadCloser, cmd *exec.Cmd) {
	defer close(h.Lines)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.ReplaceAll(scanner.Text(), "\r", "")
		// A blocking send is fine: the TUI consumer loop is always live
		// while a process is running.
		h.Lines <- line
	}
	// Wait for command completion even after PTY EOF.
	err := cmd.Wait()
	exitCode := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		} else {
			// Non-ExitError means the command couldn't start or was signalled.
			// Report the raw error so the state machine can show a sensible message.
			h.Done <- ExitResult{ExitCode: -1, Err: err}
			return
		}
	}
	h.Done <- ExitResult{ExitCode: exitCode}
}

// shellFor returns (shell, flag) for the current OS — we centralize the
// branching here so the rest of the code can stay OS-agnostic.
func shellFor(goos string) (string, string) {
	if goos == "windows" {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// PickInstallCommand selects the platform-specific install line from a
// registry entry. Returns "" if the entry has no CLI or no matching install.
func PickInstallCommand(e registry.Entry) string {
	if e.CLI == nil || e.CLI.Install == nil {
		return ""
	}
	switch runtime.GOOS {
	case "darwin":
		return e.CLI.Install.Macos
	case "linux":
		return e.CLI.Install.Linux
	case "windows":
		return e.CLI.Install.Windows
	}
	return ""
}

// IsInstalled returns (version, true) if the binary is on PATH and its
// version command exits 0. The version string is whatever the first line
// of the command's output produced — displayed verbatim to the user.
func IsInstalled(ctx context.Context, e registry.Entry) (string, bool) {
	if e.CLI == nil || e.CLI.Binary == "" {
		// No CLI => treat as "already set up" from a presence standpoint.
		return "", true
	}
	// Cheap presence check first: does the binary even exist on PATH?
	if _, err := exec.LookPath(e.CLI.Binary); err != nil {
		return "", false
	}
	// Then run the version command to confirm it works.
	version := ""
	if e.CLI.VersionCmd != "" {
		shell, arg := shellFor(runtime.GOOS)
		c := exec.CommandContext(ctx, shell, arg, e.CLI.VersionCmd)
		out, err := c.CombinedOutput()
		if err == nil {
			// First non-empty line is the version.
			for _, ln := range strings.Split(string(out), "\n") {
				s := strings.TrimSpace(ln)
				if s != "" {
					version = s
					break
				}
			}
		}
	}
	return version, true
}

// VerifyAuth runs the auth.verify_cmd and returns (identity, ok).
// ok=true when: exit 0 AND (no success pattern set OR pattern is present
// in the combined output). identity is a best-effort email/handle extracted
// from the output — it's only a display hint, not authoritative.
func VerifyAuth(ctx context.Context, e registry.Entry) (string, bool) {
	if e.Auth == nil || e.Auth.VerifyCmd == "" {
		return "", false
	}
	shell, arg := shellFor(runtime.GOOS)
	c := exec.CommandContext(ctx, shell, arg, e.Auth.VerifyCmd)
	out, err := c.CombinedOutput()
	if err != nil {
		return "", false
	}
	combined := string(out)
	if e.Auth.VerifySuccessPattern != "" && !strings.Contains(combined, e.Auth.VerifySuccessPattern) {
		return "", false
	}
	return extractIdentity(combined), true
}

// extractIdentity pulls a best-effort email or @handle out of verify_cmd
// output. Totally cosmetic — just a nice touch on the "authenticated as …" line.
func extractIdentity(s string) string {
	// Look for an email-looking token first.
	for _, word := range strings.Fields(s) {
		if strings.Contains(word, "@") && strings.Contains(word, ".") {
			return strings.Trim(word, ".,;:()[]{}\"'")
		}
	}
	// Fall back to the first non-empty line.
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			return ln
		}
	}
	return ""
}

// OpenURL asks the OS to open the given URL in the user's default browser.
// Returns the error so callers can display "couldn't open — copy this URL"
// instead of silently pretending. Platform strategy (macOS/Windows/Linux/WSL/
// $BROWSER) lives in internal/browser.
func OpenURL(url string) error {
	return browser.Open(url)
}

// SetupOrder is the dependency-ordered tech list the wizard walks in sequence.
// Lower-level things first (infra, db) so subsequent tools can assume they're
// already present/authenticated.
func SetupOrder(entries []registry.Entry) []registry.Entry {
	priority := map[registry.Category]int{
		registry.CategoryInfra:    0,
		registry.CategoryDatabase: 1,
		registry.CategoryCache:    2,
		registry.CategoryAuth:     3,
		registry.CategoryPayments: 4,
		registry.CategoryBackend:  5,
		registry.CategoryFrontend: 6,
		registry.CategoryCICD:     7,
		registry.CategoryService:  8,
	}
	out := make([]registry.Entry, len(entries))
	copy(out, entries)
	// Stable insertion sort — keeps original order between entries of the
	// same category and avoids pulling in sort for tiny slices.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && priority[out[j].Category] < priority[out[j-1].Category]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// AuthTimeout is the hard ceiling on a single auth command (spec: 5 min).
const AuthTimeout = 5 * time.Minute

// urlRegex matches http(s) URLs in streaming CLI output. The negated class
// excludes typical terminators (quotes, brackets, whitespace) so we don't
// grab trailing punctuation.
var urlRegex = regexp.MustCompile(`https?://[^\s"'<>()\[\]{}|\\^` + "`" + `]+`)

// ExtractURLs returns every http(s) URL found in s, de-duplicated while
// preserving first-seen order. Used by the setup wizard to auto-open the
// URL an auth CLI prints (most auth flows).
func ExtractURLs(s string) []string {
	matches := urlRegex.FindAllString(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		// Strip trailing punctuation that's not part of the URL
		// (periods/commas at end of sentence, for example).
		m = strings.TrimRight(m, ".,;:!?")
		if seen[m] {
			continue
		}
		seen[m] = true
		out = append(out, m)
	}
	return out
}
