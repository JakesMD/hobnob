package runner

import (
	"bytes"
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
	cmd, err := eval.EvalTemplate(step.Command, scope.Vars)
	if err != nil {
		return fmt.Errorf("run template: %w", err)
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

	displayCmd := maskSecrets(cmd, scope)
	for _, displayLine := range tui.RunDisplayLines(displayCmd, execState.task, displayDir) {
		fmt.Println(displayLine)
	}
	prefix := tui.TaskPrefix(execState.task)
	stdoutLineWriter := tui.NewLineWriter(os.Stdout, prefix)
	stderrLineWriter := tui.NewLineWriter(os.Stderr, prefix)
	shellCmd := osExec.CommandContext(execState.ctx, "sh", "-c", cmd)
	// setProcAttr (unix) puts sh in its own process group so cancelFunc can
	// signal the whole group — otherwise SIGTERM only reaches sh itself,
	// leaving any children it forked (multi-command scripts, pipelines,
	// background jobs) running. No-op on windows, which has no process
	// groups in this sense.
	setProcAttr(shellCmd)
	shellCmd.Cancel = cancelFunc(shellCmd)
	// No WaitDelay: graceful shutdown waits for the step to exit on its own
	// timeline, however long that takes. A step that ignores SIGTERM only
	// stops via a 2nd CTRL+C, handled by KillRunningStep below.
	shellCmd.Dir = runDir

	var stdoutBuf, stderrBuf bytes.Buffer
	if len(step.IntoEntries) > 0 {
		shellCmd.Stdout = io.MultiWriter(stdoutLineWriter, &stdoutBuf)
		shellCmd.Stderr = io.MultiWriter(stderrLineWriter, &stderrBuf)
	} else {
		shellCmd.Stdout = stdoutLineWriter
		shellCmd.Stderr = stderrLineWriter
	}
	shellCmd.Env = envWithScopeOverrides(scope.Vars)

	err = shellCmd.Start()
	if err == nil {
		setRunningPID(shellCmd.Process.Pid)
		err = shellCmd.Wait()
		clearRunningPID()
	}
	stdoutLineWriter.Flush()
	stderrLineWriter.Flush()
	if err != nil {
		if execState.ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, err)
		}
		return err
	}

	return captureRunInto(step.IntoEntries, scope, stdoutBuf.String(), stderrBuf.String())
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
func captureRunInto(entries []config.IntoEntry, scope *cli.Scope, stdout, stderr string) error {
	evalLeaf := func(expr string) (value.Value, error) {
		return eval.EvalRunIntoPipe(expr, stdout, stderr)
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
