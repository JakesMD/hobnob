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
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("job %d still running after the run", pid)
}

// readPid reads the PID a fixture wrote to name in dir.
func readPid(t *testing.T, dir, name string) int {
	t.Helper()
	text, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return pid
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

func TestE2E_RunBackground_JobKeepsRunningAfterItsStepEnds(t *testing.T) {
	// given a step that backgrounds a job, when the step ends and a later
	// step runs, then the job is still running and can do its work (why:
	// `cmd &` is how a step launches an app, such as a GUI, that must stay
	// open while the rest of the run goes on)

	// Arrange
	// The job blocks on the fifo until the next step opens it with <>
	// (read-write, so the open never blocks). The next step then spins until
	// the job has either touched alive or is gone, so a job stopped when its
	// step ended shows as "job gone" instead of hanging the run.
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo gate
		          (trap '' TERM; read _ < gate; touch alive) &
		          echo $! > job.pid
		      - run: |
		          echo 1<> gate
		          while [ ! -f alive ] && kill -0 "$(cat job.pid)" 2>/dev/null; do :; done
		          if [ -f alive ]; then echo "job alive"; else echo "job gone"; fi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "job alive")
}

func TestE2E_RunBackground_JobNotHoldingTheOutputKeepsRunningAfterItsStepEnds(t *testing.T) {
	// given a step that backgrounds a job which sends its output elsewhere,
	// when the step ends and a later step runs, then the job is still running
	// (why: an app launched with `app >/dev/null 2>&1 &` must stay open too,
	// though nothing ties it to the step's pipes)

	// Arrange
	// As in JobKeepsRunningAfterItsStepEnds, but the job leaves SIGTERM's
	// default alone, so even a SIGTERM at step end would show as "job gone".
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo gate
		          (read _ < gate; touch alive) >/dev/null 2>&1 &
		          echo $! > job.pid
		      - run: |
		          echo 1<> gate
		          while [ ! -f alive ] && kill -0 "$(cat job.pid)" 2>/dev/null; do :; done
		          if [ -f alive ]; then echo "job alive"; else echo "job gone"; fi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "job alive")
}

func TestE2E_RunBackground_JobIsKilledWhenHobnobExits(t *testing.T) {
	// given a step that backgrounds a job which ignores SIGTERM, when the
	// job is still running at the last step and hobnob then exits, then the
	// job is gone (why: a run cleans up what its steps left running, even a
	// job that won't stop politely)
	skipWithoutProcessGroups(t)

	// Arrange
	restore := runner.SetLeftoverGrace(50 * time.Millisecond)
	defer restore()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          (trap '' TERM; exec tail -f /dev/null) &
		          echo $! > job.pid
		      - run: kill -0 "$(cat job.pid)" && echo "job alive"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "job alive")
	waitGone(t, readPid(t, res.Dir, "job.pid"))
}

func TestE2E_RunBackground_JobGetsSigtermWhenHobnobExitsAndItsOutputIsKept(t *testing.T) {
	// given a backgrounded job that traps SIGTERM and prints on it, when the
	// run ends, then the job is signalled and what it prints on the way out
	// still shows under its step's prefix (why: a job's output keeps showing
	// after its step, until the run has stopped it)
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
	res.Lines(t, "shell done", "next", "stopped")
}

func TestE2E_RunBackground_JobNotHoldingTheOutputIsKilledWhenHobnobExits(t *testing.T) {
	// given a backgrounded job that ignores SIGTERM and sends its output
	// elsewhere, when the run ends, then the job is gone (why: hobnob finds
	// leftovers by the step's process group, not only by its open pipes)
	skipWithoutProcessGroups(t)

	// Arrange
	restore := runner.SetLeftoverGrace(50 * time.Millisecond)
	defer restore()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          (trap '' TERM; exec tail -f /dev/null) >/dev/null 2>&1 &
		          echo $! > job.pid
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	waitGone(t, readPid(t, res.Dir, "job.pid"))
}

func TestE2E_RunBackground_JobIsKilledWhenALaterStepFails(t *testing.T) {
	// given a step that backgrounds a job ignoring SIGTERM, when a later step
	// fails, then the job is gone once the run has failed (why: a failing run
	// cleans up what its steps left running, like a finished one)
	skipWithoutProcessGroups(t)

	// Arrange
	restore := runner.SetLeftoverGrace(50 * time.Millisecond)
	defer restore()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          (trap '' TERM; exec tail -f /dev/null) &
		          echo $! > job.pid
		      - run: exit 1
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	waitGone(t, readPid(t, res.Dir, "job.pid"))
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
	// step ends with the job still running, then into: exit is the shell's
	// own code (why: the status is read from the shell while hobnob holds it
	// unreaped for the job's sake)
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

func TestE2E_RunBackground_QuietStepsJobStaysQuiet(t *testing.T) {
	// given quiet: on a step that leaves a job running, when the job prints
	// after the step has succeeded, then that output stays hidden too (why:
	// a quiet step's job writes to the same pipes as the step)
	skipWithoutProcessGroups(t)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          mkfifo gate
		          echo hidden
		          (read _ < gate; echo late) &
		        quiet: true
		      - run: |
		          echo 1<> gate
		          echo next
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "next")
}

func TestE2E_RunBackground_SecondCtrlCKillsJobsHobnobIsWaitingOn(t *testing.T) {
	// given a job that ignores SIGTERM, left running by an earlier step, when
	// a 2nd CTRL+C (KillRunningStep) arrives as the run ends, then it kills
	// the job at once instead of waiting out the grace (why: the 2nd CTRL+C
	// reaches every job the run has left, not only the running step)
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
		// Give the last step time to exit after writing the marker. If the
		// kill lands first anyway, it ends that step too and soft: lets the
		// run finish, so the test can't flake; only a kill that can't reach
		// the job makes the run wait out the minute and fail.
		time.Sleep(100 * time.Millisecond)
		runner.KillRunningStep()
	}()
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          (trap '' TERM; exec tail -f /dev/null) &
		          echo $! > job.pid
		      - run: touch "$MARKER"
		        soft: true
	`

	// Act
	start := time.Now()
	res := Run(t, Case{Yml: yml, Args: []string{"t"}, Env: map[string]string{"MARKER": marker}})

	// Assert
	res.OK(t)
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("run took %s: the 2nd CTRL+C didn't reach the job", elapsed)
	}
	waitGone(t, readPid(t, res.Dir, "job.pid"))
}
