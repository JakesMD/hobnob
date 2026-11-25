package tui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RunLog is the one writer for the lines the runner and app print during a
// run, and for the output of the commands it starts.
type RunLog struct {
	stdout io.Writer
	stderr io.Writer
}

// NewRunLog returns a RunLog writing to stdout, with failures and command
// stderr going to stderr.
func NewRunLog(stdout, stderr io.Writer) *RunLog {
	return &RunLog{stdout: stdout, stderr: stderr}
}

// Run logs a run: step's command, which the caller passes already masked. The first line carries the notes,
// each as a grey "# …"; continuation lines of a multi-line command sit under
// it, indented to the command's start.
func (l *RunLog) Run(task, cmd string, notes ...string) {
	lines := strings.Split(cmd, "\n")
	first := lead(task, taskMark(task, "$")) + lines[0]
	for _, note := range notes {
		first += " " + SInfo.Render("# "+note)
	}
	fmt.Fprintln(l.stdout, first)
	indent := strings.Repeat(" ", lipgloss.Width(lead(task, "$")))
	for _, line := range lines[1:] {
		fmt.Fprintln(l.stdout, indent+line)
	}
}

// Call logs a call: step, the called task in its own colour.
func (l *RunLog) Call(task, calledTask string) {
	fmt.Fprintln(l.stdout, lead(task, taskMark(task, "→"))+TaskStyle(calledTask).Render(calledTask))
}

// Var logs a get: value used, display already masked when it is a secret.
// note says where a value nobody typed came from.
func (l *RunLog) Var(task, name, display, note string) {
	line := lead(task, taskMark(task, "»")) + name + "=" + display
	if note != "" {
		line += " " + SInfo.Render("# "+note)
	}
	fmt.Fprintln(l.stdout, line)
}

// VarSkipped logs an optional get: that got no value.
func (l *RunLog) VarSkipped(task, name string) {
	l.RunSkipped(task, name)
}

// DoneEarlier logs the end of a once: task a call: found already run.
func (l *RunLog) DoneEarlier(task string) {
	fmt.Fprintln(l.stdout, lead(task, SChecked.Render("✓"))+SChecked.Render("done")+" "+SInfo.Render("earlier"))
}

// TaskSkipped logs a task whose if: was false.
func (l *RunLog) TaskSkipped(task string) {
	l.RunSkipped(task, "skipped")
}

// RunSkipped logs a skipped step, naming what it would have run.
func (l *RunLog) RunSkipped(task, what string) {
	fmt.Fprintln(l.stdout, lead(task, SInfo.Render("⊘"))+SInfo.Render(what))
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
// prefixed "task │".
func (l *RunLog) Output(task string) (stdout, stderr *LineWriter) {
	prefix := lead(task, SInfo.Render("│"))
	return NewLineWriter(l.stdout, prefix), NewLineWriter(l.stderr, prefix)
}
