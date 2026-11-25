//go:build !windows

package runner

import (
	"errors"
	"os"
	osExec "os/exec"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
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

// signalGroupTerm and signalGroupKill signal a step's process group. Errors
// are ignored: ESRCH just means nothing is left in it.
func signalGroupTerm(pid int) { _ = syscall.Kill(-pid, syscall.SIGTERM) }
func signalGroupKill(pid int) { _ = syscall.Kill(-pid, syscall.SIGKILL) }

// KillRunningStep force-kills the process group of the currently executing
// run: step, if any, and those of the jobs earlier steps left running. It
// exists for a 2nd CTRL+C: the graceful path (cancelFunc, above) already
// SIGTERMs that group, but a step that ignores SIGTERM needs something
// stronger than signaling hobnob's own process group — hobnob's group is a
// different one, since the step was deliberately Setpgid'd out of it.
// Reports whether a step was actually running.
func KillRunningStep() bool {
	killLeftovers()
	runningStepMu.Lock()
	defer runningStepMu.Unlock()
	pid := runningStepPID
	if pid == 0 {
		return false
	}
	signalGroupKill(pid)
	return true
}

// foregroundTerminal makes cmd's process group the foreground group of the
// terminal on stdin, which cmd inherits: a command in a background group that
// reads the terminal is stopped by SIGTTIN. It does nothing when stdin is no
// terminal, or hobnob's group isn't its foreground group (hobnob is in the
// background). ok reports whether it took effect; then release must run once
// the command's process has exited, to hand the terminal back to hobnob.
//
// While the command holds the terminal, CTRL+C signals its group and not
// hobnob: see interruptSelf.
func foregroundTerminal(cmd *osExec.Cmd) (release func(), ok bool) {
	foreground, err := unix.IoctlGetInt(int(os.Stdin.Fd()), unix.TIOCGPGRP)
	if err != nil || foreground != syscall.Getpgrp() {
		return func() {}, false
	}
	cmd.SysProcAttr.Foreground = true
	cmd.SysProcAttr.Ctty = int(os.Stdin.Fd())
	return func() {
		// A background group setting the foreground group is stopped by
		// SIGTTOU unless it ignores it.
		signal.Ignore(syscall.SIGTTOU)
		_ = unix.IoctlSetPointerInt(int(os.Stdin.Fd()), unix.TIOCSPGRP, syscall.Getpgrp())
		signal.Reset(syscall.SIGTTOU)
	}, true
}

// diedOfInterrupt reports whether err says the command was killed by SIGINT.
func diedOfInterrupt(err error) bool {
	var interrupted interface{ Interrupted() bool }
	if errors.As(err, &interrupted) {
		return interrupted.Interrupted()
	}
	var exitErr *osExec.ExitError
	if errors.As(err, &exitErr) {
		status, ok := exitErr.Sys().(syscall.WaitStatus)
		return ok && status.Signaled() && status.Signal() == syscall.SIGINT
	}
	return false
}

// interruptSelf delivers the CTRL+C a command held the terminal for to
// hobnob itself, as a shell does for a foreground job killed by SIGINT.
func interruptSelf() { _ = syscall.Kill(os.Getpid(), syscall.SIGINT) }
