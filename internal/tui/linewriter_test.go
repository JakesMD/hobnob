package tui

import (
	"bytes"
	"strings"
	"testing"
)

func TestLineWriter_Write(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "given input ending in newline, when written, then line is prefixed and flushed immediately (why: standard line-buffered output)",
			input: "hello\n",
			want:  "[t] hello\n",
		},
		{
			name:  "given input with a carriage return, when written, then partial line is prefixed and flushed on \\r, not held (why: rsync --progress redraws via \\r and must reach the terminal live, not only at Flush)",
			input: "50%\r",
			want:  "[t] 50%\r",
		},
		{
			name:  "given a \\r-redrawn progress sequence, when written, then each update is flushed on its own \\r (why: intermediate updates must not be silently overwritten in the buffer)",
			input: "0%\r50%\r100%\n",
			want:  "[t] 0%\r\033[K[t] 50%\r\033[K[t] 100%\n",
		},
		{
			name:  "given a \\r redraw followed by a shorter one, when written, then clear-to-eol precedes the shorter redraw (why: without it, trailing chars from the longer line would linger onscreen)",
			input: "100%\r5%\r",
			want:  "[t] 100%\r\033[K[t] 5%\r",
		},
		{
			name:  "given no line terminator at all, when written, then nothing is flushed yet (why: partial lines wait for Flush so they aren't split mid-write)",
			input: "no terminator",
			want:  "",
		},
		{
			name:  "given a CRLF-terminated line, when written, then it is flushed once as an ordinary line with no clear-to-eol (why: \\r\\n is a plain line ending, not a progress redraw — treating it as one erased the content just written)",
			input: "hello\r\n",
			want:  "[t] hello\n",
		},
		{
			name:  "given a \\r progress redraw followed by a CRLF-terminated line, when written, then clear-to-eol still precedes the CRLF line (why: the cursor is left mid-line by the prior \\r redraw regardless of how the next line ends)",
			input: "100%\rdone\r\n",
			want:  "[t] 100%\r\033[K[t] done\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer
			lineWriter := NewLineWriter(&buf, "[t] ")

			// Act
			lineWriter.Write([]byte(test.input))

			// Assert
			if got := buf.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestLineWriter_Write_CRLFSplitAcrossWriteCalls(t *testing.T) {
	// given a CRLF pair split across two Write calls (\r in one, \n in the next), when both are written, then no spurious clear-to-eol or blank prefixed line appears (why: os/exec feeds LineWriter from a pipe in arbitrary chunks, so a subprocess's CRLF line ending isn't guaranteed to land in a single Write call)
	// Arrange
	var buf bytes.Buffer
	lineWriter := NewLineWriter(&buf, "[t] ")

	// Act
	lineWriter.Write([]byte("hello\r"))
	lineWriter.Write([]byte("\n"))

	// Assert
	got := buf.String()
	if strings.Contains(got, clearLine) {
		t.Errorf("got %q: spurious clear-to-eol for a plain CRLF line ending", got)
	}
	want := "[t] hello\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLineWriter_Write_BareCRWithNoFollowingByteStaysLive(t *testing.T) {
	// given a bare \r flushed live at the end of one Write call, when the next Write call starts with ordinary content (not \n), then it's treated as a new progress redraw, not absorbed (why: only a following \n completes a CRLF pair — anything else means the \r really was a standalone redraw)
	// Arrange
	var buf bytes.Buffer
	lineWriter := NewLineWriter(&buf, "[t] ")

	// Act
	lineWriter.Write([]byte("100%\r"))
	lineWriter.Write([]byte("5%\r"))

	// Assert
	want := "[t] 100%\r" + clearLine + "[t] 5%\r"
	if got := buf.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLineWriter_Flush(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "given a partial line with no terminator, when Flush called, then it is prefixed and newline-terminated (why: last partial line must still reach the terminal)",
			input: "partial",
			want:  "[t] partial\n",
		},
		{
			name:  "given an empty buffer, when Flush called, then nothing is written (why: must not emit a stray empty line every step)",
			input: "",
			want:  "",
		},
		{
			name:  "given buffer already drained by a trailing \\r write, when Flush called, then nothing more is written (why: \\r already flushed its content, Flush must not repeat it)",
			input: "100%\r",
			want:  "",
		},
		{
			name:  "given a \\r redraw followed by unterminated trailing text, when Flush called, then clear-to-eol precedes the final line (why: the final partial line sits where the last \\r left the cursor, so leftovers must be wiped too)",
			input: "100%\rdone",
			want:  "\033[K[t] done\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer
			lineWriter := NewLineWriter(&buf, "[t] ")
			lineWriter.Write([]byte(test.input))
			buf.Reset() // isolate Flush's own output from Write's

			// Act
			lineWriter.Flush()

			// Assert
			if got := buf.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}
