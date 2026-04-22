package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

func TestGenerateEndToEnd(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("smoke-app").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi").
		WithSelection(tui.LayerDatabase, "postgres-16").
		WithSelection(tui.LayerCache, "upstash-redis").
		WithSelection(tui.LayerAuth, "clerk").
		WithSelection(tui.LayerInfra, "flyio")

	dir := t.TempDir()
	out := filepath.Join(dir, "smoke-app")

	ch := Generate(out, reg, stack)
	var total, wrote int
	var errs []string
	for r := range ch {
		if r.Total > 0 && r.Path == "" {
			total = r.Total
			continue
		}
		if r.Err != nil {
			errs = append(errs, r.Path+": "+r.Err.Error())
			continue
		}
		wrote++
	}
	if len(errs) > 0 {
		t.Fatalf("generate errors:\n%s", strings.Join(errs, "\n"))
	}
	if total == 0 {
		t.Fatal("no total reported")
	}
	if wrote != total {
		t.Errorf("wrote %d files, expected %d", wrote, total)
	}

	// Must-have generated files.
	for _, want := range []string{
		"package.json",
		"app/layout.tsx",
		"api/go.mod",
		"api/main.go",
		"Dockerfile",
		"fly.toml",
		"stack.yaml",
		"SETUP.md",
	} {
		p := filepath.Join(out, want)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing generated file: %s (%v)", want, err)
		}
	}

	// Verify the append-strategy .env.sample consolidation: should contain
	// both upstash and clerk + stripe? Clerk is present, stripe isn't selected
	// in this stack so it shouldn't appear.
	envBytes, err := os.ReadFile(filepath.Join(out, ".env.sample"))
	if err != nil {
		t.Fatalf("read .env.sample: %v", err)
	}
	env := string(envBytes)
	for _, want := range []string{"UPSTASH_REDIS_REST_URL", "CLERK_SECRET_KEY"} {
		if !strings.Contains(env, want) {
			t.Errorf(".env.sample missing %s", want)
		}
	}
	if strings.Contains(env, "STRIPE_SECRET_KEY") {
		t.Error(".env.sample should not contain STRIPE_SECRET_KEY (stripe not selected)")
	}

	// stack.yaml must name the app + the infra slug.
	stackBytes, _ := os.ReadFile(filepath.Join(out, "stack.yaml"))
	stackText := string(stackBytes)
	if !strings.Contains(stackText, "smoke-app") {
		t.Error("stack.yaml missing app name")
	}
	if !strings.Contains(stackText, "flyio") {
		t.Error("stack.yaml missing flyio slug")
	}
}

func TestRenderArchitecturePNG(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("png-app").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi")

	dir := t.TempDir()
	out := filepath.Join(dir, "test.png")
	if err := WriteArchitecturePNG(out, reg, stack); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if st.Size() < 100 {
		t.Errorf("png too small (%d bytes) — probably corrupt", st.Size())
	}
}
