//go:build windows

package runner

import (
	"os"
	osExec "os/exec"
)

// setProcAttr is a no-op on windows: there's no process-group concept to opt
// the step into here.
func setProcAttr(cmd *osExec.Cmd) {}

// cancelFunc falls back to os/exec's default (Process.Kill) since windows has
// no SIGTERM/process-group signaling to do a graceful shutdown with.
func cancelFunc(cmd *osExec.Cmd) func() error {
	return cmd.Process.Kill
}

// signalGroupTerm and signalGroupKill are no-ops on windows: there's no
// process group to signal. settle is never asked to signal here anyway, since
// waitExitUnreaped is unsupported.
func signalGroupTerm(pid int) {}
func signalGroupKill(pid int) {}

// KillRunningStep kills the currently executing run: step's process, if any.
// Reports whether a step was actually running.
func KillRunningStep() bool {
	runningStepMu.Lock()
	defer runningStepMu.Unlock()
	pid := runningStepPID
	if pid == 0 {
		return false
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
	return true
}
