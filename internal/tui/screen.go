package tui

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// Screen is the terminal a person watches a run on. A run with no Screen is
// piped, to a file or in CI.
type Screen struct {
	// Size reports the terminal's current width and height in cells.
	Size func() (cols, rows int)

	// OnResize calls fn each time the terminal is resized, until the
	// returned stop is called.
	OnResize func(fn func()) (stop func())
}

// StdoutScreen returns the Screen for os.Stdout, or nil when stdout is not a
// terminal.
func StdoutScreen() *Screen {
	if !term.IsTerminal(os.Stdout.Fd()) {
		return nil
	}
	return &Screen{
		Size: func() (int, int) {
			cols, rows, err := term.GetSize(os.Stdout.Fd())
			if err != nil || cols <= 0 || rows <= 0 {
				return 80, 24
			}
			return cols, rows
		},
		OnResize: onResize,
	}
}
