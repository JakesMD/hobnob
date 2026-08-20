package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"hobnob/internal/config"
	"hobnob/internal/value"
)

func makeRunCfg(command string, into []config.IntoEntry) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{Kind: config.KindRun, Command: command, IntoEntries: into}}},
		},
	}
}

func makeArgvRunCfg(argv []string) *config.ConfigFile {
	return &config.ConfigFile{
		Tasks: map[string]config.Task{
			"t": {Steps: []config.Step{{Kind: config.KindRun, Argv: argv}}},
		},
	}
}

func TestExecRun_CtxCancelled_ReturnsErrInterrupted(t *testing.T) {
	// given a run: step whose command is still executing, when ctx is
	// cancelled, then the returned error wraps ErrInterrupted (why: callers
	// need to distinguish a graceful shutdown from an ordinary command failure)

	// Arrange
	cfg := makeRunCfg("sleep 5", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}

func TestExecRun_OrdinaryFailure_NotWrappedAsInterrupted(t *testing.T) {
	// given a run: step that fails on its own (ctx never cancelled), when
	// executed, then the error is returned as-is, not wrapped as
	// ErrInterrupted (why: regression guard so ordinary command failures keep
	// their original exit-status error instead of being misreported as a
	// graceful shutdown)

	// Arrange
	cfg := makeRunCfg("exit 3", nil)

	// Act
	err := ExecuteTask(context.Background(), "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrInterrupted) {
		t.Fatalf("ordinary failure should not be wrapped as ErrInterrupted, got: %v", err)
	}
}

func TestExecRun_CtxCancelled_KillsWholeProcessGroup(t *testing.T) {
	// given a run: step whose shell forks a background child (not just the
	// tail-exec'd sh itself), when ctx is cancelled, then the child is killed
	// too (why: shellCmd.Cancel signals the whole process group so multi-
	// command scripts/pipelines/background jobs don't outlive the graceful
	// shutdown)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(fmt.Sprintf("(sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, dir)

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
	time.Sleep(200 * time.Millisecond) // give a leaked child a chance to finish
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("background child kept running after ctx cancellation — process group was not killed")
	}
}

func TestKillRunningStep_ForceKillsGroupThatIgnoredSIGTERM(t *testing.T) {
	// given a run: step whose shell (and its background child) ignore
	// SIGTERM, when ctx is cancelled (graceful shutdown) and KillRunningStep
	// is then called (2nd CTRL+C), then the whole still-running process group
	// is force-killed (why: regression test for the bug where the 2nd
	// CTRL+C targeted hobnob's own process group instead of the step's —
	// see cmd/hobnob/main.go)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeRunCfg(fmt.Sprintf("trap '' TERM; (sleep 5; touch %s) & wait", marker), nil)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, dir)
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
		t.Fatal("background child kept running after KillRunningStep — force-kill did not reach the step's process group")
	}
}

func TestExecRun_Argv_CtxCancelled_ReturnsErrInterrupted(t *testing.T) {
	// given a list-form run: step (no shell involved — CommandContext is
	// built from argv, not "sh -c"), when ctx is cancelled, then the same
	// ErrInterrupted wrapping applies as the string form (why: setProcAttr /
	// cancelFunc / the ctx.Err() check are shared by both branches of
	// execRun — this is the regression guard for that sharing)

	// Arrange
	cfg := makeArgvRunCfg([]string{"sleep", "5"})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Act
	err := ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, t.TempDir())

	// Assert
	if !errors.Is(err, ErrInterrupted) {
		t.Fatalf("expected ErrInterrupted, got: %v", err)
	}
}

func TestKillRunningStep_Argv_ForceKillsGroupThatIgnoredSIGTERM(t *testing.T) {
	// given a list-form run: step whose direct child (sh, invoked as argv[0]
	// rather than via the hardcoded "sh -c" branch) ignores SIGTERM, when ctx
	// cancels and KillRunningStep is then called, then the process group is
	// force-killed — same as the string form (why: proves setProcAttr's
	// Setpgid and cancelFunc's group-kill apply regardless of which branch
	// constructed the *exec.Cmd)

	// Arrange
	dir := t.TempDir()
	marker := filepath.Join(dir, "child-finished")
	cfg := makeArgvRunCfg([]string{"sh", "-c", fmt.Sprintf("trap '' TERM; (sleep 5; touch %s) & wait", marker)})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- ExecuteTask(ctx, "t", makeScope(map[string]value.Value{}), cfg, true, dir)
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
		t.Fatal("background child kept running after KillRunningStep — force-kill did not reach the step's process group")
	}
}

func TestKillRunningStep_NoStepRunning_NoOp(t *testing.T) {
	// given no run: step is currently executing, when KillRunningStep is
	// called, then it does nothing and does not panic (why: the 2nd-CTRL+C
	// handler must be able to call it unconditionally, including when
	// hobnob is idle e.g. blocked on a get: prompt)

	// Act / Assert
	KillRunningStep()
}
