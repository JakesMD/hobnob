package web

import (
	"strings"
	"sync"
	"time"
)

// state is what the run has shown so far. It implements tui.RunListener, and
// snapshot turns it into what the page shows.
type state struct {
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

	picking bool
	tasks   []Task
	home    bool
	homed   bool
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

func (s *state) StepStarted(task, cmd string) {
	s.update(func() {
		s.step = stepState{began: time.Now()}
		s.logf(task, "$", cmd)
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

func (s *state) Var(task, name, display string) {
	s.update(func() { s.logf(task, "»", name+"="+display) })
}

func (s *state) RunSucceeded() { s.finish(OutcomeDone, "", "") }
func (s *state) RunCancelled() { s.finish(OutcomeCancelled, "", "") }

func (s *state) RunFailed(task, reason string) {
	s.finish(OutcomeFailed, task, reason)
}

func (s *state) finish(outcome, task, reason string) {
	s.update(func() {
		res := &Result{Outcome: outcome}
		if !s.started.IsZero() {
			res.DurationMS = time.Since(s.started).Milliseconds()
		}
		if outcome == OutcomeFailed {
			res.Label = s.labelOf(task)
			res.LastLine = s.step.lastLine
			if res.LastLine == "" {
				res.LastLine = reason
			}
			s.logf(task, "✗", reason)
		}
		res.Log = strings.Join(s.logLines, "\n")
		s.result = res
	})
}

func (s *state) labelOf(task string) string {
	for i := len(s.setup) - 1; i >= 0; i-- {
		if s.setup[i].name == task {
			return s.setup[i].label
		}
	}
	return task
}

// startPick clears the last run and shows the task picker.
func (s *state) startPick(tasks []Task, offerHome bool) {
	s.update(func() {
		s.task, s.setup, s.running, s.step = Task{}, nil, nil, stepState{}
		s.started, s.result, s.logLines = time.Time{}, nil, nil
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
	snap.Progress = progress
	return snap, s.version
}
