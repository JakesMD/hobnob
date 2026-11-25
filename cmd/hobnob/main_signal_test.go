//go:build !windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// reExecSelf builds the subprocess command that re-runs t as the sole test in
// this binary. Anchored and quoted from t.Name() rather than a hand-typed
// string literal — a hand-typed "-test.run=TestFoo" silently desyncs from a
// renamed test: the child then matches zero tests, exits 0, and the parent
// reports a false pass.
func reExecSelf(t *testing.T) *exec.Cmd {
	t.Helper()
	return exec.Command(os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$")
}

// waitForFile polls until path exists or t fails the test.
func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

// gracefulCleanupScript traps SIGTERM (what hobnob sends on the 1st CTRL+C)
// and spends cleanupDur "shutting down" before exiting on its own, instead of
// dying the instant the signal arrives. This is what distinguishes a graceful
// wait from a hard kill: SIGTERM's default disposition is to terminate a
// process immediately, so a step that dies right away wouldn't prove hobnob
// waited for anything — it would look identical to a force-kill.
const gracefulCleanupScript = `trap 'sleep 0.8; exit 0' TERM; touch %s; sleep 5`

// childStartedSentinel is printed by the subprocess branch right before it
// calls main(). If the -test.run regex in reExecSelf ever stops matching (a
// rename, a typo), the child process exits 0 having run zero tests, and every
// assertion below it would silently pass against empty output. Asserting
// this sentinel is present turns that failure mode into a loud one.
const childStartedSentinel = "HOBNOB_E2E_CHILD_STARTED"

func TestSigint_GracefulShutdown_WaitsForStepToExit(t *testing.T) {
	// given a run: step that takes time to shut down after receiving SIGTERM,
	// when SIGINT arrives once, then hobnob prints the shutdown notice and
	// waits for that cleanup to finish on its own instead of killing it
	// immediately (why: documented CTRL+C contract in docs/reference/cli.md — "waits for
	// it to exit on its own, however long that takes")
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		fmt.Fprintln(os.Stderr, childStartedSentinel)
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "t"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	taskYAML := "tasks:\n  t:\n    steps:\n      - run: " + fmt.Sprintf(gracefulCleanupScript, started) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte(taskYAML), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := reExecSelf(t)
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForFile(t, started)

	// Act
	sigAt := time.Now()
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Assert
	// Upper bound is a generous hang-detector, not a performance assertion —
	// CI scheduling jitter shouldn't fail a test whose contract is "waits,
	// then exits", only a genuine hang should.
	var err error
	select {
	case err = <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		t.Fatal("hobnob did not exit after a 1st SIGINT — graceful shutdown hung")
	}
	elapsed := time.Since(sigAt)

	if err == nil {
		t.Fatal("expected non-zero exit for interrupted run, got success")
	}
	if elapsed < 600*time.Millisecond {
		t.Errorf("expected hobnob to wait out the step's ~0.8s cleanup before exiting, only took %v", elapsed)
	}
	if !strings.Contains(stdout.String(), "t ! stopping, ctrl+c again to force") {
		t.Errorf("expected stdout to contain the stopping line, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), childStartedSentinel) {
		t.Fatalf("subprocess never started (missing %s) — -test.run likely matched zero tests; stderr: %s", childStartedSentinel, stderr.String())
	}
}

func TestSigint_SecondPress_ForceKillsImmediately(t *testing.T) {
	// given a run: step that would take ~0.8s to clean up after SIGTERM, when
	// a 2nd SIGINT arrives before that cleanup finishes, then hobnob
	// force-kills the step and exits immediately rather than waiting for it
	// (why: documented "press CTRL+C again to force-kill the running step
	// immediately" behavior — the 2nd press must cut the graceful wait short)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		fmt.Fprintln(os.Stderr, childStartedSentinel)
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "t"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	started := filepath.Join(dir, "started")
	taskYAML := "tasks:\n  t:\n    steps:\n      - run: " + fmt.Sprintf(gracefulCleanupScript, started) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte(taskYAML), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := reExecSelf(t)
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForFile(t, started)

	// Act
	start := time.Now()
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("1st signal: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("2nd signal: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Assert
	select {
	case <-done:
		elapsed := time.Since(start)
		if elapsed > 600*time.Millisecond {
			t.Errorf("expected 2nd CTRL+C to force-exit before the step's ~0.8s cleanup finishes, took %v", elapsed)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		t.Fatal("hobnob did not exit after a 2nd SIGINT — force-kill path is broken")
	}
	if !strings.Contains(stderr.String(), childStartedSentinel) {
		t.Fatalf("subprocess never started (missing %s) — -test.run likely matched zero tests; stderr: %s", childStartedSentinel, stderr.String())
	}
}

// readPidFile reads the PID a fixture wrote to path.
func readPidFile(t *testing.T, path string) int {
	t.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(text)))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return pid
}

// waitPidGone fails the test unless pid is gone within 5s, killing it first
// so a failing test leaves nothing behind.
func waitPidGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Fatalf("background job %d outlived hobnob", pid)
}

func TestSighup_KillsBackgroundJobs(t *testing.T) {
	// given a job ignoring SIGTERM that an earlier step left running, when
	// hobnob gets SIGHUP (its terminal closed), then hobnob exits and the
	// job is gone (why: background jobs never outlive hobnob, and closing
	// the terminal is a way hobnob exits)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		fmt.Fprintln(os.Stderr, childStartedSentinel)
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "t"}
		main()
		return
	}

	// Arrange
	dir := t.TempDir()
	taskYAML := `tasks:
  t:
    steps:
      - run: |
          (trap '' TERM; exec tail -f /dev/null) &
          echo $! > job.pid
      - run: |
          touch started
          exec tail -f /dev/null
`
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte(taskYAML), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := reExecSelf(t)
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForFile(t, filepath.Join(dir, "started"))
	jobPid := readPidFile(t, filepath.Join(dir, "job.pid"))

	// Act
	if err := cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Assert
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		t.Fatal("hobnob did not exit after SIGHUP")
	}
	if !strings.Contains(stderr.String(), childStartedSentinel) {
		t.Fatalf("subprocess never started (missing %s) — -test.run likely matched zero tests; stderr: %s", childStartedSentinel, stderr.String())
	}
	waitPidGone(t, jobPid)
}

func TestSigint_FirstPress_StopsEarlierStepsJobs(t *testing.T) {
	// given a job that an earlier step left running and a step that ignores
	// SIGTERM, when SIGINT arrives once, then the job gets its SIGTERM while
	// that step is still running (why: CTRL+C stops the whole run, not just
	// the step in front of it)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
		fmt.Fprintln(os.Stderr, childStartedSentinel)
		dir := os.Getenv("HOBNOB_TEST_DIR")
		os.Args = []string{"hobnob", "--file", filepath.Join(dir, "hobnob.yml"), "t"}
		main()
		return
	}

	// Arrange
	// The fifo holds the first step until the job's trap is armed. The second
	// step waits up to 3s for the job to be stopped, and records whether it was.
	dir := t.TempDir()
	taskYAML := `tasks:
  t:
    steps:
      - run: |
          mkfifo armed
          (trap 'touch stopped; exit 0' TERM; tail -f /dev/null & echo > armed; wait) &
          read _ < armed
      - run: |
          trap '' TERM
          touch started
          for _ in $(seq 300); do
            [ -f stopped ] && touch seen && exit 0
            sleep 0.01
          done
`
	if err := os.WriteFile(filepath.Join(dir, "hobnob.yml"), []byte(taskYAML), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := reExecSelf(t)
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForFile(t, filepath.Join(dir, "started"))

	// Act
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("signal: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	// Assert
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		t.Fatal("hobnob did not exit after a 1st SIGINT")
	}
	if !strings.Contains(stderr.String(), childStartedSentinel) {
		t.Fatalf("subprocess never started (missing %s) — -test.run likely matched zero tests; stderr: %s", childStartedSentinel, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "seen")); err != nil {
		t.Fatal("the earlier step's job wasn't stopped until the running step ended")
	}
}
