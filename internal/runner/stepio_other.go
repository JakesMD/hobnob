//go:build !unix

package runner

import (
	"errors"
	"io"
)

// errStepAbandoned is what a late write gets once the run has given up on the
// step's output: it ends the copy goroutine.
var errStepAbandoned = errors.New("step output abandoned")

// canDrainNow: the pipes' read ends block, so handoff waits briefly for the
// copy goroutines instead.
const canDrainNow = false

// copyOut copies pipe into its dest until EOF.
func (p *stepPipes) copyOut(pipe *outPipe) {
	w, done := p.filterPTY(pipe, pipeWriter{p, pipe})
	io.Copy(w, pipe.r)
	done()
}

func drainNow(pipe *outPipe) {}

// pipeWriter writes to pipe's current dest until the run abandons the step.
type pipeWriter struct {
	pipes *stepPipes
	pipe  *outPipe
}

func (w pipeWriter) Write(b []byte) (int, error) {
	w.pipes.mu.Lock()
	defer w.pipes.mu.Unlock()
	if w.pipes.abandoned {
		return 0, errStepAbandoned
	}
	return w.pipe.dest.Write(b)
}
