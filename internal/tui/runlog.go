package tui

import (
	"fmt"
	"io"
)

// RunLog is the one writer for the lines the runner and app print during a
// run, and for the output of the commands it starts. The get: line is not
// routed through it yet.
type RunLog struct {
	stdout io.Writer
	stderr io.Writer
}

// NewRunLog returns a RunLog writing to stdout, with failures and command
// stderr going to stderr.
func NewRunLog(stdout, stderr io.Writer) *RunLog {
	return &RunLog{stdout: stdout, stderr: stderr}
}

// Run logs a run: step's command with its dir: when it has one. It does not
// mask: the caller passes cmd already masked.
func (l *RunLog) Run(task, cmd, dir string) {
	for _, line := range RunDisplayLines(cmd, task, dir) {
		fmt.Fprintln(l.stdout, line)
	}
}

// CallCached logs a call: step that reused a once: task's cached result.
func (l *RunLog) CallCached(task, calledTask, summary string) {
	fmt.Fprintln(l.stdout, CallCacheHitLine(task, calledTask, summary))
}

// TaskSkipped logs a task whose if: was false.
func (l *RunLog) TaskSkipped(task string) {
	fmt.Fprintln(l.stdout, SkipLine(task))
}

// RunSkipped logs a run: step whose if: was false.
func (l *RunLog) RunSkipped(task string) {
	fmt.Fprintln(l.stdout, RunSkipLine(task))
}

// Note logs the stand-in line for a quiet: run: step's hidden output.
func (l *RunLog) Note(task, msg string) {
	fmt.Fprintln(l.stdout, RunQuietLine(task, msg))
}

// TaskDone logs the end of a task that succeeded.
func (l *RunLog) TaskDone(task string) {
	fmt.Fprintln(l.stdout, SChecked.Render("✓")+" "+TaskPrefix(task)+SChecked.Render("done"))
}

// TaskFailed logs the end of a task that failed or was interrupted.
func (l *RunLog) TaskFailed(task, message string) {
	fmt.Fprintln(l.stderr, SError.Render("✗")+" "+TaskPrefix(task)+SError.Render(message))
}

// Output returns the writers for a command's stdout and stderr, each line
// prefixed with the task's name.
func (l *RunLog) Output(task string) (stdout, stderr *LineWriter) {
	prefix := TaskPrefix(task)
	return NewLineWriter(l.stdout, prefix), NewLineWriter(l.stderr, prefix)
}
