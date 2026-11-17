//go:build linux

package runner

import (
	"os"
	"strconv"
	"strings"
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

// shellStatus reads the wait status of pid, an exited but unreaped child, as
// the error Wait would report for it. Reports false if it can't be read.
func shellStatus(pid int) (error, bool) {
	fields, ok := procStat(strconv.Itoa(pid))
	if !ok || fields[0] != "Z" || len(fields) < 50 {
		return nil, false
	}
	status, err := strconv.Atoi(fields[49]) // exit_code, field 52 of proc_pid_stat(5)
	if err != nil {
		return nil, false
	}
	return exitErrorOf(syscall.WaitStatus(status)), true
}

// groupHasOthers reports whether process group pgid has a live member other
// than the exited shell that leads it.
func groupHasOthers(pgid int) bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	leader := strconv.Itoa(pgid)
	for _, entry := range entries {
		name := entry.Name()
		if name == leader || name[0] < '0' || name[0] > '9' {
			continue
		}
		fields, ok := procStat(name)
		if ok && fields[0] != "Z" && fields[2] == leader {
			return true
		}
	}
	return false
}

// procStat returns /proc/<pid>/stat's fields from state (field 3) on, past
// the parenthesised command name, which may itself contain spaces.
func procStat(pid string) ([]string, bool) {
	data, err := os.ReadFile("/proc/" + pid + "/stat")
	if err != nil {
		return nil, false
	}
	text := string(data)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return nil, false
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 3 {
		return nil, false
	}
	return fields, true
}
