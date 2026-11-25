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

// conPTYFilter drops conPTYModes from the stream written through it, and the
// opening sequences ConPTY writes before the command's first output.
type conPTYFilter struct {
	dest    io.Writer
	started bool   // the command's own output has begun
	held    []byte // a possibly unfinished mode switch at the end of the last write
}

func newConPTYFilter(dest io.Writer) *conPTYFilter { return &conPTYFilter{dest: dest} }

func (f *conPTYFilter) Write(b []byte) (int, error) {
	data := append(f.held, b...)
	f.held = nil
	if !f.started {
		data = f.skipOpening(data)
	}
	var out []byte
	for len(data) > 0 {
		esc := bytes.IndexByte(data, 0x1b)
		if esc < 0 {
			out = append(out, data...)
			break
		}
		out = append(out, data[:esc]...)
		data = data[esc:]
		match, n := matchMode(data)
		switch match {
		case modeWhole:
			data = data[n:]
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

// skipOpening drops ConPTY's own set-up at the start of the stream: cursor
// visibility and mode switches, screen clear, cursor home, attribute reset,
// erase-line and the window title. Left in, the clear and the home look like a
// tool redrawing the screen and the title makes a prefixed empty line, so the
// command's output after them would pass through unprefixed. It returns what
// follows, and holds back an unfinished sequence.
func (f *conPTYFilter) skipOpening(data []byte) []byte {
	for len(data) > 0 {
		if data[0] != 0x1b {
			f.started = true
			return data
		}
		n, complete := openingSequence(data)
		if !complete {
			f.held = append([]byte(nil), data...)
			return nil
		}
		if n == 0 {
			f.started = true
			return data
		}
		data = data[n:]
	}
	return data
}

// openingSequence reports the length of the set-up sequence data starts with,
// 0 if it starts with another one. complete is false when data ends before
// the sequence does.
func openingSequence(data []byte) (n int, complete bool) {
	if len(data) < 2 {
		return 0, false
	}
	switch data[1] {
	case '[':
		for i := 2; i < len(data); i++ {
			c := data[i]
			if c >= 0x40 && c <= 0x7e {
				return openingCSI(data[2:i], c, i+1)
			}
			if c < 0x20 || c > 0x3f {
				return 0, true
			}
		}
		return 0, false
	case ']':
		for i := 2; i < len(data); i++ {
			if data[i] == 0x07 {
				return i + 1, true
			}
			if data[i] == 0x1b {
				if i+1 >= len(data) {
					return 0, false
				}
				if data[i+1] == '\\' {
					return i + 2, true
				}
				return 0, true
			}
		}
		return 0, false
	}
	return 0, true
}

// openingCSI says whether a CSI sequence with these parameters and final byte
// is set-up, giving its total length n.
func openingCSI(params []byte, final byte, n int) (int, bool) {
	switch final {
	case 'h', 'l':
		if len(params) > 0 && params[0] == '?' {
			return n, true
		}
	case 'J', 'H', 'K':
		return n, true
	case 'm':
		if len(params) == 0 || string(params) == "0" {
			return n, true
		}
	}
	return 0, true
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
// with a whole mode switch, or is a prefix of one. For a whole one, n is its
// length.
func matchMode(data []byte) (match modeMatch, n int) {
	for _, mode := range conPTYModes {
		if bytes.HasPrefix(data, mode) {
			return modeWhole, len(mode)
		}
	}
	for _, mode := range conPTYModes {
		if len(data) < len(mode) && bytes.HasPrefix(mode, data) {
			return modePartial, 0
		}
	}
	return modeNone, 0
}
