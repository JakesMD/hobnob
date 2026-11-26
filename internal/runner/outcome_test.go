package runner

import (
	"context"
	"os"
	"testing"

	"hobnob/internal/config"
	"hobnob/internal/runevents"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

type outcomeListener struct {
	runevents.Nop
	outcome *runevents.Outcome
	called  []string
}

func (o *outcomeListener) Finished(out runevents.Outcome) { o.outcome = &out }
func (o *outcomeListener) Call(task, called string)       { o.called = append(o.called, task+"->"+called) }

func runWithListener(t *testing.T, cfg *config.ConfigFile, task string) (*outcomeListener, error) {
	t.Helper()
	listener := &outcomeListener{}
	err := ExecuteTask(context.Background(), task, scope.New(), nil, cfg, true, t.TempDir(), runevents.New(listener), tui.NewRunLog(os.Stdout, os.Stderr), TerminalPrompts())
	return listener, err
}

func TestRunOutcomeNamesTheCalledTaskWhoseStepFailed(t *testing.T) {
	// given a task calling a child whose command fails, when the run ends,
	// then the outcome is Failed and names the child with its exit reason
	// (why: the page and any other front end report the failing task, not
	// the one the person started)
	// Arrange
	cfg := &config.ConfigFile{Tasks: map[string]config.Task{
		"t":     {Steps: []config.Step{{Kind: config.KindCall, CallTarget: "child"}}},
		"child": {Info: "Child task", Steps: []config.Step{{Kind: config.KindRun, Command: shellCommand(t, "exit 3")}}},
	}}

	// Act
	listener, err := runWithListener(t, cfg, "t")

	// Assert
	if err == nil || listener.outcome == nil {
		t.Fatalf("err = %v, outcome = %v, want a failed run with an outcome", err, listener.outcome)
	}
	if got := *listener.outcome; got.Kind != runevents.Failed || got.Task != "child" || got.Label != "Child task" || got.Reason == "" {
		t.Errorf("outcome = %+v, want Failed naming child", got)
	}
	if len(listener.called) != 1 || listener.called[0] != "t->child" {
		t.Errorf("calls = %v, want [t->child]", listener.called)
	}
}

func TestRunOutcomeIgnoresASoftFailedStep(t *testing.T) {
	// given a soft: step that fails, when the run succeeds, then the outcome
	// is Succeeded (why: a forgiven failure must not end the run)
	// Arrange
	cfg := &config.ConfigFile{Tasks: map[string]config.Task{
		"t": {Steps: []config.Step{{Kind: config.KindRun, Command: shellCommand(t, "exit 1"), Soft: true}}},
	}}

	// Act
	listener, err := runWithListener(t, cfg, "t")

	// Assert
	if err != nil || listener.outcome == nil || listener.outcome.Kind != runevents.Succeeded {
		t.Errorf("err = %v, outcome = %+v, want Succeeded", err, listener.outcome)
	}
}

func TestRunOutcomeForAnUnknownTaskCarriesOnlyTheReason(t *testing.T) {
	// given a task name the config doesn't have, when run, then the outcome
	// is Failed with the reason and no task (why: nothing ran to name)
	// Arrange
	cfg := &config.ConfigFile{Tasks: map[string]config.Task{}}

	// Act
	listener, _ := runWithListener(t, cfg, "nope")

	// Assert
	want := runevents.Outcome{Kind: runevents.Failed, Reason: `no task "nope"`}
	if listener.outcome == nil || *listener.outcome != want {
		t.Errorf("outcome = %+v, want %+v", listener.outcome, want)
	}
}
