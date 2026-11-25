package tui

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// clearLine is the ANSI "erase from cursor to end of line" sequence, used to
// wipe leftover characters when a \r-redrawn line (e.g. rsync --progress) is
// shorter than the one it's overwriting.
const clearLine = "\033[K"

// minWrapWidth is the narrowest text column worth wrapping to; narrower
// terminals let the terminal wrap.
const minWrapWidth = 20

// cursorMove matches output that moves the cursor between rows, clears the
// screen or switches to the alternate screen: a tool drawing its own
// multi-line display.
var cursorMove = regexp.MustCompile(`\x1b\[[0-9;]*[ABEFHJf]|\x1b\[\?1049h|\x1b\[\?47h`)

// drawIdle is how long a tool must go without moving the cursor before its
// output is a plain log again: prefixed, with the status line back.
const drawIdle = 300 * time.Millisecond

// LineWriter prefixes a command's output line by line, writing text as it
// arrives so a prompt with no newline shows. Watched (see Watch), long lines
// also wrap under their own text, and a tool that moves the cursor to draw
// several rows gets the terminal to itself, unprefixed, until it stops. Each
// Write reaches out in one write.
type LineWriter struct {
	out     io.Writer
	prefix  string
	indent  string
	pending bytes.Buffer // what this Write or Flush sends to out

	watched  bool
	width    func() int       // watched: the terminal's width; 0 means no wrapping
	now      func() time.Time // watched: the clock for drawIdle
	col      int              // watched: visible width of the open line's text
	restart  atomic.Bool      // Restart was called since col was last set
	drawing  bool             // watched: a tool is drawing its own display
	lastMove time.Time        // watched: when the tool last moved the cursor

	open   bool // the current line has its prefix written
	crOver bool // cursor sits at column 0 over previously written content
}

func NewLineWriter(out io.Writer, prefix string) *LineWriter {
	return &LineWriter{out: out, prefix: prefix, indent: strings.Repeat(" ", ansi.StringWidth(prefix)), now: time.Now}
}

// Watch makes the writer stream for a terminal whose current width width
// reports.
func (writer *LineWriter) Watch(width func() int) {
	writer.watched, writer.width = true, width
}

func (writer *LineWriter) Write(p []byte) (int, error) {
	total := len(p)
	defer writer.send()
	if writer.restart.Swap(false) {
		writer.col = 0
	}
	if !writer.watched {
		writer.lines(p)
		return total, nil
	}
	if move := cursorMove.FindIndex(p); move != nil {
		writer.lines(p[:move[0]])
		writer.end()
		writer.pending.Write(p[move[0]:])
		writer.drawing, writer.lastMove = true, writer.now()
		return total, nil
	}
	if writer.drawing {
		if writer.now().Sub(writer.lastMove) <= drawIdle {
			writer.pending.Write(p)
			return total, nil
		}
		writer.drawing = false
	}
	writer.lines(p)
	return total, nil
}

// lines prefixes p, which holds no cursor move, line by line.
func (writer *LineWriter) lines(p []byte) {
	start := 0
	for i := 0; i < len(p); i++ {
		switch p[i] {
		case '\n':
			writer.text(p[start:i])
			writer.newline()
			start = i + 1
		case '\r':
			writer.text(p[start:i])
			start = i + 1
			if i+1 < len(p) && p[i+1] == '\n' {
				continue
			}
			writer.begin()
			writer.pending.WriteByte('\r')
			writer.open, writer.crOver, writer.col = false, true, 0
		}
	}
	writer.text(p[start:])
}

// send writes what Write or Flush gathered, in one write.
func (writer *LineWriter) send() {
	if writer.pending.Len() > 0 {
		writer.out.Write(writer.pending.Bytes())
		writer.pending.Reset()
	}
}

// begin writes the prefix of a new line, wiping what a \r left behind.
func (writer *LineWriter) begin() {
	if writer.open {
		return
	}
	if writer.crOver {
		writer.pending.WriteString(clearLine)
	}
	writer.pending.WriteString(writer.prefix)
	writer.open, writer.crOver, writer.col = true, false, 0
}

func (writer *LineWriter) text(b []byte) {
	if len(b) == 0 {
		return
	}
	writer.begin()
	limit := 0
	if writer.watched && writer.width != nil {
		limit = writer.width() - ansi.StringWidth(writer.prefix)
	}
	if limit < minWrapWidth {
		writer.pending.Write(b)
		return
	}
	var state byte
	for len(b) > 0 {
		seq, width, n, next := ansi.DecodeSequence(b, state, nil)
		state = next
		if width > 0 && writer.col+width > limit {
			writer.pending.WriteString("\n" + writer.indent)
			writer.col = 0
		}
		writer.pending.Write(seq)
		writer.col += width
		b = b[n:]
	}
}

func (writer *LineWriter) newline() {
	if !writer.open && writer.crOver {
		// Completes a CRLF pair whose \r was already written.
		writer.pending.WriteByte('\n')
		writer.crOver = false
		return
	}
	writer.begin()
	writer.pending.WriteByte('\n')
	writer.open, writer.col = false, 0
}

// Restart tells the writer its open line was ended behind its back: wrapping
// starts again from the line's first column. It may be called from another
// goroutine, so it only flags; the next Write resets col.
func (writer *LineWriter) Restart() { writer.restart.Store(true) }

// Flush ends a partial line. Errors from out are discarded: a failing
// terminal stream leaves nothing useful to do about it.
func (writer *LineWriter) Flush() {
	writer.end()
	writer.send()
}

func (writer *LineWriter) end() {
	if writer.open {
		writer.newline()
	}
}
