//go:build unix

package runner

import "syscall"

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
