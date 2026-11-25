//go:build !unix && !windows

package runner

import (
	"context"
	"errors"
	"io"
	"os"
	osExec "os/exec"
	"time"
)

// newPTYPipe: there is no PTY here, so steps get pipes.
func newPTYPipe(size ptySize) (*outPipe, *os.File, error) {
	return nil, nil, errors.New("no PTY on this platform")
}

func setPTYSize(master *os.File, size ptySize) {}

func (p *stepPipes) startOnPTY(ctx context.Context, cmd *osExec.Cmd) error { return cmd.Start() }
func (p *stepPipes) closePTY()                                             {}
func (p *stepPipes) waitPTY(time.Duration)                                 {}

func (p *stepPipes) filterPTY(pipe *outPipe, w io.Writer) (io.Writer, func()) { return w, func() {} }
