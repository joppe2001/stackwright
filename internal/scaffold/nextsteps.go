package scaffold

import (
	"fmt"
	"strings"

	"github.com/joppe2001/stackwright/internal/registry"
	"github.com/joppe2001/stackwright/internal/tui"
)

// NextStep is one line of guidance rendered at the end of the scaffold phase.
// Command is an exact shell incantation the user can copy-paste; Note is a
// human sentence describing what it does.
type NextStep struct {
	Command string
	Note    string
}

// NextSteps computes a recommended sequence of commands to run after
// scaffolding finishes. The list is stack-aware: we only emit commands that
// make sense for the technologies the user selected. Order matches a typical
// first-run path: enter the directory → install deps → set up the DB →
// start the dev server.
//
// This returns lines, not a rendered string, so the TUI can style each
// differently (command in accent, note in muted) and, in theory, also
// export them to a "NEXT_STEPS.md" if ever wanted.
func NextSteps(outDir string, stack tui.Stack, bundle registry.Bundle) []NextStep {
	var steps []NextStep
	add := func(cmd, note string) { steps = append(steps, NextStep{Command: cmd, Note: note}) }

	// Keep a short reference the user can copy even if they scrolled up.
	add(fmt.Sprintf("cd %s", outDir), "Enter the project directory.")

	// Node/Next.js bootstrap when a frontend is selected.
	if stack.Slug(tui.LayerFrontend) != "" {
		add("npm install", "Install Node dependencies for the frontend.")
	}

	// Go backend bootstrap when go-chi is selected.
	if stack.Slug(tui.LayerBackend) == "go-chi" {
		add("(cd api && go mod tidy)", "Resolve Go dependencies for the backend.")
	}

	// Environment sample → real .env.
	if len(envFileContributors(stack, bundle)) > 0 {
		add("cp .env.sample .env && editor .env", "Fill in real values. Sources: " + strings.Join(envFileContributors(stack, bundle), ", "))
	}

	// Database bootstrap paths.
	switch stack.Slug(tui.LayerDatabase) {
	case "postgres-16":
		if stack.Slug(tui.LayerInfra) == "flyio" || stack.Slug(tui.LayerInfra) == "docker" {
			add("docker compose up -d db", "Start the local Postgres container.")
		} else {
			add(fmt.Sprintf("createdb %s", slugifyModule(stack.AppName)),
				"Create a local Postgres database (requires psql from the setup phase).")
		}
	case "neon":
		add("neonctl projects create --name " + slugifyModule(stack.AppName),
			"Create a Neon project and copy the connection string into .env.")
	case "planetscale":
		add("pscale database create " + slugifyModule(stack.AppName),
			"Create a PlanetScale database (requires pscale from the setup phase).")
	case "supabase":
		add("supabase init && supabase start",
			"Initialize the Supabase local stack.")
	}

	// ORM / schema apply.
	if isServiceSelected(stack, "prisma") {
		add("npx prisma migrate dev --name init",
			"Apply the initial Prisma migration.")
	}
	if isServiceSelected(stack, "drizzle") {
		add("npx drizzle-kit generate && npx drizzle-kit migrate",
			"Generate and apply Drizzle migrations.")
	}

	// Dev server choice based on frontend/backend selection.
	switch {
	case stack.Slug(tui.LayerFrontend) == "nextjs-14" && stack.Slug(tui.LayerBackend) == "go-chi":
		add("npm run dev  # in one terminal", "Start the Next.js dev server at http://localhost:3000")
		add("(cd api && go run .)  # in another terminal", "Start the Go API at http://localhost:8080")
	case stack.Slug(tui.LayerFrontend) == "nextjs-14":
		add("npm run dev", "Start the Next.js dev server at http://localhost:3000")
	case stack.Slug(tui.LayerBackend) == "go-chi":
		add("(cd api && go run .)", "Start the Go API at http://localhost:8080")
	}

	// Deployment hints.
	switch stack.Slug(tui.LayerInfra) {
	case "flyio":
		add("flyctl launch --no-deploy", "Link the project to a new Fly app; then `flyctl deploy` when ready.")
	case "vercel":
		add("vercel", "Link to Vercel and deploy a preview.")
	case "railway":
		add("railway link && railway up", "Link to a Railway project and deploy.")
	}

	// CI nudge.
	if stack.Slug(tui.LayerCICD) == "github-actions" {
		add("git init && git add . && git commit -m 'initial scaffold'",
			"Initialize git; push to GitHub to activate the CI workflow.")
	}

	return steps
}

// envFileContributors returns the human names of selected techs that wrote
// something into .env.sample. Used to phrase the "fill in real values" note.
func envFileContributors(stack tui.Stack, bundle registry.Bundle) []string {
	var out []string
	for _, l := range tui.AllLayers {
		slug := stack.Slug(l)
		if slug == "" {
			continue
		}
		if contributesEnv(slug) {
			if e, ok := bundle.BySlug(slug); ok {
				out = append(out, e.Name)
			} else {
				out = append(out, slug)
			}
		}
	}
	return out
}

// contributesEnv hard-codes the short list of slugs whose bundled template
// appends to .env.sample. Cheaper than introspecting template manifests at
// render time and gives us predictable NextSteps wording.
func contributesEnv(slug string) bool {
	switch slug {
	case "postgres-16", "upstash-redis", "clerk", "stripe",
		"supabase", "planetscale", "neon", "resend", "aws-s3":
		return true
	}
	return false
}

// isServiceSelected reports whether the given service slug appears anywhere
// in the selection map. Services can be picked from any sub-list that
// exposes them, but we care about slug identity here.
func isServiceSelected(stack tui.Stack, slug string) bool {
	for _, l := range tui.AllLayers {
		if stack.Slug(l) == slug {
			return true
		}
	}
	return false
}
