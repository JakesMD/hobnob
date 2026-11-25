package runner

import (
	"bytes"
	"testing"
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
