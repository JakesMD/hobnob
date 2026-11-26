package tui

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"

	"hobnob/internal/runevents"
)

// RunLog is the terminal's listener on a run's events: it prints each as a
// line, and writes the output of the commands the runner starts. It also owns
// what the terminal needs while a run is on screen: the live area, the PTY
// size and Pause.
type RunLog struct {
	runevents.Nop

	stdout io.Writer
	stderr io.Writer

	// logOut and logErr take hobnob's own lines: stdout and stderr, or the
	// live area's wrappers of them when watched.
	logOut io.Writer
	logErr io.Writer

	mu    sync.Mutex
	depth int
	quiet bool

	screen *Screen
	live   *liveArea
}

// NewRunLog returns a RunLog writing to stdout, with failures and command
// stderr going to stderr.
func NewRunLog(stdout, stderr io.Writer) *RunLog {
	return &RunLog{stdout: stdout, stderr: stderr, logOut: stdout, logErr: stderr}
}

// SetQuiet makes the log keep only each run: command's line and every task's
// end; a failed step's held output and the failure lines still print.
func (l *RunLog) SetQuiet(quiet bool) { l.quiet = quiet }

// Quiet reports whether the log is quiet, so the runner holds a step's output.
func (l *RunLog) Quiet() bool { return l.quiet }

// TaskEntered starts the live area with the run's first task.
func (l *RunLog) TaskEntered(string, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.depth++
	if l.depth == 1 && l.live != nil {
		l.live.begin()
	}
}

// TaskLeft ends the live area with the run's first task.
func (l *RunLog) TaskLeft(string, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.depth--
	if l.depth == 0 && l.live != nil {
		l.live.end()
	}
}

// Stopping logs the first CTRL+C on task.
func (l *RunLog) Stopping(task string) {
	if l.live != nil {
		l.live.setNote(stoppingNote)
	}
	fmt.Fprintln(l.logOut, lead(task, SStep.Render("!"))+SStep.Render(stoppingNote))
}

// StepStarted logs a run: step's command. The first line carries the notes,
// each as a grey "# …"; continuation lines of a multi-line command sit under
// it, indented to the command's start.
func (l *RunLog) StepStarted(task, cmd string, notes []string) {
	if l.quiet {
		notes = nil
	}
	lines := strings.Split(strings.TrimSuffix(cmd, "\n"), "\n")
	first := lead(task, taskMark(task, "$")) + lines[0]
	for _, note := range notes {
		first += " " + SInfo.Render("# "+note)
	}
	fmt.Fprintln(l.logOut, first)
	indent := strings.Repeat(" ", lipgloss.Width(lead(task, "$")))
	for _, line := range lines[1:] {
		fmt.Fprintln(l.logOut, indent+line)
	}
	if l.live != nil {
		l.live.stepBegin()
	}
}

// Call logs a call: step, the called task in its own colour.
func (l *RunLog) Call(task, calledTask string) {
	if l.quiet {
		return
	}
	fmt.Fprintln(l.logOut, lead(task, taskMark(task, "→"))+TaskStyle(calledTask).Render(calledTask))
}

// Var logs a get: value used. note says where a value nobody typed came from.
func (l *RunLog) Var(task, name, display, note string) {
	if l.quiet {
		return
	}
	line := lead(task, taskMark(task, "»")) + name + "=" + display
	if note != "" {
		line += " " + SInfo.Render("# "+note)
	}
	fmt.Fprintln(l.logOut, line)
}

// Say logs a say: step's message, already masked. It shows under --quiet
// too: it was written for the person running the task.
func (l *RunLog) Say(task, message string) {
	lines := strings.Split(message, "\n")
	fmt.Fprintln(l.logOut, lead(task, taskMark(task, "»"))+SBold.Render(lines[0]))
	indent := strings.Repeat(" ", lipgloss.Width(lead(task, "»")))
	for _, line := range lines[1:] {
		fmt.Fprintln(l.logOut, indent+SBold.Render(line))
	}
}

// VarSkipped logs an optional get: that got no value.
func (l *RunLog) VarSkipped(task, name string) {
	l.StepSkipped(task, name)
}

// DoneEarlier logs the end of a once: task a call: found already run.
func (l *RunLog) DoneEarlier(task string) {
	fmt.Fprintln(l.logOut, lead(task, SChecked.Render("✓"))+SChecked.Render("done")+" "+SInfo.Render("earlier"))
}

// TaskSkipped logs a task whose if: was false.
func (l *RunLog) TaskSkipped(task string) {
	l.StepSkipped(task, "skipped")
}

// StepSkipped logs a skipped step, naming what it would have run.
func (l *RunLog) StepSkipped(task, what string) {
	if l.quiet {
		return
	}
	fmt.Fprintln(l.logOut, lead(task, SInfo.Render("⊘"))+SInfo.Render(what))
}

// StepDone logs the end of a step that succeeded: "task ✓ 1.6s".
func (l *RunLog) StepDone(task string, elapsed time.Duration) {
	l.endStep()
	if l.quiet {
		return
	}
	fmt.Fprintln(l.logOut, lead(task, SChecked.Render("✓"))+SInfo.Render(runevents.FormatElapsed(elapsed)))
}

// StepFailed logs the end of a step that failed, with the command as written
// and why it failed.
func (l *RunLog) StepFailed(task string, elapsed time.Duration, cmd, reason string) {
	l.endStep()
	fmt.Fprintln(l.logErr, lead(task, SError.Render("✗"))+SInfo.Render(runevents.FormatElapsed(elapsed))+" "+cmd+": "+reason)
}

// StepSoftFailed logs the end of a soft: step that failed; the task carries on.
func (l *RunLog) StepSoftFailed(task string, elapsed time.Duration, cmd, reason string) {
	l.endStep()
	fmt.Fprintln(l.logOut, lead(task, SStep.Render("!"))+SInfo.Render(runevents.FormatElapsed(elapsed))+" "+cmd+": "+reason)
}

// StepError logs a step that failed with a message but no run to time, soft
// or not.
func (l *RunLog) StepError(task string, soft bool, message string) {
	l.endStep()
	if soft {
		fmt.Fprintln(l.logOut, lead(task, SStep.Render("!"))+SStep.Render(message))
		return
	}
	fmt.Fprintln(l.logErr, lead(task, SError.Render("✗"))+SError.Render(message))
}

// TaskDone logs the end of a task that succeeded, counting its soft failures.
func (l *RunLog) TaskDone(task string, elapsed time.Duration, softFailures int) {
	line := lead(task, SChecked.Render("✓")) + SChecked.Render("done") + " " + SInfo.Render("after "+runevents.FormatElapsed(elapsed))
	if softFailures > 0 {
		line += SInfo.Render(" • " + runevents.SoftFailureCount(softFailures))
	}
	fmt.Fprintln(l.logOut, line)
}

// TaskFailed logs the end of a task that failed, or stopped when it was
// interrupted.
func (l *RunLog) TaskFailed(task string, elapsed time.Duration, stopped bool) {
	word := "failed"
	if stopped {
		word = "stopped"
	}
	fmt.Fprintln(l.logErr, lead(task, SError.Render("✗"))+SError.Render(word)+" "+SInfo.Render("after "+runevents.FormatElapsed(elapsed)))
}

func (l *RunLog) endStep() {
	if l.live != nil {
		l.live.stepEnd()
	}
}

// Watch tells the log a person watches on screen: command output is then
// written as it arrives and wrapped to the screen, and PTYSize applies.
func (l *RunLog) Watch(screen *Screen) {
	l.screen = screen
	if screen == nil {
		return
	}
	l.live = newLiveArea(l.stdout, screen)
	l.logOut = lineSink{l.live, l.stdout}
	l.logErr = lineSink{l.live, l.stderr}
}

// Pause clears the status line while a card prompt is open, until resume.
func (l *RunLog) Pause() (resume func()) {
	if l.live == nil {
		return func() {}
	}
	return l.live.pause()
}

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
	if l.live == nil {
		return NewLineWriter(l.stdout, prefix), NewLineWriter(l.stderr, prefix)
	}
	restart := func() []byte {
		stdout.Restart()
		stderr.Restart()
		return []byte(prefix)
	}
	stdout = NewLineWriter(liveWriter{l.live, l.stdout, restart}, prefix)
	stderr = NewLineWriter(liveWriter{l.live, l.stderr, restart}, prefix)
	if l.screen != nil {
		width := func() int { cols, _ := l.screen.Size(); return cols }
		stdout.Watch(width)
		stderr.Watch(width)
		stdout.now, stderr.now = l.live.now, l.live.now
	}
	return stdout, stderr
}
