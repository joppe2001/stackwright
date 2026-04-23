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

func TestExtractURLs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "flyctl output",
			in:   "Opening https://fly.io/app/auth-cli/abc123 in your browser",
			want: []string{"https://fly.io/app/auth-cli/abc123"},
		},
		{
			name: "gh auth device flow",
			in:   "First copy your one-time code: ABCD-1234\nThen visit https://github.com/login/device to continue.",
			want: []string{"https://github.com/login/device"},
		},
		{
			name: "url with query",
			in:   "Visit https://example.com/auth?token=abc&state=xyz please",
			want: []string{"https://example.com/auth?token=abc&state=xyz"},
		},
		{
			name: "url in parens stripped",
			in:   "See docs (https://example.com/docs) for help.",
			want: []string{"https://example.com/docs"},
		},
		{
			name: "trailing punctuation removed",
			in:   "See https://example.com/page.",
			want: []string{"https://example.com/page"},
		},
		{
			name: "multiple urls deduped",
			in:   "Go to https://example.com/a or https://example.com/b or https://example.com/a again",
			want: []string{"https://example.com/a", "https://example.com/b"},
		},
		{
			name: "no urls",
			in:   "Your pairing code is: bravo-amply-tough-reward",
			want: nil,
		},
	}
	for _, tc := range cases {
		got := ExtractURLs(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %d urls (%v), want %d (%v)", tc.name, len(got), got, len(tc.want), tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s[%d]: got %q want %q", tc.name, i, got[i], tc.want[i])
			}
		}
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
