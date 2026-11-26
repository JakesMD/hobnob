package browser

import (
	"strings"
	"sync"
	"time"

	"hobnob/internal/runevents"
)

// state is what the run has shown so far. It is a runevents.Listener, and
// snapshot turns it into what the page shows.
type state struct {
	runevents.Nop

	mu      sync.Mutex
	version int
	changed func()

	task     Task
	setup    []setupEntry
	running  []int // indexes into setup, outermost first
	step     stepState
	started  time.Time
	result   *Result
	logLines []string
	// failureLogged is whether logLines already holds a failure line, so a
	// run that failed before any step doesn't lack one.
	failureLogged bool

	picking    bool
	tasks      []Task
	home       bool
	homed      bool
	prompt     *openPrompt
	nextPrompt int
	answers    []RailEntry

	cancelling bool
	// stop and kill act on the live run; both are nil between runs.
	stop, kill func()
}

type setupEntry struct {
	name, label, state string
}

type stepState struct {
	held     bool
	lastLine string
	began    time.Time
}

func label(info, name string) string {
	if info != "" {
		return info
	}
	return name
}

func (s *state) update(fn func()) {
	s.mu.Lock()
	fn()
	s.version++
	s.mu.Unlock()
	if s.changed != nil {
		s.changed()
	}
}

func (s *state) logf(task, mark, text string) {
	s.logLines = append(s.logLines, task+" "+mark+" "+text)
}

func (s *state) TaskEntered(name, info string) {
	s.update(func() {
		if len(s.setup) == 0 {
			s.task = Task{Name: name, Info: info}
			s.started = time.Now()
		}
		s.setup = append(s.setup, setupEntry{name: name, label: label(info, name), state: StateRunning})
		s.running = append(s.running, len(s.setup)-1)
	})
}

func (s *state) TaskLeft(name string, failed bool) {
	s.update(func() {
		last := s.running[len(s.running)-1]
		s.running = s.running[:len(s.running)-1]
		s.setup[last].state = StateDone
		if failed {
			s.setup[last].state = StateFailed
		}
	})
}

func (s *state) StepStarted(task, cmd string, notes []string) {
	s.update(func() {
		s.step = stepState{began: time.Now()}
		s.logf(task, "$", cmd+noteSuffix(notes))
	})
}

func (s *state) StepHeld(string) {
	s.update(func() { s.step.held = true })
}

func (s *state) OutputLine(task, line string) {
	s.update(func() {
		s.step.lastLine = line
		s.logf(task, "│", line)
	})
}

func (s *state) StepSkipped(task, what string) {
	s.update(func() { s.logf(task, "⊘", what) })
}

func (s *state) StepDone(task string, elapsed time.Duration) {
	s.update(func() { s.logf(task, "✓", runevents.FormatElapsed(elapsed)) })
}

func (s *state) StepFailed(task string, elapsed time.Duration, cmd, reason string) {
	s.update(func() {
		s.logf(task, "✗", runevents.FormatElapsed(elapsed)+" "+cmd+": "+reason)
		s.failureLogged = true
	})
}

func (s *state) StepSoftFailed(task string, elapsed time.Duration, cmd, reason string) {
	s.update(func() { s.logf(task, "!", runevents.FormatElapsed(elapsed)+" "+cmd+": "+reason) })
}

func (s *state) StepError(task string, soft bool, message string) {
	s.update(func() {
		if soft {
			s.logf(task, "!", message)
			return
		}
		s.logf(task, "✗", message)
		s.failureLogged = true
	})
}

func (s *state) TaskSkipped(task string) {
	s.update(func() { s.logf(task, "⊘", "skipped") })
}

func (s *state) TaskDone(task string, elapsed time.Duration, softFailures int) {
	s.update(func() {
		text := "done after " + runevents.FormatElapsed(elapsed)
		if softFailures > 0 {
			text += " • " + runevents.SoftFailureCount(softFailures)
		}
		s.logf(task, "✓", text)
	})
}

func (s *state) TaskFailed(task string, elapsed time.Duration, stopped bool) {
	s.update(func() {
		word := "failed"
		if stopped {
			word = "stopped"
		}
		s.logf(task, "✗", word+" after "+runevents.FormatElapsed(elapsed))
	})
}

func (s *state) Call(task, called string) {
	s.update(func() { s.logf(task, "→", called) })
}

func (s *state) DoneEarlier(task string) {
	s.update(func() { s.logf(task, "✓", "done earlier") })
}

func (s *state) Var(task, name, display, note string) {
	s.update(func() { s.logf(task, "»", name+"="+display+noteSuffix([]string{note})) })
}

func (s *state) VarSkipped(task, name string) {
	s.update(func() { s.logf(task, "⊘", name) })
}

func (s *state) Say(task, message string) {
	s.update(func() { s.logf(task, "»", message) })
}

func noteSuffix(notes []string) string {
	var suffix strings.Builder
	for _, note := range notes {
		if note != "" {
			suffix.WriteString(" # " + note)
		}
	}
	return suffix.String()
}

func (s *state) Finished(outcome runevents.Outcome) {
	s.update(func() {
		res := &Result{Outcome: outcomeName(outcome.Kind)}
		if !s.started.IsZero() {
			res.DurationMS = time.Since(s.started).Milliseconds()
		}
		if outcome.Kind == runevents.Failed {
			res.Label = outcome.Label
			res.LastLine = s.step.lastLine
			if res.LastLine == "" {
				res.LastLine = outcome.Reason
			}
			if !s.failureLogged {
				s.logf(outcome.Task, "✗", outcome.Reason)
			}
		}
		res.Log = strings.Join(s.logLines, "\n")
		s.result = res
	})
}

func outcomeName(kind runevents.Kind) string {
	switch kind {
	case runevents.Cancelled:
		return OutcomeCancelled
	case runevents.Failed:
		return OutcomeFailed
	}
	return OutcomeDone
}

// startPick clears the last run and shows the task picker.
func (s *state) startPick(tasks []Task, offerHome bool) {
	s.update(func() {
		s.task, s.setup, s.running, s.step = Task{}, nil, nil, stepState{}
		s.started, s.result, s.logLines, s.failureLogged = time.Time{}, nil, nil, false
		s.prompt, s.answers, s.cancelling = nil, nil, false
		s.picking, s.tasks, s.home, s.homed = true, tasks, offerHome, false
	})
}

// choose ends the pick for name, which must be one the picker offered.
func (s *state) choose(name string) bool {
	chosen := false
	s.mu.Lock()
	if s.picking {
		for _, task := range s.tasks {
			chosen = chosen || task.Name == name
		}
	}
	if chosen {
		s.picking = false
		s.version++
	}
	s.mu.Unlock()
	if chosen && s.changed != nil {
		s.changed()
	}
	return chosen
}

// goHome accepts Home, once, on a finished run that offers it.
func (s *state) goHome() bool {
	s.mu.Lock()
	ok := s.result != nil && s.home && !s.homed
	s.homed = s.homed || ok
	s.mu.Unlock()
	return ok
}

func (s *state) snapshot() (Snapshot, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.picking {
		return Snapshot{Screen: ScreenTasks, Tasks: s.tasks, Rail: Rail{Answers: []RailEntry{}, Setup: []RailEntry{}}}, s.version
	}
	snap := Snapshot{Screen: ScreenProgress, Task: s.task, Rail: Rail{Answers: []RailEntry{}, Setup: []RailEntry{}}}
	snap.Rail.Answers = append(snap.Rail.Answers, s.answers...)
	for _, entry := range s.setup {
		snap.Rail.Setup = append(snap.Rail.Setup, RailEntry{Label: entry.label, State: entry.state})
	}
	if s.result != nil {
		snap.Screen = ScreenDone
		if s.result.Outcome != OutcomeDone {
			snap.Screen = ScreenFailed
		}
		res := *s.result
		res.Home = s.home
		snap.Result = &res
		return snap, s.version
	}
	if s.prompt != nil {
		snap.Screen = ScreenPrompt
		prompt := s.prompt.Prompt
		snap.Prompt = &prompt
		return snap, s.version
	}
	progress := &Progress{}
	if len(s.running) > 0 {
		progress.Label = s.setup[s.running[len(s.running)-1]].label
	}
	if !s.step.held {
		progress.LastLine = s.step.lastLine
	}
	if !s.started.IsZero() {
		progress.ElapsedMS = time.Since(s.started).Milliseconds()
	}
	progress.Cancelling = s.cancelling
	snap.Progress = progress
	return snap, s.version
}
