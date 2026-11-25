package tui

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// RunListener observes a run through its RunLog. It takes only plain values,
// so a front end can implement it without importing this package. Every
// string reaching it is already masked.
type RunListener interface {
	TaskEntered(name, info string)
	TaskLeft(name string, failed bool)
	// StepStarted announces a run: step, with its command as logged.
	StepStarted(task, cmd string)
	// StepHeld says the step just announced keeps its output out of sight (quiet:).
	StepHeld(task string)
	// OutputLine is a line of a command's output, ANSI stripped.
	OutputLine(task, line string)
	Var(task, name, display string)
	RunSucceeded()
	// RunFailed names the task of the step that failed, if any, and why.
	RunFailed(task, reason string)
	RunCancelled()
}

type runningTask struct {
	name   string
	failed bool
}

type taskFailure struct{ task, reason string }

// Listen makes l hear every event of the run from now on.
func (l *RunLog) Listen(listener RunListener) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.listener = listener
}

// StepHeld tells the listener the step that just started holds its output.
func (l *RunLog) StepHeld(task string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener != nil {
		l.listener.StepHeld(task)
	}
}

// RunSucceeded, RunFailed and RunCancelled end the run for the listener.
// fallback is what RunFailed reports when no step recorded a failure.
func (l *RunLog) RunSucceeded() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener != nil {
		l.listener.RunSucceeded()
	}
}

func (l *RunLog) RunFailed(fallback string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener == nil {
		return
	}
	if l.failed != nil {
		l.listener.RunFailed(l.failed.task, l.failed.reason)
		return
	}
	l.listener.RunFailed("", fallback)
}

func (l *RunLog) RunCancelled() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.listener != nil {
		l.listener.RunCancelled()
	}
}

// recordFailure keeps the first, innermost failed step for RunFailed.
func (l *RunLog) recordFailure(task, reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failed == nil {
		l.failed = &taskFailure{task, reason}
	}
}

// markFailed flags the innermost running task as failed for its TaskLeft.
func (l *RunLog) markFailed() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.running); n > 0 {
		l.running[n-1].failed = true
	}
}

// Tap returns a writer that passes everything to dest and reports each
// finished line of it to the listener, masked with mask. With no listener it
// is dest itself.
func (l *RunLog) Tap(task string, dest io.Writer, mask func(string) string) io.Writer {
	l.mu.Lock()
	listener := l.listener
	l.mu.Unlock()
	if listener == nil {
		return dest
	}
	return &lineTap{dest: dest, emit: func(line string) {
		l.mu.Lock()
		defer l.mu.Unlock()
		listener.OutputLine(task, mask(line))
	}}
}

// lineTap splits what it is written into lines, ended by \n or \r so a
// progress bar redrawing itself reports each state.
type lineTap struct {
	dest    io.Writer
	emit    func(string)
	pending strings.Builder
}

func (t *lineTap) Write(p []byte) (int, error) {
	n, err := t.dest.Write(p)
	for _, b := range p {
		if b != '\n' && b != '\r' {
			t.pending.WriteByte(b)
			continue
		}
		if line := strings.TrimSpace(ansi.Strip(t.pending.String())); line != "" {
			t.emit(line)
		}
		t.pending.Reset()
	}
	return n, err
}
