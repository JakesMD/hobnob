package web

import "testing"

func TestRunFailedWithNoTaskNamesTheInnermostTaskRunning(t *testing.T) {
	// given two nested tasks and a failure logged against no step, when the
	// run fails, then the result is labelled with the inner task's info
	// (why: Failed names the innermost task running, so it is never blank)
	// Arrange
	s := &state{}
	s.TaskEntered("outer", "Do everything")
	s.TaskEntered("inner", "Install tools")

	// Act
	s.RunFailed("", "boom")

	// Assert
	if got := s.result.Label; got != "Install tools" {
		t.Errorf("label = %q, want the inner task's info", got)
	}
}

func TestRunFailedWithNoTaskFallsBackToTheRunsTask(t *testing.T) {
	// given a task that has already left, when the run fails with no task
	// named, then the result is labelled with the run's task name (why: a
	// blank label tells the person nothing)
	// Arrange
	s := &state{}
	s.TaskEntered("deploy", "")
	s.TaskLeft("deploy", false)

	// Act
	s.RunFailed("", "boom")

	// Assert
	if got := s.result.Label; got != "deploy" {
		t.Errorf("label = %q, want the task name", got)
	}
}
