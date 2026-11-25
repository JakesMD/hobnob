//go:build windows

package e2e

import "testing"

func TestE2E_RunConPTY_WatchedCommandSeesATTYOnStdoutOnly(t *testing.T) {
	// given a run watched on a terminal, when a command checks its streams,
	// then stdout is a terminal and stderr is not (why: stdout on a
	// pseudo console keeps colour and progress bars while stderr stays
	// separate, as on Unix)

	// Arrange
	screen, _ := screenOf(80, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          [ -t 1 ] && echo "stdout tty" || echo "stdout pipe"
		          [ -t 2 ] && echo "stderr tty" || echo "stderr pipe"
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen})

	// Assert
	res.OK(t)
	res.Lines(t, "stdout tty", "stderr pipe")
}
