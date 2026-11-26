package app

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"hobnob/internal/config"
	"hobnob/internal/runevents"
	"hobnob/internal/runner"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

type outcomeRecorder struct {
	runevents.Nop
	outcomes *[]runevents.Outcome
}

func (o outcomeRecorder) Finished(out runevents.Outcome) { *o.outcomes = append(*o.outcomes, out) }

// fakeFrontEnd records what dispatch asks of a front end.
type fakeFrontEnd struct {
	picks       []string
	pickErr     error
	alwaysPicks bool
	interact    bool
	beginErr    error
	again       []bool

	picksAsked int
	waits      int
	begun      []string
	picked     []bool
	outcomes   []runevents.Outcome
}

func (f *fakeFrontEnd) AlwaysPicks() bool { return f.alwaysPicks }

func (f *fakeFrontEnd) CanPick(bool) bool { return f.interact }

func (f *fakeFrontEnd) Pick(_ context.Context, _ []tui.TaskItem, _ bool) (string, bool, error) {
	f.picksAsked++
	if f.pickErr != nil {
		return "", false, f.pickErr
	}
	if len(f.picks) == 0 {
		return "", false, nil
	}
	name := f.picks[0]
	f.picks = f.picks[1:]
	return name, true, nil
}

func (f *fakeFrontEnd) Begin(ctx context.Context, cfg *config.ConfigFile, name string, picked bool) (frontRun, error) {
	f.begun = append(f.begun, name)
	f.picked = append(f.picked, picked)
	if f.beginErr != nil {
		return frontRun{}, f.beginErr
	}
	return frontRun{
		Ctx:     ctx,
		Events:  runevents.New(outcomeRecorder{outcomes: &f.outcomes}),
		Log:     tui.NewRunLog(os.Stdout, os.Stderr),
		Prompts: runner.TerminalPrompts(),
		Done: func(context.Context) bool {
			f.waits++
			again := len(f.again) > 0 && f.again[0]
			if len(f.again) > 0 {
				f.again = f.again[1:]
			}
			return again
		},
	}, nil
}

func (f *fakeFrontEnd) Close() {}

func dispatchConfig() *config.ConfigFile {
	return &config.ConfigFile{
		TaskfileDir: ".",
		TaskNames:   []string{"build", "test"},
		Tasks: map[string]config.Task{
			"build": {Info: "Build it"},
			"test":  {Info: "Test it"},
		},
	}
}

func TestSelectAndRunPicksRunsThenWaits(t *testing.T) {
	// given a front end that picks "build" once, when selectAndRun dispatches,
	// then that task runs once as a picked task and the front end is waited on once
	// (why: the after-run wait is once per pick)
	// Arrange
	fe := &fakeFrontEnd{interact: true, picks: []string{"build"}}
	cfg := dispatchConfig()
	app := &App{}

	// Act
	err := app.selectAndRun(context.Background(), fe, taskfile{scope: scope.New(), cfg: cfg}, false)

	// Assert
	if err != nil {
		t.Fatalf("selectAndRun: %v", err)
	}
	if fe.waits != 1 {
		t.Errorf("waits = %d, want 1", fe.waits)
	}
	if want := []string{"build"}; !reflect.DeepEqual(fe.begun, want) || !fe.picked[0] {
		t.Errorf("begun %v picked %v, want build picked", fe.begun, fe.picked)
	}
	if len(fe.outcomes) != 1 {
		t.Errorf("outcomes = %+v, want exactly one run", fe.outcomes)
	}
}

func TestSelectAndRunPicksAgainWhileTheWaitSaysSo(t *testing.T) {
	// given a front end whose wait asks for another pick once, when
	// selectAndRun dispatches, then it runs both picks
	// (why: the browser's Home returns to the picker for as long as the person keeps choosing)
	// Arrange
	fe := &fakeFrontEnd{interact: true, picks: []string{"build", "test"}, again: []bool{true, false}}

	// Act
	err := (&App{}).selectAndRun(context.Background(), fe, taskfile{scope: scope.New(), cfg: dispatchConfig()}, false)

	// Assert
	if err != nil {
		t.Fatalf("selectAndRun: %v", err)
	}
	if want := []string{"build", "test"}; !reflect.DeepEqual(fe.begun, want) {
		t.Errorf("begun = %v, want %v", fe.begun, want)
	}
}

func TestSelectAndRunReturnsTheLastRunsFailureWhenThePickerEnds(t *testing.T) {
	// given a front end that picks a failing task then offers nothing more,
	// when selectAndRun dispatches, then it returns the failure
	// (why: the exit code is the last run's, as the page's Home loop always had)
	// Arrange
	fe := &fakeFrontEnd{interact: true, picks: []string{"missing"}, again: []bool{true}}

	// Act
	err := (&App{}).selectAndRun(context.Background(), fe, taskfile{scope: scope.New(), cfg: dispatchConfig()}, false)

	// Assert
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v, want ErrSilent", err)
	}
}

func TestSelectAndRunReturnsAPickError(t *testing.T) {
	// given a front end whose pick fails, when selectAndRun dispatches, then
	// nothing runs and the error comes back
	// (why: an aborted picker must not start a task)
	// Arrange
	boom := errors.New("aborted")
	fe := &fakeFrontEnd{interact: true, pickErr: boom}

	// Act
	err := (&App{}).selectAndRun(context.Background(), fe, taskfile{scope: scope.New(), cfg: dispatchConfig()}, false)

	// Assert
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
	if len(fe.begun) != 0 {
		t.Errorf("begun = %v, want none", fe.begun)
	}
}

func TestSelectAndRunListsWhenTheFrontEndCannotPick(t *testing.T) {
	// given a front end that cannot pick, when selectAndRun dispatches, then
	// the task list is printed and the front end is never asked to pick
	// (why: piped or --no-input runs must list instead of blocking on a picker)
	// Arrange
	fe := &fakeFrontEnd{interact: false}
	var err error

	// Act
	out := captureStdout(t, func() {
		err = (&App{Version: "dev"}).selectAndRun(context.Background(), fe, taskfile{scope: scope.New(), cfg: dispatchConfig(), noPrompts: true}, false)
	})

	// Assert
	if err != nil {
		t.Fatalf("selectAndRun: %v", err)
	}
	if fe.picksAsked != 0 || len(fe.begun) != 0 {
		t.Errorf("picks asked %d, begun %v, want neither", fe.picksAsked, fe.begun)
	}
	if !strings.Contains(out, "build") || !strings.Contains(out, "test") {
		t.Errorf("output %q lacks the task list", out)
	}
}

func TestExecTaskRunsANamedTaskOnceAndWaits(t *testing.T) {
	// given a named task, when execTask dispatches, then it begins as not
	// picked, runs, waits once and does not pick
	// (why: a named task never opens the picker)
	// Arrange
	fe := &fakeFrontEnd{again: []bool{true}}

	// Act
	err := (&App{}).execTask(context.Background(), fe, "build", taskfile{scope: scope.New(), cfg: dispatchConfig()})

	// Assert
	if err != nil {
		t.Fatalf("execTask: %v", err)
	}
	if want := []string{"build"}; !reflect.DeepEqual(fe.begun, want) || fe.picked[0] {
		t.Errorf("begun %v picked %v, want build named", fe.begun, fe.picked)
	}
	if fe.picksAsked != 0 || fe.waits != 1 || len(fe.outcomes) != 1 {
		t.Errorf("picks asked %d, waits %d, outcomes %d, want 0, 1, 1", fe.picksAsked, fe.waits, len(fe.outcomes))
	}
}

func TestExecTaskFailsSilentlyWhenTheFrontEndRefusesTheTask(t *testing.T) {
	// given a front end that refuses to begin, when execTask dispatches, then
	// the run never starts and the failure is ErrSilent
	// (why: a refused task, such as one named under --port, must not run)
	// Arrange
	fe := &fakeFrontEnd{beginErr: errors.New("refused")}

	// Act
	err := (&App{}).execTask(context.Background(), fe, "build", taskfile{scope: scope.New(), cfg: dispatchConfig()})

	// Assert
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v, want ErrSilent", err)
	}
	if len(fe.outcomes) != 0 {
		t.Errorf("outcomes = %v, want no run", fe.outcomes)
	}
}

func TestExecTaskReportsAFailedRunAndStillWaits(t *testing.T) {
	// given an unknown task, when execTask dispatches, then the run fails
	// silently and the wait still happens
	// (why: the page must still be told how a failed run ended)
	// Arrange
	fe := &fakeFrontEnd{}

	// Act
	err := (&App{}).execTask(context.Background(), fe, "nope", taskfile{scope: scope.New(), cfg: dispatchConfig()})

	// Assert
	if !errors.Is(err, ErrSilent) {
		t.Errorf("err = %v, want ErrSilent", err)
	}
	if fe.waits != 1 || len(fe.outcomes) != 1 || fe.outcomes[0].Kind != runevents.Failed {
		t.Errorf("waits %d, outcomes %+v, want one wait and one failed outcome", fe.waits, fe.outcomes)
	}
}

func TestPortNeedsBrowser(t *testing.T) {
	// given --port without --browser, when the front end is chosen, then it is an error
	// (why: a port only means something to the browser front end)
	// Arrange
	app := &App{}

	// Act
	_, err := app.frontEnd(false, 8080, false, nil)

	// Assert
	if err == nil || err.Error() != "--port needs --browser" {
		t.Errorf("err = %v, want --port needs --browser", err)
	}
}

func TestBrowserFrontEndRefusesANamedTaskUnderPort(t *testing.T) {
	// given the browser front end on a fixed port, when a named task begins,
	// then it is refused before any page opens
	// (why: with --port the page is always the picker)
	// Arrange
	fe, err := (&App{}).frontEnd(true, 8080, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fe.Close()

	// Act
	_, err = fe.Begin(context.Background(), dispatchConfig(), "build", false)

	// Assert
	if err == nil {
		t.Error("named task under --port was accepted")
	}
}

func TestBrowserFrontEndRefusesFlagsItCannotBeCombinedWith(t *testing.T) {
	// given --browser beside --no-input, when the front end is chosen, then it is refused
	// (why: the page has nothing to do without prompts)
	// Arrange
	app := &App{}

	// Act
	_, err := app.frontEnd(true, 0, false, []string{"build", "--no-input"})

	// Assert
	if err == nil || err.Error() != "--browser can't be combined with --no-input" {
		t.Errorf("err = %v, want --browser can't be combined with --no-input", err)
	}
}
