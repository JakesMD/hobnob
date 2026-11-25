package tui

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestLineWriter_Write(t *testing.T) {
	// given output with newlines and carriage returns, when written, then
	// each line or redraw is prefixed and flushed promptly (why: progress
	// output like rsync --progress must show live)
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
			name:  "given no line terminator at all, when written, then the prefixed partial line is written at once (why: a prompt with no newline must show before the answer, piped or not)",
			input: "no terminator",
			want:  "[t] no terminator",
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
	// given buffered or drained output, when flushed, then only a real
	// partial line is written (why: the last line must reach the user without
	// a stray blank line)
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "given a partial line with no terminator, when Flush called, then it is newline-terminated (why: the last partial line must end before the next line starts)",
			input: "partial",
			want:  "\n",
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
			name:  "given a \\r redraw followed by unterminated trailing text, when Flush called, then the line is newline-terminated (why: the text was already written with its clear-to-eol)",
			input: "100%\rdone",
			want:  "\n",
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

func TestLineWriter_Watched_Write(t *testing.T) {
	// given a writer watched on a 24-column terminal and a 4-column prefix, when output arrives, then partial lines show at once, long lines wrap under their text and cursor-moving output passes through unprefixed (why: prompts, long lines and multi-row redraws all need different handling on a terminal)
	tests := []struct {
		name   string
		writes []string
		want   string
	}{
		{
			name:   "given a prompt with no newline, when written, then it shows at once (why: a read prompt must be visible before the user types)",
			writes: []string{"name? "},
			want:   "[t] name? ",
		},
		{
			name:   "given a partial line completed by a later write, when both are written, then the prefix appears once (why: the line is one line)",
			writes: []string{"name? ", "bob\n"},
			want:   "[t] name? bob\n",
		},
		{
			name:   "given a line longer than the text column, when written, then it wraps under its own text (why: a wrapped line must not run back under the prefix)",
			writes: []string{"aaaaaaaaaaaaaaaaaaaaaaaa\n"},
			want:   "[t] aaaaaaaaaaaaaaaaaaaa\n    aaaa\n",
		},
		{
			name:   "given colour codes inside a long line, when written, then they take no width (why: escape sequences are invisible)",
			writes: []string{"\x1b[31m" + "aaaaaaaaaaaaaaaaaaaa" + "\x1b[0m" + "b\n"},
			want:   "[t] \x1b[31maaaaaaaaaaaaaaaaaaaa\x1b[0m\n    b\n",
		},
		{
			name:   "given a cursor-up sequence, when written, then it and what follows pass through without a prefix (why: a tool redrawing rows can't be prefixed per line)",
			writes: []string{"one\n", "\x1b[1Atwo\n", "three\n"},
			want:   "[t] one\n\x1b[1Atwo\nthree\n",
		},
		{
			name:   "given lines before a cursor move in the same write, when written, then they keep their prefix (why: only what the tool redraws is unprefixed)",
			writes: []string{"a\nb\n\x1b[2Ac\n"},
			want:   "[t] a\n[t] b\n\x1b[2Ac\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			var buf bytes.Buffer
			lineWriter := NewLineWriter(&buf, "[t] ")
			lineWriter.Watch(func() int { return 24 })

			// Act
			for _, w := range test.writes {
				lineWriter.Write([]byte(w))
			}

			// Assert
			if got := buf.String(); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestLineWriter_Piped_Write_LongLineIsNotWrapped(t *testing.T) {
	// given a writer that isn't watched, when a long line is written, then it passes whole (why: logs and files must keep one line per output line)
	// Arrange
	var buf bytes.Buffer
	lineWriter := NewLineWriter(&buf, "[t] ")
	long := strings.Repeat("a", 200)

	// Act
	lineWriter.Write([]byte(long + "\n"))

	// Assert
	if want := "[t] " + long + "\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

func TestLineWriter_Watched_Write_PrefixesReturnOnceTheToolStopsMovingTheCursor(t *testing.T) {
	// given a tool that moved the cursor, when it writes plain lines after a
	// pause, then they are prefixed again (why: one redraw must not strip the
	// prefix from the rest of a long step)
	// Arrange
	var buf bytes.Buffer
	now := time.Unix(1000, 0)
	lineWriter := NewLineWriter(&buf, "[t] ")
	lineWriter.Watch(func() int { return 40 })
	lineWriter.now = func() time.Time { return now }
	lineWriter.Write([]byte("\x1b[2Ax\n"))

	// Act
	now = now.Add(drawIdle / 2)
	lineWriter.Write([]byte("during\n"))
	now = now.Add(drawIdle)
	lineWriter.Write([]byte("after\n"))

	// Assert
	if want := "\x1b[2Ax\nduring\n[t] after\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}

type countingWriter struct{ bytes, writes int }

func (c *countingWriter) Write(p []byte) (int, error) {
	c.bytes += len(p)
	c.writes++
	return len(p), nil
}

func TestLineWriter_Watched_Write_ReachesTheTerminalInOneWritePerChunk(t *testing.T) {
	// given a watched writer, when a chunk of many long lines arrives, then it
	// reaches the terminal in one write (why: a write per grapheme made a
	// verbose build flood the terminal)
	// Arrange
	var out countingWriter
	lineWriter := NewLineWriter(&out, "[t] ")
	lineWriter.Watch(func() int { return 100 })
	chunk := []byte(strings.Repeat(strings.Repeat("a", 80)+"\n", 50))

	// Act
	lineWriter.Write(chunk)

	// Assert
	if out.writes != 1 {
		t.Errorf("writes = %d, want 1", out.writes)
	}
}

func TestLineWriter_Piped_Write_PartialLineContinuesWhenMoreArrives(t *testing.T) {
	// given a piped writer, when a prompt with no newline arrives and its
	// answer follows, then the prompt is written at once and the line stays one
	// prefixed line (why: the prompt must show before the command reads)
	// Arrange
	var buf bytes.Buffer
	lineWriter := NewLineWriter(&buf, "[t] ")

	// Act
	lineWriter.Write([]byte("name? "))
	atPrompt := buf.String()
	lineWriter.Write([]byte("bob\n"))

	// Assert
	if atPrompt != "[t] name? " {
		t.Errorf("at the prompt: %q", atPrompt)
	}
	if want := "[t] name? bob\n"; buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
