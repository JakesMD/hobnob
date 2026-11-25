package e2e

import (
	"regexp"
	"testing"
)

// secs is how a time reads in an end line.
const secs = `[0-9]+\.[0-9]s`

func (r *Result) streamMatches(t *testing.T, stream, text, pattern string) {
	t.Helper()
	if !regexp.MustCompile(pattern).MatchString(text) {
		t.Errorf("expected %s to match %q\n%s:\n%s", stream, pattern, stream, text)
	}
}

func TestE2E_Log_StepAndTaskEndWithTheirOwnLines(t *testing.T) {
	// given a task with one run: step, when it succeeds, then the step ends
	// with "task ✓ time" and the task with "task ✓ done after time" (why: a
	// step's end and a task's end must not be confusable)
	// Arrange
	yml := `
		tasks:
		  setup:
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "setup")

	// Assert
	res.OK(t)
	res.streamMatches(t, "stdout", res.Stdout, `(?m)^setup │ hi\nsetup ✓ `+secs+`\nsetup ✓ done after `+secs+`\n$`)
}

func TestE2E_Log_FailureTwoCallsDeepEndsEveryTaskInnermostFirst(t *testing.T) {
	// given a failing step two calls deep, when the run fails, then the step
	// ends with ✗, its time and "cmd: reason", and every task in the chain
	// prints "failed after", innermost first (why: the chain shows how the
	// failure reached the top)
	// Arrange
	yml := `
		tasks:
		  top:
		    steps:
		      - call: mid
		  mid:
		    steps:
		      - call: leaf
		  leaf:
		    steps:
		      - run: exit 3
	`

	// Act
	res := Yml(t, yml, "top")

	// Assert
	res.Fails(t)
	res.streamMatches(t, "stderr", res.Stderr, `^leaf ✗ `+secs+` exit 3: exit status 3\nleaf ✗ failed after `+secs+`\nmid ✗ failed after `+secs+`\ntop ✗ failed after `+secs+`\n$`)
	res.NotOut(t, "done after")
}

func TestE2E_Log_SoftFailureEndsWithABangAndTheTaskCountsIt(t *testing.T) {
	// given a soft: true step that fails, when the task finishes, then the
	// step ends with "!" and no ✗, and the task's end counts the soft failure
	// (why: a soft failure must not read as a success or a failure)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - run: exit 1
		        soft: true
		      - run: exit 2
		        soft: true
		      - run: echo after
	`

	// Act
	res := Yml(t, yml, "deploy")

	// Assert
	res.OK(t)
	res.streamMatches(t, "stdout", res.Stdout, `deploy ! `+secs+` exit 1: exit status 1\ndeploy \$ exit 2\ndeploy ! `+secs+` exit 2: exit status 2\n`)
	res.streamMatches(t, "stdout", res.Stdout, `deploy ✓ done after `+secs+` • 2 soft failures\n$`)
	res.NotOut(t, "✗")
}

func TestE2E_Log_StepErrorWithoutARunEndsTheStepWithTheMessage(t *testing.T) {
	// given a call: to no task, when it runs, then the step ends with ✗ and
	// the message, no time, then the task's end (why: a failure the run
	// can't time must still say what went wrong)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - call: nope
	`

	// Act
	res := Yml(t, yml, "deploy")

	// Assert
	res.Fails(t)
	res.streamMatches(t, "stderr", res.Stderr, `^deploy ✗ .*nope.*\ndeploy ✗ failed after `+secs+`\n$`)
}
