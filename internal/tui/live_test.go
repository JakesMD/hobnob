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
	// and the echoed answer continues the prompt's line (why: the cursor must
	// stay at the prompt's end, so the rule can't be redrawn under it)

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
	stdout.Write([]byte("bob\r\n")) // the PTY echoes the answer
	stdout.Write([]byte("hi\n"))
	rig.advance(time.Second)

	// Assert
	got = rig.screen()
	if got[1] != "t │ name? bob" || got[2] != "t │ hi" {
		t.Errorf("screen after the answer:\n%s", strings.Join(got, "\n"))
	}
}

func TestLiveStatusLineHidesWhileAToolRedrawsRowsAndReturnsWhenItStops(t *testing.T) {
	// given a command that moves the cursor between rows, when it draws, then
	// the rule is gone while it does, the lines before the move keep their
	// prefix, and once it has moved the cursor no more the rule and prefixes
	// are back, before the step ends (why: a tool drawing its own display
	// would fight the rule for the bottom row, but a long step must not stay
	// bare)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")
	rig.log.Run("t", "tool")
	stdout, _ := rig.log.Output("t")

	// Act
	stdout.Write([]byte("a\nb\n\x1b[2Ac\n"))
	rig.advance(drawIdle / 2)
	during := strings.Join(rig.screen(), "\n")
	rig.now = rig.now.Add(drawIdle)
	stdout.Write([]byte("after\n"))
	rig.advance(time.Millisecond)
	later := rig.screen()

	// Assert
	if strings.Contains(during, "──") {
		t.Errorf("rule shown while the tool draws:\n%s", during)
	}
	if !strings.Contains(rig.out.String(), "t │ a\nt │ b\n") {
		t.Errorf("lines before the move lost their prefix: %q", rig.out.String())
	}
	if !strings.Contains(later[len(later)-1], "total") || later[len(later)-3] != "t │ after" {
		t.Errorf("prefix or rule missing after the tool stopped:\n%s", strings.Join(later, "\n"))
	}
}

func TestLiveStatusLineComesBackOnItsOwnWhenAToolGoesQuiet(t *testing.T) {
	// given a tool that moved the cursor and then stopped writing, when time
	// passes, then the rule is back without more output (why: a step that
	// goes quiet must not leave the bottom row bare)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")
	rig.log.Run("t", "tool")
	stdout, _ := rig.log.Output("t")
	stdout.Write([]byte("a\n\x1b[1Ab\n"))

	// Act
	rig.advance(drawIdle + time.Millisecond)

	// Assert
	got := rig.screen()
	if !strings.Contains(got[len(got)-1], "total") {
		t.Errorf("rule missing:\n%s", strings.Join(got, "\n"))
	}
}

func TestLiveWatchedOutputCostsAFewTimesItsSizeInTerminalWrites(t *testing.T) {
	// given a verbose command, when it writes many 80-character lines in the
	// chunks a PTY delivers, then the terminal gets a small multiple of that
	// text (why: a redraw per grapheme flooded the terminal and ssh links)

	// Arrange
	lipgloss.SetColorProfile(termenv.Ascii)
	var out countingWriter
	var errs bytes.Buffer
	log := NewRunLog(&out, &errs)
	log.Watch(&Screen{
		Size:     func() (int, int) { return 100, 24 },
		OnResize: func(func()) func() { return func() {} },
	})
	log.live.tick = 0
	log.Enter("t")
	log.Run("t", "tool")
	stdout, _ := log.Output("t")
	chunk := []byte(strings.Repeat(strings.Repeat("a", 80)+"\n", 50))
	before := out.bytes

	// Act
	for range 20 {
		stdout.Write(chunk)
	}

	// Assert
	input := 20 * len(chunk)
	if got := out.bytes - before; got > 3*input {
		t.Errorf("%d bytes written for %d bytes of output", got, input)
	}
}

func TestLiveLateWriteFromAnEarlierStepUsesItsOwnPrefix(t *testing.T) {
	// given a job a step backgrounded, when a later step starts and the job
	// then writes after its own line was ended, then it takes its own step's
	// prefix (why: a late write must not pick up the next step's writers;
	// under -race this also checks the shared state is locked)

	// Arrange
	rig := newLiveRig(t, 60)
	rig.log.Enter("a")
	rig.log.Run("a", "bg")
	lateOut, _ := rig.log.Output("a")
	lateOut.Write([]byte("early"))
	rig.log.StepDone("a", time.Second)
	rig.log.Run("b", "next")

	// Act
	done := make(chan struct{})
	go func() {
		defer close(done)
		lateOut.Write([]byte(" late\n"))
	}()
	rig.log.Output("b")
	<-done

	// Assert
	if got := rig.out.String(); !strings.Contains(got, "a │  late") {
		t.Errorf("late output lost its step's prefix: %q", got)
	}
}

func TestClearStatusLineLeavesNoRuleBehind(t *testing.T) {
	// given a watched run, when the status line is cleared as on a forced
	// exit, then no rule is left (why: the shell prompt must not land on it)

	// Arrange
	rig := newLiveRig(t, 40)
	rig.log.Enter("t")
	rig.log.Run("t", "sleep 9")
	rig.log.Stopping()

	// Act
	ClearStatusLine()

	// Assert
	for _, row := range rig.screen() {
		if strings.Contains(row, "──") {
			t.Errorf("rule left on screen: %q", row)
		}
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
