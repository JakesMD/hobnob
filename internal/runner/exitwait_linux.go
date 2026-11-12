//go:build linux

package runner

import (
	"syscall"
	"unsafe"
)

const (
	pPID    = 1         // idtype_t P_PID
	wNoWait = 0x1000000 // WNOWAIT
)

// waitExitUnreaped blocks until pid exits but leaves it a zombie, so its PID
// (and process group ID) can't be reused until os/exec's Wait reaps it.
// Reports false if it couldn't wait, and the caller must reap first instead.
func waitExitUnreaped(pid int) bool {
	var siginfo [128]byte
	for {
		_, _, errno := syscall.Syscall6(syscall.SYS_WAITID, pPID, uintptr(pid),
			uintptr(unsafe.Pointer(&siginfo)), syscall.WEXITED|wNoWait, 0, 0)
		if errno != syscall.EINTR {
			return errno == 0
		}
	}
}
