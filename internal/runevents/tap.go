package runevents

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Tap returns a writer that passes everything to dest and reports each
// finished line of it as an OutputLine, masked with mask. It reports even
// under --quiet, which hides output on the terminal only.
func (s *Stream) Tap(task string, dest io.Writer, mask func(string) string) *TapWriter {
	return &TapWriter{dest: dest, emit: func(line string) {
		s.locked(func(l Listener) { l.OutputLine(task, mask(line)) })
	}}
}

// TapWriter splits what it is written into lines, ended by \n or \r so a
// progress bar redrawing itself reports each state.
type TapWriter struct {
	dest    io.Writer
	emit    func(string)
	pending strings.Builder
}

func (t *TapWriter) Write(p []byte) (int, error) {
	n, err := t.dest.Write(p)
	for _, b := range p {
		if b == '\n' || b == '\r' {
			t.Flush()
			continue
		}
		t.pending.WriteByte(b)
	}
	return n, err
}

// Flush reports a line still open, so output that ends without a newline
// isn't lost. The caller must not Write meanwhile.
func (t *TapWriter) Flush() {
	if line := strings.TrimSpace(ansi.Strip(t.pending.String())); line != "" {
		t.emit(line)
	}
	t.pending.Reset()
}
