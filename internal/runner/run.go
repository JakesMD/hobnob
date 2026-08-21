package runner

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

func execRun(execState execCtx, step config.Step, scope *cli.Scope) error {
	var shellCmd *osExec.Cmd
	var displayCmd string
	if len(step.Argv) > 0 {
		argv, err := eval.ResolveArgv(step.Argv, scope.Vars)
		if err != nil {
			return fmt.Errorf("run argv: %w", err)
		}
		shellCmd = osExec.CommandContext(execState.ctx, argv[0], argv[1:]...)
		displayCmd = displayArgv(argv)
	} else {
		cmd, err := eval.EvalTemplate(step.Command, scope.Vars)
		if err != nil {
			return fmt.Errorf("run template: %w", err)
		}
		shellCmd = osExec.CommandContext(execState.ctx, "sh", "-c", cmd)
		displayCmd = cmd
	}

	runDir := execState.dir
	displayDir := ""
	if step.DirTmpl != "" {
		resolved, err := eval.EvalTemplate(step.DirTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("run dir template: %w", err)
		}
		runDir = resolveDirPath(resolved, execState.cfg.TaskfileDir)
		displayDir = displayDirPath(runDir, scope.Vars["HOBNOB_INVOCATION_DIR"].String())
	}

	displayCmd = maskSecrets(displayCmd, scope)
	for _, displayLine := range tui.RunDisplayLines(displayCmd, execState.task, displayDir) {
		fmt.Println(displayLine)
	}
	if step.Quiet {
		quietMsg, err := eval.EvalTemplate(step.QuietMsg, scope.Vars)
		if err != nil {
			return fmt.Errorf("quiet template: %w", err)
		}
		fmt.Println(tui.RunQuietLine(execState.task, maskSecrets(quietMsg, scope)))
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
	shellCmd.Cancel = cancelFunc(shellCmd)
	// No WaitDelay: graceful shutdown waits for the step to exit on its own
	// timeline, however long that takes. A step that ignores SIGTERM only
	// stops via a 2nd CTRL+C, handled by KillRunningStep below.
	shellCmd.Dir = runDir

	var stdoutBuf, stderrBuf bytes.Buffer
	switch {
	case step.Quiet:
		// Buffered only — never teed to the LineWriters live. On failure,
		// below, the buffers are replayed through them so a hidden step
		// never fails silently.
		shellCmd.Stdout = &stdoutBuf
		shellCmd.Stderr = &stderrBuf
	case len(step.IntoEntries) > 0:
		shellCmd.Stdout = io.MultiWriter(stdoutLineWriter, &stdoutBuf)
		shellCmd.Stderr = io.MultiWriter(stderrLineWriter, &stderrBuf)
	default:
		shellCmd.Stdout = stdoutLineWriter
		shellCmd.Stderr = stderrLineWriter
	}
	shellCmd.Env = envWithScopeOverrides(scope.Vars)

	err := shellCmd.Start()
	if err != nil {
		// The process never ran — no exit status exists, so there's nothing
		// for into: exit to capture.
		return err
	}
	setRunningPID(shellCmd.Process.Pid)
	waitErr := shellCmd.Wait()
	clearRunningPID()

	if step.Quiet && waitErr != nil {
		stdoutLineWriter.Write(stdoutBuf.Bytes())
		stderrLineWriter.Write(stderrBuf.Bytes())
	}
	stdoutLineWriter.Flush()
	stderrLineWriter.Flush()
	if waitErr != nil && execState.ctx.Err() != nil {
		return fmt.Errorf("%w: %v", ErrInterrupted, waitErr)
	}

	captureErr := captureRunInto(step.IntoEntries, scope, stdoutBuf.String(), stderrBuf.String(), exitCodeOf(waitErr))
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

// captureRunInto stores each into: result as a typed Value straight into
// scope — a captured JSON array/object (see value.Capture, called inside
// EvalRunIntoPipe) lands as a real Array/Object, not text, which is what
// lets loop: iterate it without re-parsing.
func captureRunInto(entries []config.IntoEntry, scope *cli.Scope, stdout, stderr string, exitCode int) error {
	evalLeaf := func(expr string) (value.Value, error) {
		return eval.EvalRunIntoPipe(expr, stdout, stderr, exitCode, scope.Vars)
	}
	for _, intoEntry := range entries {
		var val value.Value
		var err error
		if intoEntry.ValNode != nil {
			val, err = config.EvalJSONNode(*intoEntry.ValNode, evalLeaf)
		} else {
			val, err = evalLeaf(intoEntry.ValueTmpl)
		}
		if err != nil {
			return fmt.Errorf("run into %q: %w", intoEntry.ParentKey, err)
		}
		scope.Vars[intoEntry.ParentKey] = val
	}
	return nil
}
