package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"strings"
	"sync/atomic"
	"time"

	"hobnob/internal/config"
	"hobnob/internal/scope"
	"hobnob/internal/value"
)

// skippedRunText is the command a skipped run: step would have run, for the
// skip line; the command as written when it can't be rendered.
func skippedRunText(step config.Step, scope *scope.Scope) string {
	if _, display, err := renderRun(step, scope); err == nil {
		return display
	}
	if !step.Argv.IsZero() {
		return scope.Mask(displayArgv(step.Argv.Raw()))
	}
	return scope.Mask(step.Command.String())
}

// startFailure says why a command couldn't start in hobnob's terms, rather
// than os/exec's: fork/exec blames the shell when the directory is what's
// missing.
func startFailure(dir, invocationDir string, err error) error {
	if _, statErr := os.Stat(dir); errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("no directory %s", displayDirPath(dir, invocationDir))
	}
	if errors.Is(err, osExec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return errors.New("no such command")
	}
	return err
}

// renderRun renders a run: step into the argv to execute and the masked
// command to show.
func renderRun(step config.Step, scope *scope.Scope) (argv []string, display string, err error) {
	if !step.Argv.IsZero() {
		argv, err := step.Argv.Resolve(scope.Vars())
		if err != nil {
			return nil, "", err
		}
		if len(argv) == 0 {
			return nil, "", fmt.Errorf("run argv: no command")
		}
		return argv, scope.Mask(displayArgv(argv)), nil
	}
	cmd, err := step.Command.Text(scope.Vars())
	if err != nil {
		return nil, "", err
	}
	return []string{"sh", "-c", cmd}, scope.Mask(cmd), nil
}

func execRun(execState execCtx, step config.Step, scope *scope.Scope) error {
	argv, displayCmd, err := renderRun(step, scope)
	if err != nil {
		return err
	}
	shellCmd := osExec.CommandContext(execState.ctx, argv[0], argv[1:]...)

	runDir := execState.dir
	displayDir := ""
	if !step.DirTmpl.IsZero() {
		resolved, err := step.DirTmpl.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("dir: %w", err)
		}
		runDir = resolveDirPath(resolved, execState.cfg.TaskfileDir)
		displayDir = displayDirPath(runDir, scope.InvocationDir())
	}

	var notes []string
	if displayDir != "" {
		notes = append(notes, "in "+strings.TrimSuffix(displayDir, "/")+"/")
	}
	if step.Quiet {
		quietMsg, err := step.QuietMsg.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("quiet: %w", err)
		}
		hint := "output hidden"
		if msg := scope.Mask(quietMsg); msg != "" {
			hint = msg + "… (" + hint + ")"
		}
		notes = append(notes, hint)
	}
	execState.log.Run(execState.task, displayCmd, notes...)
	began := time.Now()
	end := func(err error) error { return endRunStep(execState, step, displayCmd, began, err) }
	stdoutLineWriter, stderrLineWriter := execState.log.Output(execState.task)
	// setProcAttr (unix) puts the child in its own process group so
	// cancelFunc can signal the whole group — otherwise SIGTERM only reaches
	// the direct child, leaving any children it forked (multi-command
	// scripts, pipelines, background jobs) running. No-op on windows, which
	// has no process groups in this sense.
	setProcAttr(shellCmd)
	// Once the process has exited, a first CTRL+C leaves it alone: signalling
	// the zombie would make Wait misreport a finished step as interrupted.
	// termSent tells stopLeftovers whether the group already got its SIGTERM.
	var exited, termSent atomic.Bool
	cancelGroup := cancelFunc(shellCmd)
	shellCmd.Cancel = func() error {
		if exited.Load() {
			return os.ErrProcessDone
		}
		termSent.Store(true)
		return cancelGroup()
	}
	// No WaitDelay: graceful shutdown waits for the step to exit on its own
	// timeline, however long that takes. A step that ignores SIGTERM only
	// stops via a 2nd CTRL+C, handled by KillRunningStep below.
	shellCmd.Dir = runDir

	var stdoutBuf, stderrBuf bytes.Buffer
	var stdoutDest, stderrDest io.Writer
	holdOutput := step.Quiet || execState.log.Quiet()
	switch {
	case holdOutput:
		// Buffered only — never teed to the LineWriters live. On failure,
		// below, the buffers are replayed through them so a hidden step
		// never fails silently.
		stdoutDest, stderrDest = &stdoutBuf, &stderrBuf
	case len(step.IntoEntries) > 0:
		stdoutDest = io.MultiWriter(stdoutLineWriter, &stdoutBuf)
		stderrDest = io.MultiWriter(stderrLineWriter, &stderrBuf)
	default:
		stdoutDest, stderrDest = stdoutLineWriter, stderrLineWriter
	}
	shellCmd.Env = envWithScopeOverrides(scope.Vars())

	var size *ptySize
	if cols, rows, watched := execState.log.PTYSize(execState.task); watched {
		size = &ptySize{cols, rows}
	}
	pipes, err := attachPipes(shellCmd, size)
	if err != nil {
		return end(err)
	}
	// The command shares hobnob's stdin, so read, sudo and [y/N] prompts work.
	stopStdin := attachStdin(shellCmd, pipes)
	defer stopStdin()
	if size != nil {
		stopResizing := execState.log.OnResize(func() {
			if cols, rows, ok := execState.log.PTYSize(execState.task); ok {
				pipes.resize(ptySize{cols, rows})
			}
		})
		defer stopResizing()
	}
	if err := shellCmd.Start(); err != nil {
		// The process never ran — no exit status exists, so there's nothing
		// for into: exit to capture.
		pipes.abandon()
		return end(startFailure(shellCmd.Dir, scope.InvocationDir(), err))
	}
	pipes.startCopying(stdoutDest, stderrDest)
	pid := shellCmd.Process.Pid
	setRunningPID(pid)
	// Output arriving after the shell exits is a backgrounded job's: it still
	// shows under the step's prefix, but goes into neither into: nor a quiet
	// step's buffers.
	var lateStdout, lateStderr io.Writer = stdoutLineWriter, stderrLineWriter
	if holdOutput {
		lateStdout, lateStderr = io.Discard, io.Discard
	}
	kept := &leftover{reap: shellCmd.Wait, pipes: pipes, flush: func() {
		stdoutLineWriter.Flush()
		stderrLineWriter.Flush()
	}}
	var waitErr error
	if waitExitUnreaped(pid) {
		exited.Store(true)
		pipes.handoff(lateStdout, lateStderr)
		status, ok := shellStatus(pid)
		if ok && groupHasOthers(pid) {
			// Jobs are still running in the step's process group. The shell
			// stays a zombie until the run ends, so the group ID can't be
			// reused while stopLeftovers or a 2nd CTRL+C may signal it.
			waitErr = status
			kept.group, kept.termSent = pid, termSent.Load()
			keepLeftover(kept)
			clearRunningPID()
		} else {
			clearRunningPID()
			waitErr = shellCmd.Wait()
		}
	} else {
		// No way to hold the PID here, so it's reaped and released first,
		// and the leftovers can't be signalled.
		waitErr = shellCmd.Wait()
		clearRunningPID()
		pipes.handoff(lateStdout, lateStderr)
	}
	stopStdin()
	if kept.group == 0 && !pipes.closed() {
		keepLeftover(kept)
	}

	pipes.withLock(func() {
		if holdOutput && waitErr != nil {
			stdoutLineWriter.Write(stdoutBuf.Bytes())
			stderrLineWriter.Write(stderrBuf.Bytes())
		}
		kept.flush()
	})
	if waitErr != nil && execState.ctx.Err() != nil {
		return end(fmt.Errorf("%w: %v", ErrInterrupted, waitErr))
	}

	captureErr := captureInto("run", step.IntoEntries, runSource(stdoutBuf.String(), stderrBuf.String(), exitCodeOf(waitErr)), scope, nil)
	return end(errors.Join(waitErr, captureErr))
}

// endRunStep logs how a run: step ended, soft or not, and marks a failure as
// logged.
func endRunStep(execState execCtx, step config.Step, cmd string, began time.Time, err error) error {
	elapsed := time.Since(began)
	if err == nil {
		execState.log.StepDone(execState.task, elapsed)
		return nil
	}
	firstLine, _, _ := strings.Cut(cmd, "\n")
	reason := strings.ReplaceAll(err.Error(), "\n", "; ")
	switch {
	case errors.Is(err, ErrInterrupted):
		execState.log.Stopping()
		execState.log.StepFailed(execState.task, elapsed, firstLine, "stopped")
	case step.Soft:
		execState.log.StepSoftFailed(execState.task, elapsed, firstLine, reason)
	default:
		execState.log.StepFailed(execState.task, elapsed, firstLine, reason)
	}
	return reportedError{err}
}

// exitCodeOf reports a finished command's exit code — 0 on success, the
// child's code on an ordinary non-zero exit, or -1 when it was killed by a
// signal or otherwise didn't report a code (os/exec's own convention).
func exitCodeOf(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(waitErr, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// envWithScopeOverrides strips any os.Environ() var also present in scope
// before appending scope's vars — os/exec uses first-occurrence-wins for Env,
// and scope vars must win over inherited env. A structured var is exported
// as its compact JSON text (Value.String()), same as it renders in {{ }}.
func envWithScopeOverrides(vars map[string]value.Value) []string {
	scopeKeys := make(map[string]bool, len(vars))
	for varName := range vars {
		scopeKeys[varName] = true
	}
	env := os.Environ()
	filtered := env[:0:len(env)]
	for _, envEntry := range env {
		varName, _, _ := strings.Cut(envEntry, "=")
		if !scopeKeys[varName] {
			filtered = append(filtered, envEntry)
		}
	}
	for varName, varValue := range vars {
		filtered = append(filtered, varName+"="+varValue.String())
	}
	return filtered
}
