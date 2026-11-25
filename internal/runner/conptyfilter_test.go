package runner

import (
	"bytes"
	"strings"
	"testing"

	"hobnob/internal/tui"
)

func TestConPTYFilter_Write_DropsTerminalModeSequencesSplitAcrossWrites(t *testing.T) {
	// given a ConPTY stream that opens with win32-input and focus-reporting
	// mode switches, split mid-sequence across writes, when it is filtered,
	// then only the command's own bytes come out (why: those modes would
	// otherwise be switched on in hobnob's own terminal)

	// Arrange
	var out bytes.Buffer
	filter := newConPTYFilter(&out)

	// Act
	_, _ = filter.Write([]byte("\x1b[?90"))
	_, _ = filter.Write([]byte("01h\x1b[?1004hhello \x1b[31mred\x1b[0m"))
	filter.Flush()

	// Assert
	if got, want := out.String(), "hello \x1b[31mred\x1b[0m"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConPTYFilter_Flush_PassesOnAnUnfinishedEscape(t *testing.T) {
	// given a stream that ends on a lone escape byte, when the filter is
	// flushed, then the byte is written (why: it may be the start of the
	// command's own sequence, and nothing may be swallowed)

	// Arrange
	var out bytes.Buffer
	filter := newConPTYFilter(&out)

	// Act
	_, _ = filter.Write([]byte("a\x1b[?"))
	filter.Flush()

	// Assert
	if got, want := out.String(), "a\x1b[?"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestConPTYFilter_Write_OpeningSequencesLeaveTheOutputPrefixedOnBothStreams(t *testing.T) {
	// given a watched run whose command's stdout is a ConPTY stream opening
	// with ConPTY's set-up (modes, cursor hide, screen clear, home, title)
	// and whose stderr is a plain pipe, when the command writes a line to
	// each, then both lines come out under the prefix and no empty prefixed
	// line precedes them (why: the clear and home read as a tool redrawing
	// the screen, which passed the rest of the step's output through
	// unprefixed)

	// Arrange
	var stdoutBuf, stderrBuf bytes.Buffer
	log := tui.NewRunLog(&stdoutBuf, &stderrBuf)
	log.Watch(&tui.Screen{
		Size:     func() (int, int) { return 80, 24 },
		OnResize: func(func()) func() { return func() {} },
	})
	log.Enter("t", "")
	log.Run("t", "cmd")
	stdout, stderr := log.Output("t")
	filter := newConPTYFilter(stdout)

	// Act
	_, _ = filter.Write([]byte("\x1b[?9001h\x1b[?1004h\x1b[?25l\x1b[2J\x1b[m\x1b[H"))
	_, _ = filter.Write([]byte("\x1b]0;C:\\Windows\\system32\\cmd.exe\x07\x1b[?25h"))
	_, _ = filter.Write([]byte("stdout tty\r\n"))
	_, _ = stderr.Write([]byte("stderr pipe\n"))
	filter.Flush()
	stdout.Flush()
	stderr.Flush()

	// Assert
	got := stdoutBuf.String() + stderrBuf.String()
	for _, want := range []string{"│ stdout tty\n", "│ stderr pipe\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "│ \n") || strings.Contains(got, "\x1b[2J") {
		t.Errorf("stray empty line or clear in %q", got)
	}
}

func TestConPTYFilter_Write_OpeningSequencesSplitAcrossWritesAreDropped(t *testing.T) {
	// given ConPTY's set-up split mid-sequence across writes, when the
	// command's output follows, then only that output comes out, with its
	// own colour codes intact (why: a clear that arrives in two chunks must
	// not slip through, and the command's colours must not be taken for
	// set-up)

	// Arrange
	var out bytes.Buffer
	filter := newConPTYFilter(&out)

	// Act
	_, _ = filter.Write([]byte("\x1b[?25l\x1b["))
	_, _ = filter.Write([]byte("2J\x1b[m\x1b[1;1H\x1b]0;ti"))
	_, _ = filter.Write([]byte("tle\x1b\\\x1b[31mred\x1b[0m\r\n"))
	_, _ = filter.Write([]byte("\x1b[2J"))
	filter.Flush()

	// Assert
	if got, want := out.String(), "\x1b[31mred\x1b[0m\r\n\x1b[2J"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
