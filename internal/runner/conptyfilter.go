package runner

import (
	"bytes"
	"io"
)

// conPTYModes are the mode switches ConPTY writes to its output when it
// starts and stops. They are meant for the terminal ConPTY itself sits in, so
// passed on they would switch win32-input and focus-reporting modes on in
// hobnob's own terminal.
var conPTYModes = [][]byte{
	[]byte("\x1b[?9001h"), []byte("\x1b[?9001l"),
	[]byte("\x1b[?1004h"), []byte("\x1b[?1004l"),
}

// conPTYFilter drops conPTYModes from the stream written through it.
type conPTYFilter struct {
	dest io.Writer
	held []byte // a possibly unfinished mode switch at the end of the last write
}

func newConPTYFilter(dest io.Writer) *conPTYFilter { return &conPTYFilter{dest: dest} }

func (f *conPTYFilter) Write(b []byte) (int, error) {
	data := append(f.held, b...)
	f.held = nil
	var out []byte
	for len(data) > 0 {
		esc := bytes.IndexByte(data, 0x1b)
		if esc < 0 {
			out = append(out, data...)
			break
		}
		out = append(out, data[:esc]...)
		data = data[esc:]
		switch matchMode(data) {
		case modeWhole:
			data = data[len(conPTYModes[0]):]
		case modePartial:
			f.held = append([]byte(nil), data...)
			data = nil
		default:
			out = append(out, data[0])
			data = data[1:]
		}
	}
	if len(out) > 0 {
		if _, err := f.dest.Write(out); err != nil {
			return 0, err
		}
	}
	return len(b), nil
}

// Flush writes what Write held back, once no more input is coming.
func (f *conPTYFilter) Flush() {
	if len(f.held) > 0 {
		f.dest.Write(f.held)
		f.held = nil
	}
}

type modeMatch int

const (
	modeNone modeMatch = iota
	modePartial
	modeWhole
)

// matchMode reports whether data, which starts with an escape byte, starts
// with a whole mode switch, or is a prefix of one.
func matchMode(data []byte) modeMatch {
	for _, mode := range conPTYModes {
		if bytes.HasPrefix(data, mode) {
			return modeWhole
		}
	}
	for _, mode := range conPTYModes {
		if len(data) < len(mode) && bytes.HasPrefix(mode, data) {
			return modePartial
		}
	}
	return modeNone
}
