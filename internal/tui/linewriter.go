package tui

import (
	"bytes"
	"io"
)

// clearLine is the ANSI "erase from cursor to end of line" sequence, used to
// wipe leftover characters when a \r-redrawn line (e.g. rsync --progress) is
// shorter than the one it's overwriting.
const clearLine = "\033[K"

type LineWriter struct {
	out       io.Writer
	prefix    string
	buf       bytes.Buffer
	lastWasCR bool // cursor sits at column 0 over previously written content
}

func NewLineWriter(out io.Writer, prefix string) *LineWriter {
	return &LineWriter{out: out, prefix: prefix}
}

func (writer *LineWriter) Write(p []byte) (int, error) {
	totalBytes := len(p)
	for i := 0; i < len(p); i++ {
		byteVal := p[i]
		switch byteVal {
		case '\n':
			if writer.lastWasCR && writer.buf.Len() == 0 {
				// Completes a CRLF pair whose \r was already flushed live
				// (in this Write call or an earlier one, per the '\r' case
				// below) — just advance past it, nothing new to flush.
				writer.out.Write([]byte{'\n'})
				writer.lastWasCR = false
				continue
			}
			writer.flush('\n')
		case '\r':
			// A \r immediately followed by \n in the same chunk is an
			// ordinary CRLF line ending — let the \n above flush it as a
			// plain line, since a real progress redraw (rsync --progress,
			// etc.) uses \r alone. A \r landing at the end of this chunk
			// still has to flush live (its pairing \n, if any, may not
			// arrive for a while) — the lastWasCR+empty-buf check above
			// absorbs that \n correctly if it turns up in a later call.
			if i+1 < len(p) && p[i+1] == '\n' {
				continue
			}
			writer.flush('\r')
		default:
			writer.buf.WriteByte(byteVal)
		}
	}
	return totalBytes, nil
}

func (writer *LineWriter) flush(terminator byte) {
	if writer.lastWasCR {
		writer.out.Write([]byte(clearLine))
	}
	writer.out.Write([]byte(writer.prefix))
	writer.out.Write(writer.buf.Bytes())
	writer.out.Write([]byte{terminator})
	writer.buf.Reset()
	writer.lastWasCR = terminator == '\r'
}

// Flush writes any buffered partial line. Errors from out are discarded because
// out is always os.Stdout/os.Stderr — if those fail the process is dying anyway.
func (writer *LineWriter) Flush() {
	if writer.buf.Len() > 0 {
		writer.flush('\n')
	}
}
