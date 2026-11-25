package tui

import (
	"bytes"
	"io"
	"regexp"
	"strings"

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

// LineWriter prefixes a command's output line by line. Piped, a line is held
// until it ends; watched (see Watch), text is written as it arrives so a
// prompt with no newline shows, long lines wrap under their own text, and a
// tool that moves the cursor to draw several rows gets the terminal to
// itself, unprefixed.
type LineWriter struct {
	out    io.Writer
	prefix string
	indent string
	buf    bytes.Buffer // piped: the line so far

	watched bool
	width   func() int // watched: the terminal's width; 0 means no wrapping
	col     int        // watched: visible width of the open line's text
	drawing bool       // watched: a tool is drawing its own display

	open   bool // the current line has its prefix written
	crOver bool // cursor sits at column 0 over previously written content
}

func NewLineWriter(out io.Writer, prefix string) *LineWriter {
	return &LineWriter{out: out, prefix: prefix, indent: strings.Repeat(" ", ansi.StringWidth(prefix))}
}

// Watch makes the writer stream for a terminal whose current width width
// reports.
func (writer *LineWriter) Watch(width func() int) {
	writer.watched, writer.width = true, width
}

func (writer *LineWriter) Write(p []byte) (int, error) {
	total := len(p)
	if writer.watched && !writer.drawing && cursorMove.Match(p) {
		writer.drawing = true
		writer.Flush()
	}
	if writer.drawing {
		writer.out.Write(p)
		return total, nil
	}
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
			writer.flushBuf()
			writer.begin()
			writer.out.Write([]byte{'\r'})
			writer.open, writer.crOver, writer.col = false, true, 0
		}
	}
	writer.text(p[start:])
	return total, nil
}

// begin writes the prefix of a new line, wiping what a \r left behind.
func (writer *LineWriter) begin() {
	if writer.open {
		return
	}
	if writer.crOver {
		writer.out.Write([]byte(clearLine))
	}
	writer.out.Write([]byte(writer.prefix))
	writer.open, writer.crOver, writer.col = true, false, 0
}

func (writer *LineWriter) text(b []byte) {
	if len(b) == 0 {
		return
	}
	if !writer.watched {
		writer.buf.Write(b)
		return
	}
	writer.begin()
	limit := 0
	if writer.width != nil {
		limit = writer.width() - ansi.StringWidth(writer.prefix)
	}
	if limit < minWrapWidth {
		writer.out.Write(b)
		return
	}
	var state byte
	for len(b) > 0 {
		seq, width, n, next := ansi.DecodeSequence(b, state, nil)
		state = next
		if width > 0 && writer.col+width > limit {
			writer.out.Write([]byte("\n" + writer.indent))
			writer.col = 0
		}
		writer.out.Write(seq)
		writer.col += width
		b = b[n:]
	}
}

// flushBuf writes the held line so far, prefixed.
func (writer *LineWriter) flushBuf() {
	if writer.buf.Len() > 0 {
		writer.begin()
		writer.out.Write(writer.buf.Bytes())
		writer.buf.Reset()
	}
}

func (writer *LineWriter) newline() {
	writer.flushBuf()
	if !writer.open && writer.crOver {
		// Completes a CRLF pair whose \r was already written.
		writer.out.Write([]byte{'\n'})
		writer.crOver = false
		return
	}
	writer.begin()
	writer.out.Write([]byte{'\n'})
	writer.open, writer.col = false, 0
}

// Restart tells the writer its open line was ended behind its back: wrapping
// starts again from the line's first column.
func (writer *LineWriter) Restart() { writer.col = 0 }

// Flush ends a partial line. Errors from out are discarded: a failing
// terminal stream leaves nothing useful to do about it.
func (writer *LineWriter) Flush() {
	if writer.buf.Len() > 0 || writer.open {
		writer.newline()
	}
}
