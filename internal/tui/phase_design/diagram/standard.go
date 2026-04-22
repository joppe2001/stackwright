// Package diagram renders the live architecture diagram in either standard
// (ANSI/Unicode) or Kitty Graphics mode. The two modes share geometry and
// animation timing; only the pixel/glyph backend differs.
package diagram

// Standard-mode ANSI bezier renderer. Step 6.
