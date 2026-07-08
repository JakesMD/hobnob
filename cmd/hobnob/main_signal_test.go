//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

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

func TestSigint_GracefulShutdown_WaitsForStepToExit(t *testing.T) {
	// given a run: step that takes time to shut down after receiving SIGTERM,
	// when SIGINT arrives once, then hobnob prints the shutdown notice and
	// waits for that cleanup to finish on its own instead of killing it
	// immediately (why: documented CTRL+C contract in GUIDE.md — "waits for
	// it to exit on its own, however long that takes")
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
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
	cmd := exec.Command(os.Args[0], "-test.run=TestSigint_GracefulShutdown_WaitsForStepToExit")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
	var stderr strings.Builder
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
	err := cmd.Wait()
	elapsed := time.Since(sigAt)

	// Assert
	if err == nil {
		t.Fatal("expected non-zero exit for interrupted run, got success")
	}
	if elapsed < 600*time.Millisecond {
		t.Errorf("expected hobnob to wait out the step's ~0.8s cleanup before exiting, only took %v", elapsed)
	}
	if elapsed > 3*time.Second {
		t.Errorf("expected graceful shutdown to finish shortly after the step's cleanup, took %v", elapsed)
	}
	if !strings.Contains(stderr.String(), "shutting down") {
		t.Errorf("expected stderr to contain shutdown notice, got: %s", stderr.String())
	}
}

func TestSigint_SecondPress_ForceKillsImmediately(t *testing.T) {
	// given a run: step that would take ~0.8s to clean up after SIGTERM, when
	// a 2nd SIGINT arrives before that cleanup finishes, then hobnob
	// force-kills the step and exits immediately rather than waiting for it
	// (why: documented "press CTRL+C again to force-kill the running step
	// immediately" behavior — the 2nd press must cut the graceful wait short)
	if os.Getenv("hobnob_SUBPROCESS") == "1" {
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
	cmd := exec.Command(os.Args[0], "-test.run=TestSigint_SecondPress_ForceKillsImmediately")
	cmd.Env = append(os.Environ(), "hobnob_SUBPROCESS=1", "HOBNOB_TEST_DIR="+dir)
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
}
