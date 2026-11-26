package runevents

import (
	"errors"
	"testing"
	"time"
)

type recorder struct {
	Nop
	outcomes []Outcome
	left     []string
	stopped  []string
	lines    []string
}

func (r *recorder) TaskLeft(name string, failed bool) {
	if failed {
		name += " (failed)"
	}
	r.left = append(r.left, name)
}
func (r *recorder) Stopping(task string)         { r.stopped = append(r.stopped, task) }
func (r *recorder) OutputLine(task, line string) { r.lines = append(r.lines, task+": "+line) }
func (r *recorder) Finished(o Outcome)           { r.outcomes = append(r.outcomes, o) }

func finish(t *testing.T, s *Stream, r *recorder, err error, cancelled bool) Outcome {
	t.Helper()
	s.Finish(err, cancelled)
	if len(r.outcomes) != 1 {
		t.Fatalf("got %d outcomes, want 1", len(r.outcomes))
	}
	return r.outcomes[0]
}

func TestFinishNamesTheStepThatFailed(t *testing.T) {
	// given a task whose step failed inside a nested call, when the run
	// ends in error, then the outcome names that step's task and reason
	// (why: Failed names the failure a person sees, not the outermost task)
	// Arrange
	r := &recorder{}
	s := New(r)
	leaveOuter := s.Enter("outer", "Do everything")
	leaveInner := s.Enter("inner", "Install tools")
	s.StepFailed("inner", time.Second, "make", "exit status 2")
	s.TaskFailed("inner", time.Second, false)
	leaveInner()
	s.TaskFailed("outer", time.Second, false)
	leaveOuter()

	// Act
	got := finish(t, s, r, errors.New("exit status 2"), false)

	// Assert
	want := Outcome{Kind: Failed, Task: "inner", Label: "Install tools", Reason: "exit status 2"}
	if got != want {
		t.Errorf("outcome = %+v, want %+v", got, want)
	}
}

func TestFinishIgnoresAFailureASoftStepForgave(t *testing.T) {
	// given a failed step followed by a soft-failed one, when the run ends
	// in error, then the outcome names neither, falling back to the
	// innermost task (why: a forgiven failure did not end the run)
	// Arrange
	r := &recorder{}
	s := New(r)
	leave := s.Enter("t", "")
	s.StepFailed("t", time.Second, "a", "boom")
	s.StepSoftFailed("t", time.Second, "b", "meh")
	leave()

	// Act
	got := finish(t, s, r, errors.New("late"), false)

	// Assert
	if got.Reason != "late" || got.Task != "t" {
		t.Errorf("outcome = %+v, want reason %q on task t", got, "late")
	}
}

func TestFinishOutsideAStepNamesTheInnermostTaskStillRunning(t *testing.T) {
	// given two nested tasks and an error logged against no step, when the
	// run ends in error, then the outcome names the inner task
	// (why: Failed names the innermost task running, so it is never blank)
	// Arrange
	r := &recorder{}
	s := New(r)
	s.Enter("outer", "Do everything")
	s.Enter("inner", "Install tools")

	// Act
	got := finish(t, s, r, errors.New("boom"), false)

	// Assert
	want := Outcome{Kind: Failed, Task: "inner", Label: "Install tools", Reason: "boom"}
	if got != want {
		t.Errorf("outcome = %+v, want %+v", got, want)
	}
}

func TestFinishFallsBackToTheRunsTaskOnceEverythingLeftCleanly(t *testing.T) {
	// given a task that already left, when the run ends in error, then the
	// outcome names the run's first task (why: a blank name tells nobody
	// anything)
	// Arrange
	r := &recorder{}
	s := New(r)
	s.Enter("deploy", "")()

	// Act
	got := finish(t, s, r, errors.New("boom"), false)

	// Assert
	if got.Task != "deploy" || got.Label != "deploy" {
		t.Errorf("outcome = %+v, want task and label deploy", got)
	}
}

func TestFinishWithNoTaskEnteredStillFails(t *testing.T) {
	// given a run that failed before entering any task, when it ends, then
	// the outcome carries only the reason (why: nothing ran to name)
	// Arrange
	r := &recorder{}
	s := New(r)

	// Act
	got := finish(t, s, r, errors.New("no such task"), false)

	// Assert
	want := Outcome{Kind: Failed, Reason: "no such task"}
	if got != want {
		t.Errorf("outcome = %+v, want %+v", got, want)
	}
}

func TestFinishClassifiesSuccessAndCancel(t *testing.T) {
	// given a run, when it ends cleanly or cancelled, then the outcome says so (why: a front end shows each differently)
	tests := []struct {
		name      string
		err       error
		cancelled bool
		want      Kind
	}{
		{"nil error", nil, false, Succeeded},
		{"cancelled", errors.New("interrupted"), true, Cancelled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			r := &recorder{}
			s := New(r)

			// Act
			got := finish(t, s, r, tt.err, tt.cancelled)

			// Assert
			if got.Kind != tt.want {
				t.Errorf("kind = %v, want %v", got.Kind, tt.want)
			}
		})
	}
}

func TestTaskLeftSaysWhetherItsTaskFailed(t *testing.T) {
	// given a failed task nested in a clean one, when both leave, then only
	// the failed one is reported failed (why: the rail marks a task failed)
	// Arrange
	r := &recorder{}
	s := New(r)
	leaveOuter := s.Enter("outer", "")
	leaveInner := s.Enter("inner", "")

	// Act
	s.TaskFailed("inner", time.Second, false)
	leaveInner()
	leaveOuter()

	// Assert
	if got := r.left; len(got) != 2 || got[0] != "inner (failed)" || got[1] != "outer" {
		t.Errorf("left = %v, want [inner (failed) outer]", got)
	}
}

func TestStoppingReportsOnlyTheFirstInterruptOnTheInnermostTask(t *testing.T) {
	// given nested tasks, when Stopping is called twice, then the listener
	// hears one interrupt naming the inner task (why: every path that
	// notices CTRL+C calls it)
	// Arrange
	r := &recorder{}
	s := New(r)
	s.Enter("outer", "")
	s.Enter("inner", "")

	// Act
	s.Stopping()
	s.Stopping()

	// Assert
	if len(r.stopped) != 1 || r.stopped[0] != "inner" {
		t.Errorf("stopped = %v, want [inner]", r.stopped)
	}
}

func TestStoppingWithNoTaskRunningSaysNothing(t *testing.T) {
	// given no running task, when Stopping is called, then nobody hears it (why: there is no task to name)
	// Arrange
	r := &recorder{}
	s := New(r)

	// Act
	s.Stopping()

	// Assert
	if len(r.stopped) != 0 {
		t.Errorf("stopped = %v, want none", r.stopped)
	}
}

func TestTapPassesOutputOnAndReportsEachMaskedLine(t *testing.T) {
	// given a tap over a destination, when output with a progress bar and an
	// unfinished last line is written, then the destination gets every byte
	// and the listener each state, masked and ANSI-stripped
	// (why: a redrawing progress bar reports each state)
	// Arrange
	r := &recorder{}
	s := New(r)
	var dest sink
	tap := s.Tap("t", &dest, func(line string) string { return line[:1] + "*" })

	// Act
	_, _ = tap.Write([]byte("a1\n\x1b[1mb2\rc3"))
	tap.Flush()

	// Assert
	if dest.n != len("a1\n\x1b[1mb2\rc3") {
		t.Errorf("destination got %d bytes", dest.n)
	}
	want := []string{"t: a*", "t: b*", "t: c*"}
	if len(r.lines) != 3 || r.lines[0] != want[0] || r.lines[1] != want[1] || r.lines[2] != want[2] {
		t.Errorf("lines = %v, want %v", r.lines, want)
	}
}

type sink struct{ n int }

func (s *sink) Write(p []byte) (int, error) { s.n += len(p); return len(p), nil }
