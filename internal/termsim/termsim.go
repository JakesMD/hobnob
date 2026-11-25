// Package termsim replays the bytes written to a terminal onto a grid, so a
// test can read what a person would see instead of the raw escape stream. It
// understands only what hobnob writes: text, \n, \r, colours (ignored),
// erase-line and erase-below, cursor up and down, and save and restore of the
// cursor.
package termsim

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Screen is the grid. Rows grow as needed: it models a terminal tall enough
// never to scroll.
type Screen struct {
	rows     [][]rune
	row, col int
	savedRow int
	savedCol int
	hasSaved bool
}

// Render replays stream and returns the visible rows, trailing blank rows
// dropped and trailing spaces trimmed.
func Render(stream string) []string {
	var s Screen
	s.Write(stream)
	return s.Lines()
}

// Write replays more bytes.
func (s *Screen) Write(stream string) {
	var state byte
	b := []byte(stream)
	for len(b) > 0 {
		seq, _, n, next := ansi.DecodeSequence(b, state, nil)
		state = next
		b = b[n:]
		s.apply(string(seq))
	}
}

func (s *Screen) apply(seq string) {
	switch {
	case seq == "\n":
		s.row++
		s.col = 0
	case seq == "\r":
		s.col = 0
	case seq == "\x1b7":
		s.savedRow, s.savedCol, s.hasSaved = s.row, s.col, true
	case seq == "\x1b8":
		if s.hasSaved {
			s.row, s.col = s.savedRow, s.savedCol
		}
	case strings.HasPrefix(seq, "\x1b["):
		s.csi(seq)
	case strings.HasPrefix(seq, "\x1b"):
	default:
		for _, r := range seq {
			s.put(r)
		}
	}
}

func (s *Screen) csi(seq string) {
	final := seq[len(seq)-1]
	arg := 1
	if n, err := strconv.Atoi(seq[2 : len(seq)-1]); err == nil && n > 0 {
		arg = n
	}
	switch final {
	case 'A':
		s.row = max(0, s.row-arg)
	case 'B':
		s.row += arg
	case 'K':
		if r := s.line(); s.col < len(r) {
			s.rows[s.row] = r[:s.col]
		}
	case 'J':
		if r := s.line(); s.col < len(r) {
			s.rows[s.row] = r[:s.col]
		}
		if s.row+1 < len(s.rows) {
			s.rows = s.rows[:s.row+1]
		}
	}
}

func (s *Screen) line() []rune {
	for len(s.rows) <= s.row {
		s.rows = append(s.rows, nil)
	}
	return s.rows[s.row]
}

func (s *Screen) put(r rune) {
	line := s.line()
	for len(line) < s.col {
		line = append(line, ' ')
	}
	if s.col < len(line) {
		line[s.col] = r
	} else {
		line = append(line, r)
	}
	s.rows[s.row] = line
	s.col++
}

// Lines returns the visible rows.
func (s *Screen) Lines() []string {
	out := make([]string, len(s.rows))
	for i, r := range s.rows {
		out[i] = strings.TrimRight(string(r), " ")
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}
