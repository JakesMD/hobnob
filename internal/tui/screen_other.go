//go:build !unix

package tui

import (
	"os"
	"time"

	"github.com/charmbracelet/x/term"
)

// resizePoll is how often onResize looks at the terminal's size: Windows has
// no SIGWINCH, and a console's resize events aren't delivered to a program
// that isn't reading its input.
const resizePoll = 200 * time.Millisecond

func onResize(fn func()) (stop func()) {
	done := make(chan struct{})
	go func() {
		cols, rows, _ := term.GetSize(os.Stdout.Fd())
		tick := time.NewTicker(resizePoll)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				if c, r, err := term.GetSize(os.Stdout.Fd()); err == nil && (c != cols || r != rows) {
					cols, rows = c, r
					fn()
				}
			case <-done:
				return
			}
		}
	}()
	return func() { close(done) }
}
