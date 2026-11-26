//go:build unix

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hobnob/internal/runevents"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

// watchedLog is a RunLog on a fixed 80x24 screen, so steps get a PTY.
func watchedLog() *tui.RunLog {
	log := tui.NewRunLog(os.Stdout, os.Stderr)
	log.Watch(&tui.Screen{
		Size:     func() (int, int) { return 80, 24 },
		OnResize: func(func()) func() { return func() {} },
	})
	return log
}

func TestExecRun_PTY_CtxCancelled_KillsWholeProcessGroup(t *testing.T) {
	// given a run: step on a PTY whose shell forks a background child, when
	// ctx is cancelled, then the step is interrupted and the child killed too
	// (why: the process-group handling must hold when stdout is a PTY)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(t, fmt.Sprintf("(sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	log := watchedLog()
	err := ExecuteTask(ctx, "t", scope.New(), nil, cfg, true, dir, runevents.New(log), log, TerminalPrompts())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // give a leaked child a chance to finish
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("background child kept running after ctx cancellation on a PTY")
	}
}

func TestKillRunningStep_PTY_ForceKillsGroupThatIgnoredSIGTERM(t *testing.T) {
	// given a run: step on a PTY whose shell and background child ignore
	// SIGTERM, when ctx is cancelled and KillRunningStep is then called, then
	// the whole group is force-killed (why: a 2nd CTRL+C must still reach a
	// stuck step when its stdout is a PTY)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(t, fmt.Sprintf("trap '' TERM; (sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	log := watchedLog()
	done := make(chan error, 1)
	go func() {
		done <- ExecuteTask(ctx, "t", scope.New(), nil, cfg, true, dir, runevents.New(log), log, TerminalPrompts())
	}()
	time.Sleep(150 * time.Millisecond) // let ctx cancel and the ignored SIGTERM land

	// Act
	KillRunningStep()

	// Assert
	select {
	case err := <-done:
		if !errors.Is(err, ErrInterrupted) {
			t.Fatalf("expected ErrInterrupted, got: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ExecuteTask did not return after KillRunningStep")
	}
	time.Sleep(200 * time.Millisecond) // give a leaked child a chance to finish
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("background child kept running after KillRunningStep on a PTY")
	}
}
