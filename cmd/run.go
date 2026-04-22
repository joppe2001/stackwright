package cmd

import "fmt"

// runTUI is the main entrypoint for the interactive flow.
// Wired up in later steps; Step 1 leaves it as a placeholder so the skeleton compiles.
func runTUI(noKitty, offline bool) error {
	fmt.Println("stackwright — TUI not yet wired up.")
	fmt.Printf("  flags: no-kitty=%v offline=%v\n", noKitty, offline)
	fmt.Println("  try: stackwright --detect")
	return nil
}
