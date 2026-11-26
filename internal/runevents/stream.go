// Package runevents is the one stream of everything a run does: tasks
// entering and leaving, steps starting and ending, output lines, variables,
// says and the outcome. The runner emits into a Stream; the terminal log and
// the browser page are Listeners on it. The stream also owns what the events
// mean together: which failure ended the run, and which task it names.
//
// Every string reaching a Listener is already masked.
package runevents

import (
	"fmt"
	"sync"
	"time"
)

// Listener hears every event of a run, one at a time and in order. Nop lets
// an implementation take only the events it shows.
type Listener interface {
	TaskEntered(name, info string)
	// TaskLeft ends the task entered last; failed says it ended in failure.
	TaskLeft(name string, failed bool)
	TaskSkipped(task string)
	TaskDone(task string, elapsed time.Duration, softFailures int)
	// TaskFailed precedes the TaskLeft of a task that failed or was stopped.
	TaskFailed(task string, elapsed time.Duration, stopped bool)
	// Stopping is the first CTRL+C, on the innermost running task.
	Stopping(task string)

	// StepStarted announces a run: step, with its command as logged and the
	// notes (dir:, quiet:) for its line.
	StepStarted(task, cmd string, notes []string)
	// StepHeld says the step just announced keeps its output out of sight (quiet:).
	StepHeld(task string)
	// OutputLine is a line of a command's output, ANSI stripped.
	OutputLine(task, line string)
	StepSkipped(task, what string)
	StepDone(task string, elapsed time.Duration)
	StepFailed(task string, elapsed time.Duration, cmd, reason string)
	StepSoftFailed(task string, elapsed time.Duration, cmd, reason string)
	// StepError is a step that failed with a message but no run to time.
	StepError(task string, soft bool, message string)

	Call(task, calledTask string)
	DoneEarlier(task string)
	// Var is a get: value used; note says where a value nobody typed came from.
	Var(task, name, display, note string)
	VarSkipped(task, name string)
	Say(task, message string)

	// Finished is the last event of a run.
	Finished(Outcome)
}

// Kind is how a run ended.
type Kind int

const (
	Succeeded Kind = iota
	Cancelled
	Failed
)

// Outcome is how a run ended. For Failed, Task and Label name the step's
// task that ended the run, else the innermost task that failed or was still
// running, else the run's first task; they are empty only when the run failed
// before entering any task. Label is the task's info, or its name without one.
type Outcome struct {
	Kind   Kind
	Task   string
	Label  string
	Reason string
}

// Stream sends the events of one run to its Listeners.
type Stream struct {
	mu        sync.Mutex
	listeners []Listener
	stopping  bool
	tasks     []*task
	running   []*task
	failure   *failure
}

type task struct {
	name, label string
	failed      bool
	left        bool
}

type failure struct{ task, label, reason string }

// New returns a Stream heard by listeners.
func New(listeners ...Listener) *Stream {
	return &Stream{listeners: listeners}
}

// Listen makes l hear every event from now on.
func (s *Stream) Listen(l Listener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners = append(s.listeners, l)
}

func (s *Stream) emit(fn func(Listener)) {
	for _, l := range s.listeners {
		fn(l)
	}
}

// Enter reports name as the task now running and returns the func that leaves it.
func (s *Stream) Enter(name, info string) (leave func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entered := &task{name: name, label: label(info, name)}
	s.tasks = append(s.tasks, entered)
	s.running = append(s.running, entered)
	s.emit(func(l Listener) { l.TaskEntered(name, info) })
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.running = s.running[:len(s.running)-1]
		entered.left = true
		s.emit(func(l Listener) { l.TaskLeft(name, entered.failed) })
	}
}

func label(info, name string) string {
	if info != "" {
		return info
	}
	return name
}

// Stopping reports the first CTRL+C on the innermost running task. Later
// calls say nothing, so every path that notices the interrupt can call it.
func (s *Stream) Stopping() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping || len(s.running) == 0 {
		return
	}
	s.stopping = true
	name := s.running[len(s.running)-1].name
	s.emit(func(l Listener) { l.Stopping(name) })
}

func (s *Stream) TaskSkipped(task string) {
	s.locked(func(l Listener) { l.TaskSkipped(task) })
}

func (s *Stream) TaskDone(task string, elapsed time.Duration, softFailures int) {
	s.locked(func(l Listener) { l.TaskDone(task, elapsed, softFailures) })
}

// TaskFailed flags the innermost running task as failed for its TaskLeft.
func (s *Stream) TaskFailed(task string, elapsed time.Duration, stopped bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n := len(s.running); n > 0 {
		s.running[n-1].failed = true
	}
	s.emit(func(l Listener) { l.TaskFailed(task, elapsed, stopped) })
}

func (s *Stream) StepStarted(task, cmd string, notes ...string) {
	s.locked(func(l Listener) { l.StepStarted(task, cmd, notes) })
}

func (s *Stream) StepHeld(task string) {
	s.locked(func(l Listener) { l.StepHeld(task) })
}

func (s *Stream) StepSkipped(task, what string) {
	s.locked(func(l Listener) { l.StepSkipped(task, what) })
}

func (s *Stream) StepDone(task string, elapsed time.Duration) {
	s.locked(func(l Listener) { l.StepDone(task, elapsed) })
}

// StepFailed keeps the first, innermost failed step for the outcome.
func (s *Stream) StepFailed(task string, elapsed time.Duration, cmd, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordFailure(task, reason)
	s.emit(func(l Listener) { l.StepFailed(task, elapsed, cmd, reason) })
}

// StepSoftFailed forgives any failure recorded so far: the task carries on.
func (s *Stream) StepSoftFailed(task string, elapsed time.Duration, cmd, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failure = nil
	s.emit(func(l Listener) { l.StepSoftFailed(task, elapsed, cmd, reason) })
}

func (s *Stream) StepError(task string, soft bool, message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !soft {
		s.recordFailure(task, message)
	}
	s.emit(func(l Listener) { l.StepError(task, soft, message) })
}

func (s *Stream) Call(task, calledTask string) {
	s.locked(func(l Listener) { l.Call(task, calledTask) })
}

func (s *Stream) DoneEarlier(task string) {
	s.locked(func(l Listener) { l.DoneEarlier(task) })
}

func (s *Stream) Var(task, name, display, note string) {
	s.locked(func(l Listener) { l.Var(task, name, display, note) })
}

func (s *Stream) VarSkipped(task, name string) {
	s.locked(func(l Listener) { l.VarSkipped(task, name) })
}

func (s *Stream) Say(task, message string) {
	s.locked(func(l Listener) { l.Say(task, message) })
}

func (s *Stream) locked(fn func(Listener)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.emit(fn)
}

func (s *Stream) recordFailure(name, reason string) {
	if s.failure == nil {
		s.failure = &failure{task: name, label: s.labelOf(name), reason: reason}
	}
}

// labelOf is the label of the running task called name, else of the last
// task of that name, else name itself.
func (s *Stream) labelOf(name string) string {
	for i := len(s.running) - 1; i >= 0; i-- {
		if s.running[i].name == name {
			return s.running[i].label
		}
	}
	for i := len(s.tasks) - 1; i >= 0; i-- {
		if s.tasks[i].name == name {
			return s.tasks[i].label
		}
	}
	return name
}

// innermost is the last task that failed or is still running, else the
// run's first task.
func (s *Stream) innermost() (name, label string) {
	for i := len(s.tasks) - 1; i >= 0; i-- {
		if t := s.tasks[i]; !t.left || t.failed {
			return t.name, t.label
		}
	}
	if len(s.tasks) > 0 {
		return s.tasks[0].name, s.tasks[0].label
	}
	return "", ""
}

// Finish ends the run: err is what it returned and cancelled says err is the
// interrupt. The outcome reaches every listener.
func (s *Stream) Finish(err error, cancelled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	outcome := Outcome{Kind: Succeeded}
	switch {
	case err == nil:
	case cancelled:
		outcome.Kind = Cancelled
	case s.failure != nil:
		outcome = Outcome{Kind: Failed, Task: s.failure.task, Label: s.failure.label, Reason: s.failure.reason}
	default:
		outcome = Outcome{Kind: Failed, Reason: err.Error()}
		outcome.Task, outcome.Label = s.innermost()
	}
	s.emit(func(l Listener) { l.Finished(outcome) })
}

// FormatElapsed writes d as "1.6s", or "2m05s" from a minute up.
func FormatElapsed(d time.Duration) string {
	if d >= time.Minute {
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// SoftFailureCount writes "1 soft failure" or "3 soft failures".
func SoftFailureCount(n int) string {
	if n == 1 {
		return "1 soft failure"
	}
	return fmt.Sprintf("%d soft failures", n)
}
