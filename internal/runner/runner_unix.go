//go:build !windows

package runner

import (
	"errors"
	"os"
	osExec "os/exec"
	"syscall"
)

// setProcAttr puts sh in its own process group so cancelFunc and
// KillRunningStep can signal the whole group, not just sh itself.
//
// Known limitation: this also takes the step out of the terminal's foreground
// process group. A CTRL+Z during a step suspends hobnob (still in the
// original foreground group), not the step, which keeps running orphaned in
// the background. Fixing that properly needs explicit tcsetpgrp handoff
// around the step (save/restore the foreground group, handle SIGTTOU) —
// deliberately not attempted here to avoid destabilizing bubbletea's own
// raw-mode stdin handling.
func setProcAttr(cmd *osExec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// cancelFunc returns cmd.Cancel: SIGTERM the process group on ctx
// cancellation. If the process has already exited by the time this runs
// (fast step finishing the same instant as a CTRL+C), syscall.Kill fails with
// ESRCH. Reporting that as os.ErrProcessDone tells os/exec the process is
// simply gone, so Wait() surfaces the command's real (successful) exit status
// instead of misreporting the step as interrupted and discarding its output.
func cancelFunc(cmd *osExec.Cmd) func() error {
	return func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
}

// KillRunningStep force-kills the process group of the currently executing
// run: step, if any. It exists for a 2nd CTRL+C: the graceful path
// (cancelFunc, above) already SIGTERMs that group, but a step that ignores
// SIGTERM needs something stronger than signaling hobnob's own process
// group — hobnob's group is a different one, since the step was deliberately
// Setpgid'd out of it. Reports whether a step was actually running.
func KillRunningStep() bool {
	runningStepMu.Lock()
	pid := runningStepPID
	runningStepMu.Unlock()
	if pid == 0 {
		return false
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	return true
}
