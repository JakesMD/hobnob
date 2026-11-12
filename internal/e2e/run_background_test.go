//go:build !windows

// The fixtures need sh, mkfifo and tail, and waitGone needs syscall.Kill.

package e2e

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"hobnob/internal/runner"
)

// skipWithoutProcessGroups skips tests that need a step's leftovers to be
// signalled, which hobnob does only where it can hold the step's PID.
func skipWithoutProcessGroups(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("leftovers are only signalled on linux and darwin")
	}
}

// waitGone fails the test unless pid is gone within a second. A killed
// process lingers briefly as a zombie until its new parent reaps it.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("leftover %d still running after the step", pid)
}

func TestE2E_RunBackground_StepFinishesWhenShellExits(t *testing.T) {
	// given a step that backgrounds a job that never exits on its own, when
	// the step runs, then it finishes as soon as its shell does and the next
	// step runs (why: the job holds the step's output pipe, which used to
	// block the run forever)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo started
		          tail -f /dev/null &
		      - run: echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "started", "next")
}

func TestE2E_RunBackground_LeftoverGetsSigtermAndItsOutputIsKept(t *testing.T) {
	// given a backgrounded job that traps SIGTERM and prints on it, when the
	// step's shell exits, then the job is signalled and what it prints on
	// the way out still shows under the step (why: the step owns its
	// process group, and output drains until the leftovers are gone)
	skipWithoutProcessGroups(t)

	// Arrange
	// The fifo holds the shell until the trap is armed and tail forked, so the
	// SIGTERM can't race ahead of either.
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo armed
		          (trap 'echo stopped; exit 0' TERM; tail -f /dev/null & echo > armed; wait) &
		          read _ < armed
		          echo shell done
		      - run: echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "shell done", "stopped", "next")
}

func TestE2E_RunBackground_LeftoverIgnoringSigtermIsKilledAfterGrace(t *testing.T) {
	// given a backgrounded job that ignores SIGTERM and keeps the output pipe
	// open, when the grace period runs out, then it's killed and the run
	// carries on (why: a job that won't stop must neither hang the run nor
	// outlive it)
	skipWithoutProcessGroups(t)

	// Arrange
	restore := runner.SetLeftoverGrace(50 * time.Millisecond)
	defer restore()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo armed
		          (trap '' TERM; echo > armed; exec tail -f /dev/null) &
		          echo $! > leftover.pid
		          read _ < armed
		      - run: echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "next")
	pidText, err := os.ReadFile(filepath.Join(res.Dir, "leftover.pid"))
	if err != nil {
		t.Fatalf("read leftover.pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatalf("parse leftover.pid: %v", err)
	}
	waitGone(t, pid)
}

func TestE2E_RunBackground_IntoCapturesOutputAlongsideLeftover(t *testing.T) {
	// given into: on a step that also backgrounds a long-running job, when
	// the step finishes, then into: still captures the step's output and
	// exit code (why: capture waits for the pipes, which must still close)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          printf hello
		          tail -f /dev/null &
		        into:
		          - OUT: stdout
		          - CODE: exit
		      - run: echo "captured={{.OUT}} code={{.CODE}}"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "captured=hello code=0")
}

func TestE2E_RunBackground_ExitCodeIsTheShellsNotTheLeftovers(t *testing.T) {
	// given a step that backgrounds a job and then exits non-zero, when the
	// leftover is signalled, then into: exit is the shell's own code (why:
	// ending a leftover must not change how the step itself went)
	skipWithoutProcessGroups(t)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          tail -f /dev/null &
		          exit 3
		        soft: true
		        into:
		          - CODE: exit
		      - run: echo "code={{.CODE}}"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "code=3")
}

func TestE2E_RunBackground_WaitLetsJobsFinish(t *testing.T) {
	// given a step that ends with wait, when its jobs take longer than the
	// shell's other commands, then their output is all there before the next
	// step (why: wait is the documented way to keep jobs within the step)
	skipWithoutProcessGroups(t)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo go
		          (read _ < go; echo job done) &
		          echo > go
		          wait
		      - run: echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "job done", "next")
}

func TestE2E_RunBackground_QuietStepWithLeftoverStaysQuiet(t *testing.T) {
	// given quiet: on a step that leaves a job running, when the step
	// succeeds, then its output stays hidden and the run carries on (why:
	// quiet: buffers the same pipes the leftover holds)
	skipWithoutProcessGroups(t)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo hidden
		          tail -f /dev/null &
		        quiet: true
		      - run: echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "next")
}

func TestE2E_RunBackground_SecondCtrlCReachesLeftoverDuringGrace(t *testing.T) {
	// given a leftover that ignores SIGTERM and holds the step's output, when
	// a 2nd CTRL+C (KillRunningStep) arrives after the shell has exited, then
	// it kills the leftover at once instead of waiting out the grace (why:
	// the step stays registered until its leftovers are dealt with)
	skipWithoutProcessGroups(t)

	// Arrange
	restore := runner.SetLeftoverGrace(time.Minute)
	defer restore()
	marker := filepath.Join(t.TempDir(), "exiting")
	go func() {
		for {
			if _, err := os.Stat(marker); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		// Give the shell time to exit after writing the marker. If the kill
		// lands first anyway, it ends the shell too and soft: lets the run
		// carry on, so the test can't flake; only a kill that can't reach
		// the leftover makes the run wait out the minute and fail.
		time.Sleep(100 * time.Millisecond)
		runner.KillRunningStep()
	}()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo armed
		          (trap '' TERM; echo > armed; exec tail -f /dev/null) &
		          read _ < armed
		          touch "$MARKER"
		        soft: true
		      - run: echo next
	`

	// Act
	start := time.Now()
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Env: map[string]string{"MARKER": marker}})

	// Assert
	res.OK(t)
	res.Out(t, "next")
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("run took %s: the 2nd CTRL+C didn't reach the leftover", elapsed)
	}
}
