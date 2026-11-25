//go:build unix

package runner

import (
	"context"
	"os"
	osExec "os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

// canDrainNow: the pipes' read ends are non-blocking, so handoff can read
// what the step left buffered without waiting.
const canDrainNow = true

// maxDrain bounds handoff's read, so a job writing nonstop can't hold it.
const maxDrain = 1 << 20

// copyOut copies pipe into its dest until EOF. Each read and its write happen
// under p.mu, so handoff never switches dest between the two.
func (p *stepPipes) copyOut(pipe *outPipe) {
	raw, err := pipe.r.SyscallConn()
	if err != nil {
		return
	}
	buf := make([]byte, 32*1024)
	_ = raw.Read(func(fd uintptr) bool {
		for {
			p.mu.Lock()
			if p.abandoned {
				p.mu.Unlock()
				return true
			}
			n, err := syscall.Read(int(fd), buf)
			if n > 0 {
				pipe.dest.Write(buf[:n])
			}
			p.mu.Unlock()
			switch {
			case err == syscall.EINTR:
			case err == syscall.EAGAIN:
				return false
			case err != nil || n <= 0:
				return true
			}
		}
	})
}

// drainNow delivers what is buffered in pipe to its dest. The caller holds
// p.mu.
func drainNow(pipe *outPipe) {
	raw, err := pipe.r.SyscallConn()
	if err != nil {
		return
	}
	buf := make([]byte, 32*1024)
	_ = raw.Control(func(fd uintptr) {
		for total := 0; total < maxDrain; {
			n, err := syscall.Read(int(fd), buf)
			if err == syscall.EINTR {
				continue
			}
			if err != nil || n <= 0 {
				return
			}
			pipe.dest.Write(buf[:n])
			total += n
		}
	})
}

// newPTYPipe opens a PTY as a step's stdout: the slave is the pipe's write
// end. The master is made non-blocking, as os.Pipe's ends are, so copyOut and
// drainNow never wait on it.
func newPTYPipe(size ptySize) (*outPipe, *os.File, error) {
	master, slave, err := pty.Open()
	if err != nil {
		return nil, nil, err
	}
	fd, err := syscall.Dup(int(master.Fd()))
	master.Close()
	if err == nil {
		syscall.CloseOnExec(fd)
		err = syscall.SetNonblock(fd, true)
	}
	if err != nil {
		slave.Close()
		return nil, nil, err
	}
	nonblocking := os.NewFile(uintptr(fd), "ptmx")
	setPTYSize(nonblocking, size)
	return &outPipe{r: nonblocking, w: slave}, nonblocking, nil
}

func setPTYSize(master *os.File, size ptySize) {
	raw, err := master.SyscallConn()
	if err != nil {
		return
	}
	// Control, not Fd: Fd would put the master back in blocking mode.
	_ = raw.Control(func(fd uintptr) {
		_ = unix.IoctlSetWinsize(int(fd), unix.TIOCSWINSZ, &unix.Winsize{Row: uint16(size.rows), Col: uint16(size.cols)})
	})
}

// startOnPTY: a Unix PTY needs nothing but the slave as stdout, which
// attachPipes did.
func (p *stepPipes) startOnPTY(ctx context.Context, cmd *osExec.Cmd) error { return cmd.Start() }

// closePTY and waitPTY: closing the master is the copy goroutine's own job.
func (p *stepPipes) closePTY()             {}
func (p *stepPipes) waitPTY(time.Duration) {}
