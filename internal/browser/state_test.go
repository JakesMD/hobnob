package browser

import (
	"strings"
	"testing"
	"time"

	"hobnob/internal/runevents"
)

func TestDetailsLogCarriesEveryEventTheTerminalShows(t *testing.T) {
	// given a run whose events include a call, a say, a variable with its
	// source, a skipped step and step and task ends, when it finishes, then
	// the result's log holds a line for each (why: Copy details must say
	// what the terminal said)
	// Arrange
	s := &state{}
	events := runevents.New(s)
	leave := events.Enter("deploy", "")
	events.Call("deploy", "build")
	events.Say("deploy", "heads up")
	events.Var("deploy", "ENV", "prod", "from the environment")
	events.VarSkipped("deploy", "REPLICAS")
	events.StepSkipped("deploy", "go vet")
	events.StepStarted("deploy", "make", "in ./app")
	events.StepDone("deploy", 1600*time.Millisecond)
	events.DoneEarlier("build")
	events.TaskDone("deploy", 2*time.Second, 1)
	leave()

	// Act
	events.Finish(nil, false)

	// Assert
	want := []string{
		"deploy → build",
		"deploy » heads up",
		"deploy » ENV=prod # from the environment",
		"deploy ⊘ REPLICAS",
		"deploy ⊘ go vet",
		"deploy $ make # in ./app",
		"deploy ✓ 1.6s",
		"build ✓ done earlier",
		"deploy ✓ done after 2.0s • 1 soft failure",
	}
	if got := s.result.Log; got != strings.Join(want, "\n") {
		t.Errorf("log = %q, want %q", got, strings.Join(want, "\n"))
	}
}

func TestFailedResultIsLabelledWithTheNamedTaskAndLogsTheFailureOnce(t *testing.T) {
	// given a nested task whose step failed, when the run fails, then the
	// result carries the inner task's info and its log holds one failure line
	// for the step (why: the failure shows once, against the task that failed)
	// Arrange
	s := &state{}
	events := runevents.New(s)
	events.Enter("outer", "Do everything")
	events.Enter("inner", "Install tools")
	events.StepFailed("inner", time.Second, "make", "exit status 2")

	// Act
	events.Finish(errBoom{}, false)

	// Assert
	if got := s.result.Label; got != "Install tools" {
		t.Errorf("label = %q, want the inner task's info", got)
	}
	if got := strings.Count(s.result.Log, "✗"); got != 1 {
		t.Errorf("log has %d failure lines, want 1: %q", got, s.result.Log)
	}
}

type errBoom struct{}

func (errBoom) Error() string { return "boom" }
