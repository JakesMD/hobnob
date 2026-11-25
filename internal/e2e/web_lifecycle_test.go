package e2e

import (
	"net/http"
	"testing"
	"time"

	"hobnob/internal/web"
)

const promptYml = `
	tasks:
	  t:
	    steps:
	      - get:
	          - NAME:
	              info: Your name?
	      - run: echo hello={{.NAME}}
`

func TestE2E_Web_NewestTabWinsAndTheOldOneIsToldAndClosed(t *testing.T) {
	// given an open tab and a run at a prompt, when a second tab connects,
	// then it gets the live state and the first gets another-tab and is
	// closed (why: one page at a time drives the run)
	// Arrange
	var live, replaced web.Snapshot
	var closed bool
	c := Case{Yml: promptYml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		first := b.Open()
		asked := first.Screen(web.ScreenPrompt)
		second := b.Open()
		live = second.Screen(web.ScreenPrompt)
		replaced = first.Screen(web.ScreenAnotherTab)
		closed = first.Closed()
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "Ada"})
		second.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if live.Prompt == nil || live.Prompt.Info != "Your name?" || replaced.Screen != web.ScreenAnotherTab || !closed {
		t.Errorf("live = %+v, replaced = %+v, closed = %v", live, replaced, closed)
	}
}

func TestE2E_Web_ReconnectingResumesTheCurrentScreen(t *testing.T) {
	// given a tab that answered some prompts and was then closed, when a tab
	// opens again, then it shows the prompt the run is at, with the answers
	// so far on the rail (why: snapshots carry the whole state)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - FIRST:
		              info: First?
		          - SECOND:
		              info: Second?
	`
	var resumed web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		first := tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": first.Prompt.ID, "value": "one"})
		tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Info == "Second?" })
		tab.Close()
		again := b.Open()
		resumed = again.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": resumed.Prompt.ID, "value": "two"})
		again.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	want := []web.RailEntry{{Label: "First?", Value: "one"}}
	if resumed.Prompt.Info != "Second?" || !equalRail(resumed.Rail.Answers, want) {
		t.Errorf("resumed = %+v", resumed)
	}
}

func TestE2E_Web_NoTabAtTheTaskPickerExitsAfterTheGrace(t *testing.T) {
	// given the task picker and no page ever opened, when the grace runs
	// out, then hobnob exits with 0 (why: nobody is there to choose)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(*Browser) {}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
}

func TestE2E_Web_NoTabAtDoneExitsAfterTheGrace(t *testing.T) {
	// given a run that ended with no page open, when the grace runs out,
	// then hobnob exits with the run's outcome (why: the timer starts when
	// the run ends)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: exit 3
	`
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(*Browser) {}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
}

func TestE2E_Web_NoTabAtAPromptCancelsTheRunAfterTheGraceAsInterrupted(t *testing.T) {
	// given a run at a prompt and no page open, when the grace runs out,
	// then the run is cancelled as a first CTRL+C does and the command
	// after the prompt never runs (why: nobody can answer)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	c := Case{Yml: promptYml, Args: []string{"--web", "t"}, Browser: func(*Browser) {}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	res.NotOut(t, "hello=")
}

func TestE2E_Web_NothingExitsWhileStepsRunAndTheTimerStartsWhenTheRunEnds(t *testing.T) {
	// given a step that outlasts the grace with no page open, when it ends,
	// then a page opening right after still gets Done and hobnob exits after
	// a grace of its own (why: the timer runs only while hobnob waits on the
	// person)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: while [ ! -e go ]; do :; done; echo finished
	`
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		time.Sleep(400 * time.Millisecond)
		b.Release("go")
		b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ finished")
}

func TestE2E_Web_AReconnectingTabStopsTheTimer(t *testing.T) {
	// given a run at a prompt, when a tab is open past the grace, then the
	// run is still waiting and takes the answer (why: an open tab means the
	// person is there)
	// Arrange
	defer web.SetGrace(200 * time.Millisecond)()
	c := Case{Yml: promptYml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		time.Sleep(500 * time.Millisecond)
		if status := b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "Ada"}); status != http.StatusNoContent {
			b.Fail("answer status = %d", status)
		}
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ hello=Ada")
}

func TestE2E_Web_ClosingTheLastTabAtAPromptStartsTheTimer(t *testing.T) {
	// given a run at a prompt, when its only tab closes, then the grace
	// cancels the run (why: a tab counts as open while its connection is)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	c := Case{Yml: promptYml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		tab.Screen(web.ScreenPrompt)
		tab.Close()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	res.NotOut(t, "hello=")
}

func TestE2E_Web_NoTabAtDoneInTheHomeFlowExitsAfterTheGraceWithTheRunsOutcome(t *testing.T) {
	// given a run chosen from the picker that ended on Done, when the tab
	// closes and the grace runs out, then hobnob exits with the run's
	// outcome (why: nobody is left to press Home)
	// Arrange
	defer web.SetGrace(100 * time.Millisecond)()
	c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(b *Browser) {
		tab := b.Open()
		tab.Screen(web.ScreenTasks)
		b.Post("choose", map[string]string{"task": "broken"})
		tab.Screen(web.ScreenFailed)
		tab.Close()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
}
