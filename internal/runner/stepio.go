package runner

import (
	"context"
	"io"
	"os"
	osExec "os/exec"
	"sync"
	"time"
)

// leftoverGrace is how long a step's leftover processes get, after SIGTERM at
// the end of the run, to exit before they're SIGKILLed. A var only so tests
// can shorten it (see SetLeftoverGrace).
var leftoverGrace = 2 * time.Second

// killDrain is how long output keeps draining after the SIGKILL, so whatever
// the killed processes left in the pipes still shows.
const killDrain = 100 * time.Millisecond

// outPipe is one OS pipe carrying a step's stdout or stderr. dest is guarded
// by the owning stepPipes' mu.
type outPipe struct {
	r, w      *os.File
	closeOnce sync.Once
	dest      io.Writer
}

func newOutPipe() (*outPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &outPipe{r: r, w: w}, nil
}

func (p *outPipe) closeReader() { p.closeOnce.Do(func() { p.r.Close() }) }

func (p *outPipe) close() {
	p.closeReader()
	p.w.Close()
}

// stepPipes gives a run: step real OS pipes for stdout and stderr, copied into
// the step's writers by hobnob's own goroutines.
//
// os/exec does this itself when Stdout/Stderr aren't *os.File, but then Wait
// blocks until the pipes hit EOF, which is never while a backgrounded job
// (`cmd &`) still holds them. Owning the pipes lets execRun wait for the
// step's process alone, then hand the pipes over to the job.
type stepPipes struct {
	stdout, stderr *outPipe
	copied         chan struct{}
	master         *os.File // stdout's PTY master; nil when stdout is a pipe

	mu        sync.Mutex
	abandoned bool // set once the run stops copying; later output is dropped
}

// ptyDrainWait is how long handoff waits for a pseudo console to deliver what
// the step wrote once it is closed.
const ptyDrainWait = time.Second

// ptySize is the size of the PTY a step's stdout goes to.
type ptySize struct{ cols, rows int }

// attachPipes gives cmd its stdout and stderr pipes. With a ptySize, stdout is
// a PTY instead, unless the platform has none.
func attachPipes(cmd *osExec.Cmd, size *ptySize) (*stepPipes, error) {
	var stdout *outPipe
	var master *os.File
	if size != nil {
		if pipe, m, err := newPTYPipe(*size); err == nil {
			stdout, master = pipe, m
		}
	}
	if stdout == nil {
		var err error
		if stdout, err = newOutPipe(); err != nil {
			return nil, err
		}
	}
	stderr, err := newOutPipe()
	if err != nil {
		stdout.close()
		return nil, err
	}
	cmd.Stdout = stdout.w
	cmd.Stderr = stderr.w
	return &stepPipes{stdout: stdout, stderr: stderr, master: master}, nil
}

// start starts cmd. Where the step's PTY needs the process created attached to
// it (ConPTY), that is done here, and ctx cancelling interrupts the process.
func (p *stepPipes) start(ctx context.Context, cmd *osExec.Cmd) error {
	return p.startOnPTY(ctx, cmd)
}

// resize sets the PTY's size; it does nothing when stdout is a pipe.
func (p *stepPipes) resize(size ptySize) {
	if p.master != nil {
		setPTYSize(p.master, size)
	}
}

// startCopying closes hobnob's copies of the write ends (the child holds its
// own, so EOF arrives once every process holding them is gone) and copies the
// read ends into dest until then.
func (p *stepPipes) startCopying(stdoutDest, stderrDest io.Writer) {
	p.stdout.w.Close()
	p.stderr.w.Close()
	p.stdout.dest, p.stderr.dest = stdoutDest, stderrDest
	p.copied = make(chan struct{})
	var wg sync.WaitGroup
	copyPipe := func(pipe *outPipe) {
		defer wg.Done()
		defer pipe.closeReader()
		p.copyOut(pipe)
	}
	wg.Add(2)
	go copyPipe(p.stdout)
	go copyPipe(p.stderr)
	go func() { wg.Wait(); close(p.copied) }()
}

// abandon closes everything after a failed Start.
func (p *stepPipes) abandon() {
	p.closePTY()
	p.stdout.close()
	p.stderr.close()
}

// handoff runs once the step's own process has exited. Everything the step
// wrote is delivered to its writers, and from then on the pipes, which a
// backgrounded job may still hold, feed lateStdout and lateStderr instead.
func (p *stepPipes) handoff(lateStdout, lateStderr io.Writer) {
	p.closePTY()
	p.waitPTY(ptyDrainWait)
	if !canDrainNow {
		p.waitClosed(killDrain)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	drainNow(p.stdout)
	drainNow(p.stderr)
	p.stdout.dest, p.stderr.dest = lateStdout, lateStderr
}

// closed reports whether every process holding the pipes is gone.
func (p *stepPipes) closed() bool {
	select {
	case <-p.copied:
		return true
	default:
		return false
	}
}

// waitClosed waits up to d for the pipes to close.
func (p *stepPipes) waitClosed(d time.Duration) {
	select {
	case <-p.copied:
	case <-time.After(d):
	}
}

// drop stops copying: later writes are dropped and the read ends closed in
// the background, since a Close can block while a read is pending.
func (p *stepPipes) drop() {
	p.mu.Lock()
	p.abandoned = true
	p.mu.Unlock()
	go func() {
		p.stdout.closeReader()
		p.stderr.closeReader()
	}()
}

// withLock runs fn while no copy goroutine is writing.
func (p *stepPipes) withLock(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	fn()
}
