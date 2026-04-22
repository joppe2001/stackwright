// Package tui is the root of the bubbletea model tree. The root model owns a
// phase state machine (design → setup → scaffold) and delegates rendering and
// updates to the active phase package. Wired up in Step 4.
package tui

// Phase identifies which sub-model is currently active.
type Phase int

const (
	PhaseDesign Phase = iota
	PhaseSetup
	PhaseScaffold
)
