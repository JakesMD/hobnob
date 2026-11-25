//go:build linux || darwin

package runner

import (
	"os"
	osExec "os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

// stdinTerm is a step's stdin when hobnob's own stdin is a terminal. A
// command in a process group of its own would be stopped by SIGTTIN reading
// that terminal, and handing it the terminal's foreground would take CTRL+C
// away from hobnob. So the command gets a PTY as stdin and controlling
// terminal, hobnob keeps the real terminal's foreground, puts it in a mode
// that passes every key through (output processing stays on, so hobnob's own
// lines still end properly) and forwards the keys to the PTY. A CTRL+C byte is
// not forwarded: it reaches hobnob as SIGINT, whatever the command does with
// SIGINT, so the first one stops and the second forces.
type stdinTerm struct {
	master *os.File
	saved  unix.Termios
	wake   [2]int
	done   chan struct{}
	once   sync.Once
}

var (
	activeStdinMu sync.Mutex
	activeStdin   *stdinTerm
)

// attachStdin gives cmd its stdin: hobnob's own, unless that is a terminal
// hobnob has the foreground of. stop ends the forwarding and restores the
// terminal; run it once the command has exited.
func attachStdin(cmd *osExec.Cmd, pipes *stepPipes) (stop func()) {
	fd := int(os.Stdin.Fd())
	saved, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		cmd.Stdin = os.Stdin
		return func() {}
	}
	if fg, err := unix.IoctlGetInt(fd, unix.TIOCGPGRP); err == nil && fg != syscall.Getpgrp() {
		cmd.Stdin = os.Stdin
		return func() {}
	}

	var master, slave *os.File
	closeSlave := func() {}
	if pipes.master != nil {
		master, slave = pipes.master, pipes.stdout.w
	} else {
		var pipe *outPipe
		if pipe, master, err = newPTYPipe(ptySize{80, 24}); err != nil {
			cmd.Stdin = os.Stdin
			return func() {}
		}
		slave = pipe.w
		closeSlave = func() { slave.Close() }
		go drain(master)
	}
	cmd.Stdin = slave
	cmd.SysProcAttr.Setpgid = false
	cmd.SysProcAttr.Setsid = true
	cmd.SysProcAttr.Setctty = true
	cmd.SysProcAttr.Ctty = 0

	raw := *saved
	raw.Lflag &^= unix.ECHO | unix.ICANON | unix.ISIG | unix.IEXTEN
	raw.Cc[unix.VMIN], raw.Cc[unix.VTIME] = 1, 0
	if err := unix.IoctlSetTermios(fd, setTermios, &raw); err != nil {
		closeSlave()
		cmd.SysProcAttr.Setsid, cmd.SysProcAttr.Setctty = false, false
		cmd.SysProcAttr.Setpgid = true
		cmd.Stdin = os.Stdin
		return func() {}
	}
	t := &stdinTerm{master: master, saved: *saved, done: make(chan struct{})}
	if err := syscall.Pipe(t.wake[:]); err != nil {
		t.restore()
		closeSlave()
		cmd.Stdin = os.Stdin
		return func() {}
	}
	activeStdinMu.Lock()
	activeStdin = t
	activeStdinMu.Unlock()
	go t.forward(fd)
	return func() {
		t.once.Do(func() {
			syscall.Write(t.wake[1], []byte{0})
			<-t.done
			t.restore()
			syscall.Close(t.wake[0])
			syscall.Close(t.wake[1])
			closeSlave()
			activeStdinMu.Lock()
			if activeStdin == t {
				activeStdin = nil
			}
			activeStdinMu.Unlock()
		})
	}
}

// RestoreTerminal puts the terminal back as the step found it, for a caller
// about to exit without letting the run unwind.
func RestoreTerminal() {
	activeStdinMu.Lock()
	t := activeStdin
	activeStdinMu.Unlock()
	if t != nil {
		t.restore()
	}
}

func (t *stdinTerm) restore() {
	_ = unix.IoctlSetTermios(int(os.Stdin.Fd()), setTermios, &t.saved)
}

// forward copies the terminal's keys to the PTY until stop wakes it, turning
// each CTRL+C into a SIGINT for hobnob. It polls so it never holds a read
// that would swallow the next prompt's keys.
func (t *stdinTerm) forward(fd int) {
	defer close(t.done)
	buf := make([]byte, 4096)
	for {
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}, {Fd: int32(t.wake[0]), Events: unix.POLLIN}}
		if _, err := unix.Poll(fds, -1); err != nil {
			if err == syscall.EINTR {
				continue
			}
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		n, err := syscall.Read(fd, buf)
		if err == syscall.EINTR {
			continue
		}
		if err != nil || n <= 0 {
			return
		}
		rest := buf[:n]
		for len(rest) > 0 {
			i := 0
			for i < len(rest) && rest[i] != 3 {
				i++
			}
			if i > 0 {
				t.master.Write(rest[:i])
			}
			if i == len(rest) {
				break
			}
			_ = syscall.Kill(os.Getpid(), syscall.SIGINT)
			rest = rest[i+1:]
		}
	}
}

// drain discards what a stdin-only PTY's master receives, such as the echo
// of typed keys, until it is closed.
func drain(master *os.File) {
	buf := make([]byte, 4096)
	for {
		if _, err := master.Read(buf); err != nil {
			return
		}
	}
}
