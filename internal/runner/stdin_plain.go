//go:build !linux && !darwin

package runner

import (
	"os"
	osExec "os/exec"
)

// attachStdin gives cmd hobnob's stdin as it is; there is no terminal
// hand-over here.
func attachStdin(cmd *osExec.Cmd, pipes *stepPipes) (stop func()) {
	cmd.Stdin = os.Stdin
	return func() {}
}

// RestoreTerminal does nothing here: hobnob never changes the terminal.
func RestoreTerminal() {}
