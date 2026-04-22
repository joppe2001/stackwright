package scaffold

import (
	"strings"
	"testing"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

func TestNextStepsFullSaaS(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("my-saas").
		WithSelection(tui.LayerFrontend, "nextjs-14").
		WithSelection(tui.LayerBackend, "go-chi").
		WithSelection(tui.LayerDatabase, "postgres-16").
		WithSelection(tui.LayerCache, "upstash-redis").
		WithSelection(tui.LayerAuth, "clerk").
		WithSelection(tui.LayerPayments, "stripe").
		WithSelection(tui.LayerInfra, "flyio").
		WithSelection(tui.LayerCICD, "github-actions")

	steps := NextSteps("./my-saas", stack, reg)
	cat := strings.Join(flatten(steps), "\n")

	// Must include the basics — cd, install, both dev servers, flyctl, git init.
	must := []string{
		"cd ./my-saas",
		"npm install",
		"cd api && go mod tidy",
		"docker compose up -d db",
		"npm run dev",
		"go run .",
		"flyctl launch",
		"git init",
	}
	for _, want := range must {
		if !strings.Contains(cat, want) {
			t.Errorf("NextSteps missing %q\n---\n%s", want, cat)
		}
	}
}

func TestNextStepsMinimalStack(t *testing.T) {
	reg := registry.Load(registry.LoadOptions{Offline: true}).Bundle
	stack := tui.NewStack().
		WithAppName("front-only").
		WithSelection(tui.LayerFrontend, "nextjs-14")
	steps := NextSteps("./x", stack, reg)

	// Should still have cd + npm install + npm run dev — nothing more.
	seen := map[string]bool{}
	for _, s := range steps {
		seen[s.Command] = true
	}
	if !seen["cd ./x"] || !seen["npm install"] || !seen["npm run dev"] {
		t.Errorf("expected base Node path steps; got commands: %v", seen)
	}
	// Must NOT contain Go / fly / DB paths.
	for _, unwanted := range []string{"go mod tidy", "flyctl launch", "createdb", "docker compose"} {
		for _, s := range steps {
			if strings.Contains(s.Command, unwanted) {
				t.Errorf("unexpected step for minimal stack: %q", s.Command)
			}
		}
	}
}

func flatten(s []NextStep) []string {
	out := make([]string, 0, len(s)*2)
	for _, st := range s {
		out = append(out, st.Command, st.Note)
	}
	return out
}
