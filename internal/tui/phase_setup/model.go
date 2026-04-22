// Package phase_setup drives the setup wizard: for every technology in the
// confirmed stack, it checks CLI presence, installs if missing, prompts for an
// account, runs the auth flow, and verifies success before moving on.
//
// Implemented as a per-technology state machine (see the spec's state diagram).
// Wired in Step 9.
package phase_setup
