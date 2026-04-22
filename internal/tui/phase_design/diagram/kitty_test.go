package diagram

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

func TestRenderPNGSmoke(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("my-saas").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi").
		WithSelection(tui.LayerDatabase, "postgres-16")

	layout := Compute(stack, reg, 80, 28)
	img, pngBytes, err := RenderPNG(layout, 0)
	if err != nil {
		t.Fatalf("RenderPNG: %v", err)
	}
	if img == nil {
		t.Fatal("nil image")
	}
	if len(pngBytes) == 0 {
		t.Fatal("empty png bytes")
	}
	// Decode back to prove it's a valid PNG.
	decoded, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatalf("decode roundtrip: %v", err)
	}
	want := img.Bounds()
	got := decoded.Bounds()
	if want != got {
		t.Fatalf("roundtrip bounds mismatch: got %v want %v", got, want)
	}
}

func TestKittyGFXTransmit(t *testing.T) {
	// Build a tiny PNG so we can test the chunker without rendering a full diagram.
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("x").
		WithSelection(tui.LayerFrontend, "nextjs-14")
	layout := Compute(stack, reg, 40, 14)
	_, pngBytes, err := RenderPNG(layout, 0)
	if err != nil {
		t.Fatalf("RenderPNG: %v", err)
	}
	out := KittyGFXTransmit(pngBytes)
	if !strings.HasPrefix(out, "\x1b_G") {
		t.Error("expected output to start with APC kitty prefix")
	}
	if !strings.HasSuffix(out, "\x1b\\") {
		t.Error("expected output to end with ST terminator")
	}
	// Must contain "m=0" exactly once (final-chunk marker) if there were multiple chunks.
	if strings.Contains(out, "m=1") && strings.Count(out, "m=0") != 1 {
		t.Errorf("expected exactly one m=0 final chunk, got %d", strings.Count(out, "m=0"))
	}
}
