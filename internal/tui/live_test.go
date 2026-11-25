package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"hobnob/internal/termsim"
)

type liveRig struct {
	log  *RunLog
	out  bytes.Buffer
	errs bytes.Buffer
	now  time.Time
}

func newLiveRig(t *testing.T, cols int) *liveRig {
	t.Helper()
	lipgloss.SetColorProfile(termenv.Ascii)
	rig := &liveRig{now: time.Unix(1000, 0)}
	rig.log = NewRunLog(&rig.out, &rig.errs)
	rig.log.Watch(&Screen{
		Size:     func() (int, int) { return cols, 24 },
		OnResize: func(func()) func() { return func() {} },
	})
	rig.log.live.now = func() time.Time { return rig.now }
	rig.log.live.tick = 0
	return rig
}

func (r *liveRig) advance(d time.Duration) {
	r.now = r.now.Add(d)
	r.log.live.redraw()
}

func (r *liveRig) screen() []string { return termsim.Render(r.out.String()) }

func TestLiveStatusLineSitsUnderTheLogWithTheStepAndTotalClocks(t *testing.T) {
	// given a watched run with a command running, when time passes, then the
	// bottom row is a rule with a blank row above it, holding the step and
	// total clocks on its right, and a line written to the log lands above
	// them (why: the clock is the only thing the rule says)

	// Arrange
	rig := newLiveRig(t, 60)
	rig.log.Enter("t")
	rig.log.Run("t", "sleep 1")
	stdout, _ := rig.log.Output("t")

	// Act
	rig.advance(1200 * time.Millisecond)
	stdout.Write([]byte("hello\n"))
	rig.advance(100 * time.Millisecond)

	// Assert
	got := rig.screen()
	want := []string{
		"t $ sleep 1",
		"t │ hello",
		"",
		"───────────────────────────────── step 1.3s · total 1.3s ──",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("screen:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLiveStatusLineBetweenStepsHasOnlyTheTotal(t *testing.T) {
	// given a watched run whose step has ended, when time passes, then the
	// rule shows only the total (why: no step is running)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")
	rig.log.Run("t", "true")

	// Act
	rig.advance(time.Second)
	rig.log.StepDone("t", time.Second)
	rig.advance(500 * time.Millisecond)

	// Assert
	got := rig.screen()
	if last := got[len(got)-1]; last != "───────────────────────── total 1.5s ──" {
		t.Errorf("rule = %q", last)
	}
	if got[len(got)-3] != "t ✓ 1.0s" {
		t.Errorf("screen:\n%s", strings.Join(got, "\n"))
	}
}

func TestLiveStatusLineSaysStoppingOnItsLeftAfterCtrlC(t *testing.T) {
	// given a watched run, when the first ctrl+c arrives, then the rule's
	// left says stopping and the log says it too (why: the rule shows the
	// shutdown is under way)

	// Arrange
	rig := newLiveRig(t, 70)
	rig.log.Enter("t")
	rig.log.Run("t", "sleep 9")

	// Act
	rig.log.Stopping()

	// Assert
	got := rig.screen()
	if want := "── stopping, ctrl+c again to force "; !strings.HasPrefix(got[len(got)-1], want) {
		t.Errorf("rule = %q, want prefix %q", got[len(got)-1], want)
	}
	if want := "t ! stopping, ctrl+c again to force"; got[1] != want {
		t.Errorf("log line = %q, want %q", got[1], want)
	}
}

func TestLiveStatusLineIsGoneWhenTheRunEnds(t *testing.T) {
	// given a watched run, when its last task is left, then no rule remains
	// (why: the shell prompt must not land under a stale clock)

	// Arrange
	rig := newLiveRig(t, 40)
	leave := rig.log.Enter("t")
	rig.log.TaskDone("t", time.Second, 0)

	// Act
	leave()

	// Assert
	for _, row := range rig.screen() {
		if strings.Contains(row, "──") {
			t.Errorf("rule left on screen: %q", row)
		}
	}
}

func TestLiveStatusLineIsClearedWhileACardPromptIsOpen(t *testing.T) {
	// given a watched run, when a card prompt is open, then the rule is
	// gone, and it is back once the prompt closes (why: the prompt owns the
	// bottom of the terminal)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")

	// Act
	resume := rig.log.Pause()
	open := rig.screen()
	resume()

	// Assert
	if len(open) != 0 {
		t.Errorf("screen while paused: %q", open)
	}
	if got := rig.screen(); len(got) != 2 || !strings.Contains(got[1], "total 0.0s") {
		t.Errorf("screen after resume: %q", got)
	}
}

func TestLiveStatusLineStaysUnderAPromptWaitingForInputWithItsClockFrozen(t *testing.T) {
	// given a command whose unfinished line has sat unchanged, when time
	// passes, then the rule stays below the prompt showing the clock it had,
	// and the answer's output starts a fresh prefixed line (why: hobnob
	// can't see the user's Enter, so it can't redraw under the prompt)

	// Arrange
	rig := newLiveRig(t, 50)
	rig.log.Enter("t")
	rig.log.Run("t", "read n")
	stdout, _ := rig.log.Output("t")
	stdout.Write([]byte("name? "))

	// Act
	rig.advance(300 * time.Millisecond)
	rig.advance(5 * time.Second)

	// Assert
	got := rig.screen()
	want := []string{"t $ read n", "t │ name?", "", "─────────────────────── step 0.3s · total 0.3s ──"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("screen:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Act
	rig.out.WriteString("\n") // the terminal echoes the user's Enter
	stdout.Write([]byte("hi\n"))
	rig.advance(time.Second)

	// Assert
	got = rig.screen()
	if got[1] != "t │ name?" || got[2] != "t │ hi" {
		t.Errorf("screen after the answer:\n%s", strings.Join(got, "\n"))
	}
}

func TestLiveStatusLineHidesWhileAToolRedrawsRows(t *testing.T) {
	// given a command that moves the cursor between rows, when it draws,
	// then the rule is gone and its bytes pass through unprefixed, and the
	// rule returns once the step ends (why: a tool drawing its own display
	// would fight the rule for the bottom row)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")
	rig.log.Run("t", "tool")
	stdout, _ := rig.log.Output("t")

	// Act
	stdout.Write([]byte("a\nb\n\x1b[1Ac\n"))
	rig.advance(time.Second)
	during := strings.Join(rig.screen(), "\n")
	rig.log.StepDone("t", time.Second)
	after := rig.screen()

	// Assert
	if strings.Contains(during, "──") {
		t.Errorf("rule shown while the tool draws:\n%s", during)
	}
	if !strings.Contains(after[len(after)-1], "total 1.0s") {
		t.Errorf("rule missing after the step:\n%s", strings.Join(after, "\n"))
	}
}

func TestLiveStatusLineNeverReachesAPipe(t *testing.T) {
	// given a run log nobody watches, when a run is logged, then nothing but
	// its plain lines is written (why: logs and CI get no rule and no escape
	// codes)

	// Arrange
	var out, errs bytes.Buffer
	log := NewRunLog(&out, &errs)

	// Act
	leave := log.Enter("t")
	log.Run("t", "echo hi")
	stdout, _ := log.Output("t")
	stdout.Write([]byte("hi\n"))
	log.StepDone("t", time.Second)
	leave()

	// Assert
	if strings.ContainsAny(out.String(), "\x1b\r") || strings.Contains(out.String(), "──") {
		t.Errorf("piped output has live-area furniture: %q", out.String())
	}
}
