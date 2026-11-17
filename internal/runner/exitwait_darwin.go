//go:build darwin

package runner

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// waitExitUnreaped blocks until pid exits but leaves it a zombie, so its PID
// (and process group ID) can't be reused until os/exec's Wait reaps it.
// Reports false if it couldn't wait, and the caller must reap first instead.
func waitExitUnreaped(pid int) bool {
	kq, err := syscall.Kqueue()
	if err != nil {
		return false
	}
	defer syscall.Close(kq)
	var change syscall.Kevent_t
	syscall.SetKevent(&change, pid, syscall.EVFILT_PROC, syscall.EV_ADD|syscall.EV_ONESHOT)
	change.Fflags = syscall.NOTE_EXIT
	events := make([]syscall.Kevent_t, 1)
	for {
		n, err := syscall.Kevent(kq, []syscall.Kevent_t{change}, events, nil)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || n != 1 {
			return false
		}
		if events[0].Flags&syscall.EV_ERROR != 0 {
			// ESRCH: it exited before the watch was added. It's our unreaped
			// child, so it's a zombie and still holds its PID.
			return syscall.Errno(events[0].Data) == syscall.ESRCH
		}
		return true
	}
}

// shellStatus reads the wait status of pid, an exited but unreaped child, as
// the error Wait would report for it. Reports false if it can't be read.
func shellStatus(pid int) (error, bool) {
	kinfo, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kinfo.Proc.P_stat != sZomb {
		return nil, false
	}
	return exitErrorOf(syscall.WaitStatus(kinfo.Proc.P_xstat)), true
}

// groupHasOthers reports whether process group pgid has a live member other
// than the exited shell that leads it.
func groupHasOthers(pgid int) bool {
	members, err := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pgid)
	if err != nil {
		return false
	}
	for _, member := range members {
		if int(member.Proc.P_pid) != pgid && member.Proc.P_stat != sZomb {
			return true
		}
	}
	return false
}

const sZomb = 5 // SZOMB in <sys/proc.h>
