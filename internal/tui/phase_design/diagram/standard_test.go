package diagram

import (
	"strings"
	"testing"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

func TestRenderStandardSmoke(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("my-saas").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi").
		WithSelection(tui.LayerDatabase, "postgres-16").
		WithSelection(tui.LayerCache, "upstash-redis").
		WithSelection(tui.LayerInfra, "flyio")

	layout := Compute(stack, reg, 80, 40)
	if len(layout.Nodes) != 5 {
		t.Fatalf("expected 5 nodes, got %d", len(layout.Nodes))
	}
	if len(layout.Connections) == 0 {
		t.Fatal("expected at least one connection between compatible peers")
	}

	out := RenderStandard(layout, 0)
	if out == "" {
		t.Fatal("RenderStandard returned empty string")
	}

	// Each node title should appear in the output.
	for _, want := range []string{"Next.js 14", "Go + chi", "PostgreSQL 16", "Upstash Redis", "Fly.io"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected output to contain %q", want)
		}
	}
	// At least one bezier glyph should be present.
	if !strings.ContainsAny(out, "│─╲╱") {
		t.Error("expected connection glyphs")
	}
}

// TestDumpVisual renders a representative layout to stdout so the author
// can eyeball the output from `go test -v`. Strips ANSI so the shape is
// readable even in plain logs.
func TestDumpVisual(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("my-saas").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi").
		WithSelection(tui.LayerAuth, "clerk").
		WithSelection(tui.LayerDatabase, "postgres-16").
		WithSelection(tui.LayerCache, "upstash-redis").
		WithSelection(tui.LayerInfra, "flyio")

	layout := Compute(stack, reg, 80, 28)
	out := RenderStandard(layout, 5)
	t.Logf("\n%s", stripANSI(out))
	t.Logf("nodes=%d connections=%d", len(layout.Nodes), len(layout.Connections))
}

// stripANSI removes SGR escape sequences so test logs stay legible in
// environments without color support.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func TestRenderEmptyLayout(t *testing.T) {
	out := RenderStandard(Layout{Width: 80, Height: 20}, 0)
	// Width/height set but no nodes — should produce a space-filled canvas, no panic.
	if out == "" {
		t.Fatal("expected non-empty canvas")
	}
}
