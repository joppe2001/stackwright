package phase_setup

import (
	"context"
	"testing"
	"time"

	"github.com/joppe2001/stackwright/internal/registry"
)

func TestIsInstalledPresent(t *testing.T) {
	// `ls` is on every Unix-like system. Windows CI would skip this — we're
	// on darwin/linux CI for this project.
	e := registry.Entry{
		Name: "ls",
		Slug: "ls",
		CLI: &registry.CLI{
			Binary:     "ls",
			VersionCmd: "ls --version",
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, ok := IsInstalled(ctx, e)
	if !ok {
		t.Fatal("expected ls to be detected as installed")
	}
}

func TestIsInstalledAbsent(t *testing.T) {
	e := registry.Entry{
		Name: "does-not-exist",
		Slug: "does-not-exist",
		CLI: &registry.CLI{
			Binary:     "definitely-not-a-real-binary-xyz",
			VersionCmd: "definitely-not-a-real-binary-xyz --version",
		},
	}
	ctx := context.Background()
	_, ok := IsInstalled(ctx, e)
	if ok {
		t.Fatal("expected missing binary to be reported absent")
	}
}

func TestIsInstalledNoCLI(t *testing.T) {
	// Entries with no CLI (e.g., Clerk as a dashboard-only service) should
	// report as installed so the wizard skips the install phase for them.
	e := registry.Entry{Name: "Clerk", Slug: "clerk"}
	_, ok := IsInstalled(context.Background(), e)
	if !ok {
		t.Fatal("expected no-CLI entry to be treated as already-installed")
	}
}

func TestStartCommandEcho(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h, err := Start(ctx, "echo hello-stackwright")
	if err != nil {
		// Some sandboxes forbid fork/exec — skip rather than fail so the
		// suite stays portable.
		t.Skipf("Start: %v (fork/exec likely disallowed in this sandbox)", err)
	}
	var got string
	select {
	case line := <-h.Lines:
		got = line
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for echoed line")
	}
	if got != "hello-stackwright" {
		t.Errorf("got %q want %q", got, "hello-stackwright")
	}
	// Drain remaining lines and wait for Done.
	for range h.Lines {
	}
	res := <-h.Done
	if res.ExitCode != 0 {
		t.Errorf("echo exited with %d, err=%v", res.ExitCode, res.Err)
	}
}

func TestSetupOrder(t *testing.T) {
	entries := []registry.Entry{
		{Slug: "a", Category: registry.CategoryFrontend},
		{Slug: "b", Category: registry.CategoryInfra},
		{Slug: "c", Category: registry.CategoryDatabase},
		{Slug: "d", Category: registry.CategoryBackend},
	}
	sorted := SetupOrder(entries)
	wantOrder := []string{"b", "c", "d", "a"}
	for i, got := range sorted {
		if got.Slug != wantOrder[i] {
			t.Errorf("pos %d: got %q want %q", i, got.Slug, wantOrder[i])
		}
	}
}
