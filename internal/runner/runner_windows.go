//go:build windows

package runner

import (
	osExec "os/exec"

	"golang.org/x/sys/windows"
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
// process group to signal. No leftover has a group to signal here anyway,
// since waitExitUnreaped is unsupported.
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
	conPTYs.Range(func(_, value any) bool {
		value.(*conPTY).terminate()
		return true
	})
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid)); err == nil {
		_ = windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
	return true
}
