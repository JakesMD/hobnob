//go:build unix

package e2e

import (
	"os"
	"strings"
	"testing"
	"time"
)

// waitForFile waits for name to appear in the working directory, which a
// running Case chdirs into.
func waitForFile(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(name); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never appeared", name)
}

func TestE2E_RunPTY_WatchedCommandSeesATTYOnStdoutOnly(t *testing.T) {
	// given a run watched on a terminal, when a command checks its streams,
	// then stdout is a terminal and stderr is not (why: stdout on a PTY keeps
	// colour and progress bars while stderr stays separate)

	// Arrange
	screen, _ := screenOf(80, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          [ -t 1 ] && echo "stdout tty" || echo "stdout pipe"
		          [ -t 2 ] && echo "stderr tty" || echo "stderr pipe"
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen})

	// Assert
	res.OK(t)
	res.Lines(t, "stdout tty", "stderr pipe")
}

func TestE2E_RunPTY_PipedCommandSeesNoTTY(t *testing.T) {
	// given a run whose output is piped, when a command checks its streams,
	// then neither is a terminal (why: logs and CI get no escape codes)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          [ -t 1 ] && echo "stdout tty" || echo "stdout pipe"
		          [ -t 2 ] && echo "stderr tty" || echo "stderr pipe"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "stdout pipe", "stderr pipe")
}

func TestE2E_RunPTY_CommandInheritsStdin(t *testing.T) {
	// given hobnob's stdin holds a line, when a command reads it, then it
	// gets the line, piped or watched (why: read, sudo and [y/N] prompts must
	// work, and a command used to get no stdin)

	// Arrange
	watched, _ := screenOf(80, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: read n && echo "hi $n"
	`

	// Act
	piped := Run(t, Case{Yml: yml, Args: []string{"t"}, Stdin: "bob\n"})
	onTerminal := Run(t, Case{Yml: yml, Args: []string{"t"}, Stdin: "bob\n", Screen: watched})

	// Assert
	piped.OK(t)
	piped.Lines(t, "hi bob")
	onTerminal.OK(t)
	onTerminal.Lines(t, "hi bob")
}

func TestE2E_RunPTY_PartialLineShowsBeforeInput(t *testing.T) {
	// given a watched run whose command prints a prompt with no newline,
	// when it reads its answer, then the prompt is in the output under the
	// prefix (why: a read prompt must be seen before the user answers)

	// Arrange
	screen, _ := screenOf(80, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf 'name? '; read n; echo "hi $n"
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen, Stdin: "bob\n"})

	// Assert
	res.OK(t)
	res.Out(t, "t │ name? hi bob")
}

func TestE2E_RunPTY_WidthIsTheTerminalsLessThePrefix(t *testing.T) {
	// given a watched run on a 50-column terminal, when a command asks its
	// terminal's width, then it is 50 less the prefix "t │ " (why: a tool
	// sizing its progress bar to the terminal must not run past the prefix)

	// Arrange
	screen, _ := screenOf(50, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: stty size <&1
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen})

	// Assert
	res.OK(t)
	res.Out(t, "22 46")
}

func TestE2E_RunPTY_ResizeReachesTheRunningCommand(t *testing.T) {
	// given a watched run, when the terminal is resized while a command runs,
	// then the command sees the new width (why: the PTY follows the terminal)

	// Arrange
	screen, resize := screenOf(50, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          touch ready
		          while [ ! -f go ]; do :; done
		          stty size <&1
	`
	done := make(chan *Result)

	// Act
	go func() { done <- Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen}) }()
	waitForFile(t, "ready")
	resize(70, 30)
	if err := os.WriteFile("go", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res := <-done

	// Assert
	res.OK(t)
	res.Out(t, "28 66")
}

func TestE2E_RunPTY_CursorMovingOutputHasNoPrefix(t *testing.T) {
	// given a watched run, when a command redraws rows by moving the cursor,
	// then those lines carry no prefix (why: a tool drawing its own display
	// can't be prefixed per line)

	// Arrange
	screen, _ := screenOf(80, 24)
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf 'a\n\033[1Ab\n'; echo c
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen})

	// Assert
	res.OK(t)
	res.Out(t, "\x1b[1Ab")
	res.NotOut(t, "│ b", "│ c")
}

func TestE2E_RunPTY_LongLinesWrapUnderTheirTextOnlyOnATerminal(t *testing.T) {
	// given a 40-column terminal and a stderr line longer than its text
	// column, when the run is watched, then the line wraps under its own
	// text, and piped it stays on one line (why: a wrapped line must not run
	// back under the prefix, and logs keep one line per output line)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo aaaaaaaaaabbbbbbbbbbccccccccccdddddddddd >&2
	`
	screen, _ := screenOf(34, 24)

	// Act
	watched := Run(t, Case{Yml: yml, Args: []string{"t"}, Screen: screen})
	piped := Yml(t, yml, "t")

	// Assert
	watched.OK(t)
	piped.OK(t)
	if want := "t │ aaaaaaaaaabbbbbbbbbbcccccccccc\n    dddddddddd"; !strings.Contains(watched.Stderr, want) {
		t.Errorf("watched: got %q, want it to contain %q", watched.Stderr, want)
	}
	if want := "t │ aaaaaaaaaabbbbbbbbbbccccccccccdddddddddd"; !strings.Contains(piped.Stderr, want) {
		t.Errorf("piped: got %q, want it to contain %q", piped.Stderr, want)
	}
}
