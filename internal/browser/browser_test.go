package browser

import (
	"runtime"
	"strings"
	"testing"
)

func TestOpenEmptyURL(t *testing.T) {
	if err := Open(""); err == nil {
		t.Error("expected error for empty URL")
	}
	if err := Open("   "); err == nil {
		t.Error("expected error for whitespace-only URL")
	}
}

func TestCandidatesDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only")
	}
	cs := candidates("https://example.com")
	if len(cs) != 1 {
		t.Fatalf("want 1 candidate on darwin, got %d", len(cs))
	}
	if cs[0][0] != "open" {
		t.Errorf("want first candidate 'open', got %v", cs[0])
	}
}

func TestCandidatesLinuxEnv(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	t.Setenv("BROWSER", "/usr/bin/firefox")
	cs := candidates("https://example.com")
	// Must include firefox from $BROWSER and xdg-open fallback.
	found := map[string]bool{}
	for _, c := range cs {
		found[c[0]] = true
	}
	if !found["/usr/bin/firefox"] {
		t.Error("expected $BROWSER to contribute a candidate")
	}
	if !found["xdg-open"] {
		t.Error("expected xdg-open fallback")
	}
}

func TestCandidatesLinuxBrowserWithPercentS(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	t.Setenv("BROWSER", "firefox --new-window %s")
	cs := candidates("https://example.com")
	// The first $BROWSER candidate should have the %s replaced + args preserved.
	for _, c := range cs {
		if c[0] == "firefox" {
			if len(c) < 3 {
				t.Fatalf("expected ['firefox','--new-window','https://example.com'], got %v", c)
			}
			if c[1] != "--new-window" || c[2] != "https://example.com" {
				t.Errorf("args not expanded correctly: %v", c)
			}
			return
		}
	}
	t.Error("firefox candidate not found")
}

func TestCandidatesLinuxBrowserColonSeparated(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	t.Setenv("BROWSER", "xdg-settings:chromium:firefox")
	cs := candidates("https://example.com")
	// Extract just the first-arg names in order.
	var order []string
	for _, c := range cs {
		order = append(order, c[0])
	}
	// The three $BROWSER entries must appear before xdg-open.
	joined := strings.Join(order, ",")
	xdgIdx := strings.Index(joined, "xdg-open")
	for _, want := range []string{"xdg-settings", "chromium", "firefox"} {
		i := strings.Index(joined, want)
		if i < 0 {
			t.Errorf("%q not in candidates: %v", want, order)
			continue
		}
		if xdgIdx >= 0 && i > xdgIdx {
			t.Errorf("%q should appear before xdg-open", want)
		}
	}
}

func TestIsWSLNoFalsePositive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux-only")
	}
	// The test environment isn't WSL. This just confirms isWSL doesn't
	// crash and returns a boolean — correctness depends on the host.
	_ = isWSL()
}
