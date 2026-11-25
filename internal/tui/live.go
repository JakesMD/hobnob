package tui

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	liveTick = 100 * time.Millisecond

	// promptIdle is how long a command's unfinished line must sit unchanged
	// before it counts as a prompt waiting for input.
	promptIdle = 200 * time.Millisecond

	stoppingNote = "stopping, ctrl+c again to force"
)

// liveArea is the bottom of a watched run's terminal: the command's
// unfinished line, a blank line and the status line, redrawn in place. The
// log above it is append-only. Every redraw reaches the terminal in one write.
type liveArea struct {
	mu     sync.Mutex
	out    io.Writer
	screen *Screen
	now    func() time.Time
	tick   time.Duration // 0 runs no clock: tests call redraw themselves

	// owePrefix: the command's open line was ended behind its line writers'
	// backs, so the next text needs its prefix, which they will not write
	// because they think the line is open. The writer that brings that text
	// supplies it (see liveWriter), so a late write from an earlier step never
	// takes a later step's.
	owePrefix bool

	frame bytes.Buffer

	active bool
	paused bool
	stop   chan struct{}

	runStart  time.Time
	stepStart time.Time
	inStep    bool
	note      string

	cur      []byte // the command's unfinished line
	last     []byte // the line a \r redrew over
	progress bool   // cur is a \r redraw
	lastOut  time.Time
	rows     int // rows of the live area on screen

	// hidden: a tool is redrawing several rows, so the area stays away and the
	// tool's bytes pass through untouched, until it has moved the cursor no
	// more for drawIdle or the step ends.
	hidden   bool
	hiddenNL bool      // the tool's last byte was a newline
	hiddenAt time.Time // when the tool last moved the cursor

	// waiting: the command's unfinished line is a prompt. The status line is
	// drawn once below it, its clock frozen, so the cursor can stay at the
	// prompt's end. The keys the user types are echoed by the command's PTY
	// as output, so the answer continues the prompt's line.
	waiting bool
	below   bool
	atStart bool
}

// running is the live area of the run on screen, for ClearStatusLine.
var running atomic.Pointer[liveArea]

// ClearStatusLine erases the status line of the run on screen, if any, so a
// forced exit leaves nothing of it in the scrollback.
func ClearStatusLine() {
	if a := running.Load(); a != nil {
		a.end()
	}
}

func newLiveArea(out io.Writer, screen *Screen) *liveArea {
	return &liveArea{out: out, screen: screen, now: time.Now, tick: liveTick}
}

func (a *liveArea) width() int {
	cols, _ := a.screen.Size()
	return cols
}

func (a *liveArea) emit(dst io.Writer, b []byte) {
	if dst == a.out {
		a.frame.Write(b)
		return
	}
	a.flush()
	dst.Write(b)
}

// flush writes the frame in one write, so the terminal never shows a cleared
// area waiting for its redraw.
func (a *liveArea) flush() {
	if a.frame.Len() > 0 {
		a.out.Write(a.frame.Bytes())
		a.frame.Reset()
	}
}

// begin starts the status line for a run.
func (a *liveArea) begin() {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.active, a.paused = true, false
	running.Store(a)
	a.runStart = a.now()
	a.stop = make(chan struct{})
	if a.tick > 0 {
		go a.runClock(a.stop)
	}
	a.draw()
}

func (a *liveArea) runClock(stop chan struct{}) {
	t := time.NewTicker(a.tick)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			a.redraw()
		}
	}
}

// end clears the status line for good.
func (a *liveArea) end() {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	if !a.active {
		return
	}
	a.clear()
	a.finishCur(a.out)
	a.active, a.hidden, a.waiting = false, false, false
	running.CompareAndSwap(a, nil)
	close(a.stop)
}

func (a *liveArea) stepBegin() {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.stepStart, a.inStep = a.now(), true
	a.redrawLocked()
}

// stepEnd runs before a step's end line is written.
func (a *liveArea) stepEnd() {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.clear()
	a.endWaiting()
	a.unhide()
	a.finishCur(a.out)
	a.inStep = false
	a.cur, a.last, a.progress = nil, nil, false
	a.draw()
}

func (a *liveArea) setNote(note string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.note = note
	a.redrawLocked()
}

// pause clears the status line while a card prompt is open.
func (a *liveArea) pause() (resume func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.clear()
	a.paused = true
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		defer a.flush()
		a.paused = false
		a.draw()
	}
}

// line writes one of hobnob's own lines, finishing the command's unfinished
// line first.
func (a *liveArea) line(dst io.Writer, text []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	if !a.active {
		dst.Write(text)
		return
	}
	a.clear()
	a.endWaiting()
	if a.hidden {
		a.endHiddenLine()
	}
	a.finishCur(dst)
	a.emit(dst, text)
	a.draw()
}

// endWaiting ends a waiting prompt's line, if there is one.
func (a *liveArea) endWaiting() {
	if !a.waiting {
		return
	}
	if !a.atStart {
		a.frame.WriteString("\n")
	}
	a.waiting, a.atStart = false, true
}

// endHiddenLine ends the line a tool left unfinished.
func (a *liveArea) endHiddenLine() {
	if !a.hiddenNL {
		a.frame.WriteString("\n")
		a.hiddenNL = true
	}
}

// unhide ends a tool's display so the area can come back.
func (a *liveArea) unhide() {
	if a.hidden {
		a.endHiddenLine()
		a.hidden = false
	}
}

// finishCur ends the command's unfinished line where it is.
func (a *liveArea) finishCur(dst io.Writer) {
	if len(a.cur) == 0 {
		return
	}
	a.emit(dst, append(a.cur, '\n'))
	a.cur, a.last, a.progress = nil, nil, false
	a.owePrefix = true
}

// write takes a command's output, as its line writers produce it.
func (a *liveArea) write(dst io.Writer, p []byte, restart func() []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	if !a.active || a.paused {
		dst.Write(p)
		return
	}
	a.lastOut = a.now()
	if a.hidden {
		if cursorMove.Match(p) {
			a.hiddenAt = a.lastOut
		}
		if a.lastOut.Sub(a.hiddenAt) <= drawIdle {
			a.hiddenNL = p[len(p)-1] == '\n'
			a.emit(dst, p)
			return
		}
		a.unhide()
	}
	a.clear()
	if a.waiting {
		if !a.atStart {
			i := bytes.IndexByte(p, '\n')
			if i < 0 {
				a.emit(dst, p)
				a.drawBelow()
				return
			}
			a.emit(dst, p[:i+1])
			p = p[i+1:]
			a.atStart = true
		}
		a.waiting = false
	}
	if cursorMove.Match(p) {
		a.finishCur(dst)
		a.hidden, a.hiddenNL, a.hiddenAt = true, p[len(p)-1] == '\n', a.lastOut
		a.emit(dst, p)
		return
	}
	if a.owePrefix {
		a.owePrefix = false
		if restart != nil {
			if prefix := restart(); !bytes.HasPrefix(p, prefix) {
				a.cur = append(a.cur[:0], prefix...)
			}
		}
	}
	var committed []byte
	for _, b := range p {
		switch b {
		case '\r':
			if len(a.cur) > 0 {
				a.last = a.cur
			}
			a.cur, a.progress = nil, true
		case '\n':
			line := a.cur
			if len(line) == 0 {
				line = a.last
			}
			committed = append(committed, line...)
			committed = append(committed, '\n')
			a.cur, a.last, a.progress = nil, nil, false
		default:
			a.cur = append(a.cur, b)
			a.last = nil
		}
	}
	if len(committed) > 0 {
		a.emit(dst, committed)
	}
	a.draw()
}

// redraw is the clock's tick: it redraws the area, or notices that the
// unfinished line is a prompt.
func (a *liveArea) redraw() {
	a.mu.Lock()
	defer a.mu.Unlock()
	defer a.flush()
	a.redrawLocked()
}

func (a *liveArea) redrawLocked() {
	if !a.active || a.paused || a.waiting {
		return
	}
	if a.hidden {
		if a.now().Sub(a.hiddenAt) <= drawIdle {
			return
		}
		a.unhide()
	}
	if len(a.cur) > 0 && !a.progress && a.now().Sub(a.lastOut) > promptIdle {
		a.clear()
		a.frame.Write(a.cur)
		a.cur, a.last = nil, nil
		a.waiting, a.atStart = true, false
		a.drawBelow()
		return
	}
	a.clear()
	a.draw()
}

func (a *liveArea) visible() bool {
	return a.active && !a.paused && !a.hidden && !a.waiting
}

func (a *liveArea) draw() {
	if !a.visible() {
		return
	}
	a.rows = 2
	if len(a.cur) > 0 {
		a.frame.Write(a.cur)
		a.frame.WriteString("\n")
		a.rows += (ansi.StringWidth(string(a.cur))-1)/max(a.width(), 1) + 1
	}
	a.frame.WriteString("\n" + a.rule())
}

// clear erases the area, leaving the cursor at the start of its first row.
func (a *liveArea) clear() {
	if a.below {
		a.frame.WriteString("\x1b[J")
		a.below = false
	}
	if a.rows == 0 {
		return
	}
	a.frame.WriteString("\r\x1b[K")
	for i := 1; i < a.rows; i++ {
		a.frame.WriteString("\x1b[A\r\x1b[K")
	}
	a.rows = 0
}

// drawBelow draws the blank line and the status line under a waiting prompt
// and puts the cursor back at the prompt's end. The rows exist already (the
// area was there), so nothing scrolls and the saved cursor stays right.
func (a *liveArea) drawBelow() {
	a.frame.WriteString("\x1b7\n\x1b[K\n\x1b[K" + a.rule() + "\x1b8")
	a.below = true
}

func (a *liveArea) rule() string {
	now := a.now()
	total := formatElapsed(now.Sub(a.runStart))
	right := "total " + total
	if a.inStep {
		right = "step " + formatElapsed(now.Sub(a.stepStart)) + " · " + right
	}
	var left string
	if a.note != "" {
		left = SStep.Render(a.note)
	}
	return ruleLine(left, SInfo.Render(right), a.width())
}

// ruleLine lays a grey rule across width-1 cells (the last one stays empty so
// the terminal never wraps): left after "── ", right before " ──", the gap
// filled with ─.
func ruleLine(left, right string, width int) string {
	head, tail := SInfo.Render("── "), SInfo.Render(" ──")
	if left == "" {
		head = SInfo.Render("─")
	} else {
		left += " "
	}
	right = " " + right
	if maxRight := width - 1 - lipgloss.Width(head) - lipgloss.Width(left) - lipgloss.Width(tail) - 2; lipgloss.Width(right) > maxRight {
		right = ansi.Truncate(right, max(0, maxRight), "…")
	}
	room := width - 1 - lipgloss.Width(head) - lipgloss.Width(right) - lipgloss.Width(tail)
	if lipgloss.Width(left) > room-1 {
		left = ansi.Truncate(left, max(0, room-2), "…") + " "
	}
	fill := SInfo.Render(strings.Repeat("─", max(1, room-lipgloss.Width(left))))
	return head + left + fill + right + tail
}

// liveWriter is one of the run's streams, writing through the live area.
type liveWriter struct {
	area *liveArea
	dst  io.Writer

	// restart resets the step's line writers and returns the prefix they owe.
	restart func() []byte
}

func (w liveWriter) Write(p []byte) (int, error) {
	w.area.write(w.dst, p, w.restart)
	return len(p), nil
}

// lineSink is one of hobnob's own streams: each write is whole lines.
type lineSink struct {
	area *liveArea
	dst  io.Writer
}

func (w lineSink) Write(p []byte) (int, error) {
	w.area.line(w.dst, p)
	return len(p), nil
}
