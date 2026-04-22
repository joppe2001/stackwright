// Package tui hosts the root bubbletea model, the phase state machine, and
// small shared types (Stack, messages) used by every phase subpackage.
//
// Phase-specific models live in internal/tui/phase_design, phase_setup, and
// phase_scaffold. They import internal/tui only for the shared types defined
// here and internal/tui/theme for styles.
package tui

import "github.com/joppe2001/stackwright/internal/registry"

// Layer identifies one slot in the user's stack. Matches the spec's left-pane
// list: App type, Frontend, Backend, Database, Cache, Auth, Payments, Infra, CI/CD.
//
// App type is special — it carries the project name rather than a registry slug.
type Layer string

const (
	LayerAppType  Layer = "app-type"
	LayerFrontend Layer = "frontend"
	LayerBackend  Layer = "backend"
	LayerDatabase Layer = "database"
	LayerCache    Layer = "cache"
	LayerAuth     Layer = "auth"
	LayerPayments Layer = "payments"
	LayerInfra    Layer = "infra"
	LayerCICD     Layer = "cicd"
)

// AllLayers is the display order for the left pane.
var AllLayers = []Layer{
	LayerAppType,
	LayerFrontend,
	LayerBackend,
	LayerDatabase,
	LayerCache,
	LayerAuth,
	LayerPayments,
	LayerInfra,
	LayerCICD,
}

// LayerTitle is the human-readable label for a layer.
func LayerTitle(l Layer) string {
	switch l {
	case LayerAppType:
		return "App type"
	case LayerFrontend:
		return "Frontend"
	case LayerBackend:
		return "Backend"
	case LayerDatabase:
		return "Database"
	case LayerCache:
		return "Cache"
	case LayerAuth:
		return "Auth"
	case LayerPayments:
		return "Payments"
	case LayerInfra:
		return "Infra"
	case LayerCICD:
		return "CI/CD"
	}
	return string(l)
}

// LayerCategory returns the registry category a layer maps to.
// App type has no registry category (it's just a name input).
func LayerCategory(l Layer) (registry.Category, bool) {
	switch l {
	case LayerFrontend:
		return registry.CategoryFrontend, true
	case LayerBackend:
		return registry.CategoryBackend, true
	case LayerDatabase:
		return registry.CategoryDatabase, true
	case LayerCache:
		return registry.CategoryCache, true
	case LayerAuth:
		return registry.CategoryAuth, true
	case LayerPayments:
		return registry.CategoryPayments, true
	case LayerInfra:
		return registry.CategoryInfra, true
	case LayerCICD:
		return registry.CategoryCICD, true
	}
	return "", false
}

// Stack is the user's current composition. Immutable-flavored updates
// (With… methods return a new value) so consumers can compare old/new snapshots.
type Stack struct {
	AppName    string          // user-entered project name (from App type layer)
	Selections map[Layer]string // layer → selected slug
}

// NewStack returns an empty Stack with an initialized map.
func NewStack() Stack {
	return Stack{Selections: map[Layer]string{}}
}

// WithAppName returns a copy with AppName set.
func (s Stack) WithAppName(name string) Stack {
	out := s.clone()
	out.AppName = name
	return out
}

// WithSelection returns a copy with (layer → slug) set. Empty slug clears the slot.
func (s Stack) WithSelection(layer Layer, slug string) Stack {
	out := s.clone()
	if slug == "" {
		delete(out.Selections, layer)
	} else {
		out.Selections[layer] = slug
	}
	return out
}

// Slug returns the selected slug for a layer, or "" if none is set.
func (s Stack) Slug(l Layer) string { return s.Selections[l] }

// IsSet reports whether the layer has a confirmed selection.
// App type counts as set when AppName is non-empty; all other layers require a slug.
func (s Stack) IsSet(l Layer) bool {
	if l == LayerAppType {
		return s.AppName != ""
	}
	return s.Selections[l] != ""
}

// SelectedEntries returns the registry entries corresponding to every slug-set layer,
// in AllLayers order. Unknown slugs are silently skipped — the caller decides
// whether that's an error.
func (s Stack) SelectedEntries(b registry.Bundle) []registry.Entry {
	out := make([]registry.Entry, 0, len(AllLayers))
	for _, l := range AllLayers {
		slug := s.Selections[l]
		if slug == "" {
			continue
		}
		if e, ok := b.BySlug(slug); ok {
			out = append(out, e)
		}
	}
	return out
}

func (s Stack) clone() Stack {
	m := make(map[Layer]string, len(s.Selections))
	for k, v := range s.Selections {
		m[k] = v
	}
	return Stack{AppName: s.AppName, Selections: m}
}

// PhaseChangeMsg is broadcast by a phase model when it wants the root model
// to advance to the next phase (e.g., design phase sends it on 'g').
type PhaseChangeMsg struct {
	To Phase
}

// StackUpdateMsg is how phases report that the user's stack changed.
// Emitted by phase_design's layer navigator after a selection or app-name edit.
type StackUpdateMsg struct {
	Stack Stack
}

// Phase identifies which sub-model is currently active.
type Phase int

const (
	PhaseDesign Phase = iota
	PhaseSetup
	PhaseScaffold
)

func (p Phase) String() string {
	switch p {
	case PhaseDesign:
		return "design"
	case PhaseSetup:
		return "setup"
	case PhaseScaffold:
		return "scaffold"
	}
	return "unknown"
}
