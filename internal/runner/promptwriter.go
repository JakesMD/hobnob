package runner

import (
	"bytes"
	"io"

	"hobnob/internal/tui"
)

// promptWriter keeps a --quiet step's output in held but shows a trailing
// partial line (a prompt with no newline) through shown as it arrives, so
// read, sudo and [y/N] don't look hung. Whole lines stay hidden.
type promptWriter struct {
	held    io.Writer
	shown   *tui.LineWriter
	watched bool // shown streams partial text; unwatched, it holds a line until it ends

	open bool // a partial line is showing
}

func (w *promptWriter) Write(p []byte) (int, error) {
	n, err := w.held.Write(p)
	tail := p
	if end := bytes.LastIndexByte(p, '\n'); end >= 0 {
		if w.open {
			w.shown.Flush()
			w.open = false
		}
		tail = p[end+1:]
	}
	if len(tail) > 0 {
		w.shown.Write(tail)
		if !w.watched {
			w.shown.Flush()
		}
		w.open = w.watched
	}
	return n, err
}
