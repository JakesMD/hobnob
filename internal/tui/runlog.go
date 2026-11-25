package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// RunLog is the one writer for the lines the runner and app print during a
// run, and for the output of the commands it starts.
type RunLog struct {
	stdout io.Writer
	stderr io.Writer

	mu       sync.Mutex
	running  []string
	stopping bool
	quiet    bool

	screen *Screen
}

// NewRunLog returns a RunLog writing to stdout, with failures and command
// stderr going to stderr.
func NewRunLog(stdout, stderr io.Writer) *RunLog {
	return &RunLog{stdout: stdout, stderr: stderr}
}

// SetQuiet makes the log keep only each run: command's line and every task's
// end; a failed step's held output and the failure lines still print.
func (l *RunLog) SetQuiet(quiet bool) { l.quiet = quiet }

// Quiet reports whether the log is quiet, so the runner holds a step's output.
func (l *RunLog) Quiet() bool { return l.quiet }

// Enter marks task as the one now running and returns the func that leaves it,
// so Stopping can name the innermost task.
func (l *RunLog) Enter(task string) (leave func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.running = append(l.running, task)
	return func() {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.running = l.running[:len(l.running)-1]
	}
}

// Stopping logs the first CTRL+C on the innermost running task. Later calls
// print nothing, so every path that notices the interrupt can call it.
func (l *RunLog) Stopping() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.stopping || len(l.running) == 0 {
		return
	}
	l.stopping = true
	task := l.running[len(l.running)-1]
	fmt.Fprintln(l.stdout, lead(task, SStep.Render("!"))+SStep.Render("stopping, ctrl+c again to force"))
}

// Run logs a run: step's command, which the caller passes already masked. The first line carries the notes,
// each as a grey "# …"; continuation lines of a multi-line command sit under
// it, indented to the command's start.
func (l *RunLog) Run(task, cmd string, notes ...string) {
	if l.quiet {
		notes = nil
	}
	lines := strings.Split(strings.TrimSuffix(cmd, "\n"), "\n")
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
	if l.quiet {
		return
	}
	fmt.Fprintln(l.stdout, lead(task, taskMark(task, "→"))+TaskStyle(calledTask).Render(calledTask))
}

// Var logs a get: value used, display already masked when it is a secret.
// note says where a value nobody typed came from.
func (l *RunLog) Var(task, name, display, note string) {
	if l.quiet {
		return
	}
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
	if l.quiet {
		return
	}
	fmt.Fprintln(l.stdout, lead(task, SInfo.Render("⊘"))+SInfo.Render(what))
}

// StepDone logs the end of a step that succeeded: "task ✓ 1.6s".
func (l *RunLog) StepDone(task string, elapsed time.Duration) {
	if l.quiet {
		return
	}
	fmt.Fprintln(l.stdout, lead(task, SChecked.Render("✓"))+SInfo.Render(formatElapsed(elapsed)))
}

// StepFailed logs the end of a step that failed, with the command as written
// and why it failed.
func (l *RunLog) StepFailed(task string, elapsed time.Duration, cmd, reason string) {
	fmt.Fprintln(l.stderr, lead(task, SError.Render("✗"))+SInfo.Render(formatElapsed(elapsed))+" "+cmd+": "+reason)
}

// StepSoftFailed logs the end of a soft: step that failed; the task carries on.
func (l *RunLog) StepSoftFailed(task string, elapsed time.Duration, cmd, reason string) {
	fmt.Fprintln(l.stdout, lead(task, SStep.Render("!"))+SInfo.Render(formatElapsed(elapsed))+" "+cmd+": "+reason)
}

// StepError logs a step that failed with a message but no run to time, soft
// or not.
func (l *RunLog) StepError(task string, soft bool, message string) {
	if soft {
		fmt.Fprintln(l.stdout, lead(task, SStep.Render("!"))+SStep.Render(message))
		return
	}
	fmt.Fprintln(l.stderr, lead(task, SError.Render("✗"))+SError.Render(message))
}

// TaskDone logs the end of a task that succeeded, counting its soft failures.
func (l *RunLog) TaskDone(task string, elapsed time.Duration, softFailures int) {
	line := lead(task, SChecked.Render("✓")) + SChecked.Render("done") + " " + SInfo.Render("after "+formatElapsed(elapsed))
	if softFailures > 0 {
		noun := "soft failures"
		if softFailures == 1 {
			noun = "soft failure"
		}
		line += SInfo.Render(fmt.Sprintf(" • %d %s", softFailures, noun))
	}
	fmt.Fprintln(l.stdout, line)
}

// TaskFailed logs the end of a task that failed, or stopped when it was
// interrupted.
func (l *RunLog) TaskFailed(task string, elapsed time.Duration, stopped bool) {
	word := "failed"
	if stopped {
		word = "stopped"
	}
	fmt.Fprintln(l.stderr, lead(task, SError.Render("✗"))+SError.Render(word)+" "+SInfo.Render("after "+formatElapsed(elapsed)))
}

func formatElapsed(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// Watch tells the log a person watches on screen: command output is then
// written as it arrives and wrapped to the screen, and PTYSize applies.
func (l *RunLog) Watch(screen *Screen) { l.screen = screen }

// PTYSize is the size of the PTY for a command of task: the screen's width
// less the output prefix, and its height less the two rows the status line
// takes. ok is false when nobody watches, so commands get pipes.
func (l *RunLog) PTYSize(task string) (cols, rows int, ok bool) {
	if l.screen == nil {
		return 0, 0, false
	}
	cols, rows = l.screen.Size()
	cols -= lipgloss.Width(l.outputPrefix(task))
	return max(cols, 1), max(rows-2, 1), true
}

// OnResize calls fn whenever the watched screen is resized, until stop. It
// does nothing when nobody watches.
func (l *RunLog) OnResize(fn func()) (stop func()) {
	if l.screen == nil {
		return func() {}
	}
	return l.screen.OnResize(fn)
}

func (l *RunLog) outputPrefix(task string) string { return lead(task, SInfo.Render("│")) }

// Output returns the writers for a command's stdout and stderr, each line
// prefixed "task │".
func (l *RunLog) Output(task string) (stdout, stderr *LineWriter) {
	prefix := l.outputPrefix(task)
	stdout, stderr = NewLineWriter(l.stdout, prefix), NewLineWriter(l.stderr, prefix)
	if l.screen != nil {
		width := func() int { cols, _ := l.screen.Size(); return cols }
		stdout.Watch(width)
		stderr.Watch(width)
	}
	return stdout, stderr
}
