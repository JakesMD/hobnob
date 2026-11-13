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

	"hobnob/internal/config"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

func execRun(execState execCtx, step config.Step, scope *scope.Scope) error {
	var shellCmd *osExec.Cmd
	var displayCmd string
	if !step.Argv.IsZero() {
		argv, err := step.Argv.Resolve(scope.Vars())
		if err != nil {
			return fmt.Errorf("run argv: %w", err)
		}
		shellCmd = osExec.CommandContext(execState.ctx, argv[0], argv[1:]...)
		displayCmd = displayArgv(argv)
	} else {
		cmd, err := step.Command.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("run template: %w", err)
		}
		shellCmd = osExec.CommandContext(execState.ctx, "sh", "-c", cmd)
		displayCmd = cmd
	}

	runDir := execState.dir
	displayDir := ""
	if !step.DirTmpl.IsZero() {
		resolved, err := step.DirTmpl.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("run dir template: %w", err)
		}
		runDir = resolveDirPath(resolved, execState.cfg.TaskfileDir)
		displayDir = displayDirPath(runDir, scope.Vars()["HOBNOB_INVOCATION_DIR"].String())
	}

	displayCmd = scope.Mask(displayCmd)
	for _, displayLine := range tui.RunDisplayLines(displayCmd, execState.task, displayDir) {
		fmt.Println(displayLine)
	}
	if step.Quiet {
		quietMsg, err := step.QuietMsg.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("quiet template: %w", err)
		}
		fmt.Println(tui.RunQuietLine(execState.task, scope.Mask(quietMsg)))
	}
	prefix := tui.TaskPrefix(execState.task)
	stdoutLineWriter := tui.NewLineWriter(os.Stdout, prefix)
	stderrLineWriter := tui.NewLineWriter(os.Stderr, prefix)
	// setProcAttr (unix) puts the child in its own process group so
	// cancelFunc can signal the whole group — otherwise SIGTERM only reaches
	// the direct child, leaving any children it forked (multi-command
	// scripts, pipelines, background jobs) running. No-op on windows, which
	// has no process groups in this sense.
	setProcAttr(shellCmd)
	// Once the process has exited, a first CTRL+C leaves it alone: settle
	// SIGTERMs the leftovers itself, and signalling the zombie would make
	// Wait misreport a finished step as interrupted. termSent tells settle
	// whether the group already got its SIGTERM.
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
	switch {
	case step.Quiet:
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

	pipes, err := attachPipes(shellCmd)
	if err != nil {
		return err
	}
	if err := shellCmd.Start(); err != nil {
		// The process never ran — no exit status exists, so there's nothing
		// for into: exit to capture.
		pipes.abandon()
		return err
	}
	pipes.startCopying(stdoutDest, stderrDest)
	pid := shellCmd.Process.Pid
	setRunningPID(pid)
	var waitErr error
	if waitExitUnreaped(pid) {
		// The exited process stays a zombie through settle, so its PID and
		// process group ID can't be reused yet: settle and a 2nd CTRL+C can
		// both safely signal the leftovers. The PID is cleared before Wait
		// reaps it.
		exited.Store(true)
		pipes.settle(pid, termSent.Load())
		clearRunningPID()
		waitErr = shellCmd.Wait()
	} else {
		// No way to hold the PID here, so it's reaped and released first,
		// and the leftovers can't be signalled.
		waitErr = shellCmd.Wait()
		clearRunningPID()
		pipes.settle(0, false)
	}

	if step.Quiet && waitErr != nil {
		stdoutLineWriter.Write(stdoutBuf.Bytes())
		stderrLineWriter.Write(stderrBuf.Bytes())
	}
	stdoutLineWriter.Flush()
	stderrLineWriter.Flush()
	if waitErr != nil && execState.ctx.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInterrupted, waitErr)
	}

	captureErr := captureInto("run", step.IntoEntries, runSource(stdoutBuf.String(), stderrBuf.String(), exitCodeOf(waitErr)), scope, nil)
	return errors.Join(waitErr, captureErr)
}

// exitCodeOf reports a finished command's exit code — 0 on success, the
// child's code on an ordinary non-zero exit, or -1 when it was killed by a
// signal or otherwise didn't report a code (os/exec's own convention).
func exitCodeOf(waitErr error) int {
	if waitErr == nil {
		return 0
	}
	var exitErr *osExec.ExitError
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
