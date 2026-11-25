//go:build linux || darwin

package runner

import (
	"strconv"
	"syscall"
)

// shellExitError is a failed exit read from an unreaped shell, worded as
// os/exec's ExitError would be once Wait reaps it.
type shellExitError struct{ status syscall.WaitStatus }

func exitErrorOf(status syscall.WaitStatus) error {
	if status.Exited() && status.ExitStatus() == 0 {
		return nil
	}
	return shellExitError{status}
}

func (e shellExitError) Error() string {
	if e.status.Signaled() {
		text := "signal: " + e.status.Signal().String()
		if e.status.CoreDump() {
			text += " (core dumped)"
		}
		return text
	}
	return "exit status " + strconv.Itoa(e.status.ExitStatus())
}

func (e shellExitError) ExitCode() int {
	if e.status.Exited() {
		return e.status.ExitStatus()
	}
	return -1
}

// Interrupted reports whether the shell was killed by SIGINT.
func (e shellExitError) Interrupted() bool {
	return e.status.Signaled() && e.status.Signal() == syscall.SIGINT
}
