package runner

import (
	"errors"
	"io"
	"os"
	osExec "os/exec"
	"sync"
	"time"
)

// leftoverGrace is how long a step's leftover processes get, after SIGTERM,
// to exit and release its output pipes before they're SIGKILLed and the pipes
// abandoned. A var only so tests can shorten it (see SetLeftoverGrace).
var leftoverGrace = 2 * time.Second

// killDrain is how long output keeps draining after the SIGKILL, so whatever
// the killed processes left in the pipes still shows.
const killDrain = 100 * time.Millisecond

// errStepSettled is what a late write gets once settle has given up on the
// step's output: it ends the copy goroutine without touching later steps.
var errStepSettled = errors.New("step settled")

// outPipe is one OS pipe carrying a step's stdout or stderr.
type outPipe struct {
	r, w      *os.File
	closeOnce sync.Once
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
// step's process alone, then settle the leftovers and drain.
type stepPipes struct {
	stdout, stderr *outPipe
	copied         chan struct{}

	mu      sync.Mutex
	settled bool // set once settle stops waiting; later writes are dropped
}

func attachPipes(cmd *osExec.Cmd) (*stepPipes, error) {
	stdout, err := newOutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := newOutPipe()
	if err != nil {
		stdout.close()
		return nil, err
	}
	cmd.Stdout = stdout.w
	cmd.Stderr = stderr.w
	return &stepPipes{stdout: stdout, stderr: stderr}, nil
}

// startCopying closes hobnob's copies of the write ends (the child holds its
// own, so EOF arrives once every process holding them is gone) and copies the
// read ends into dest until then.
func (p *stepPipes) startCopying(stdoutDest, stderrDest io.Writer) {
	p.stdout.w.Close()
	p.stderr.w.Close()
	p.copied = make(chan struct{})
	var wg sync.WaitGroup
	copyPipe := func(pipe *outPipe, dest io.Writer) {
		defer wg.Done()
		defer pipe.closeReader()
		io.Copy(settleGate{p, dest}, pipe.r)
	}
	wg.Add(2)
	go copyPipe(p.stdout, stdoutDest)
	go copyPipe(p.stderr, stderrDest)
	go func() { wg.Wait(); close(p.copied) }()
}

// abandon closes everything after a failed Start.
func (p *stepPipes) abandon() {
	p.stdout.close()
	p.stderr.close()
}

// settle runs once the step's own process has exited. group is the step's
// process group, or 0 when it can't be signalled safely; termSent says a
// first CTRL+C has already SIGTERMed it.
//
// Anything the step left behind gets SIGTERM, and output drains until the
// pipes close. After leftoverGrace the group is SIGKILLed, and after killDrain
// the pipes are abandoned: later writes are dropped and the read ends closed
// in the background, since a Close can block while a read is pending.
func (p *stepPipes) settle(group int, termSent bool) {
	if group != 0 && !termSent {
		signalGroupTerm(group)
	}
	select {
	case <-p.copied:
		return
	case <-time.After(leftoverGrace):
	}
	if group != 0 {
		signalGroupKill(group)
		select {
		case <-p.copied:
			return
		case <-time.After(killDrain):
		}
	}
	p.mu.Lock()
	p.settled = true
	p.mu.Unlock()
	go func() {
		p.stdout.closeReader()
		p.stderr.closeReader()
	}()
}

// settleGate forwards a copy goroutine's writes to the step's writer until
// settle abandons the step, so a straggler can't write into a later step.
type settleGate struct {
	pipes *stepPipes
	dest  io.Writer
}

func (g settleGate) Write(b []byte) (int, error) {
	g.pipes.mu.Lock()
	defer g.pipes.mu.Unlock()
	if g.pipes.settled {
		return 0, errStepSettled
	}
	return g.dest.Write(b)
}
