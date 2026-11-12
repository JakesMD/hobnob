//go:build darwin

package runner

import "syscall"

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
