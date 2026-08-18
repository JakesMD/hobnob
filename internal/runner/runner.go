package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strings"
	"sync"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
)

// promptTextFn and promptSelectFn are package-level so tests can substitute fakes.
var promptTextFn = tui.PromptText
var promptSelectFn = tui.PromptSelect

// ErrInterrupted is returned (wrapped) when a run: step's command is cut short
// by ctx cancellation (first CTRL+C), so callers can distinguish a graceful
// shutdown from an ordinary command failure.
var ErrInterrupted = errors.New("interrupted")

// wrapPromptErr classifies a promptTextFn/promptSelectFn error: one caused by
// ctx cancellation (the prompt was torn down mid-input, e.g. a SIGTERM
// arriving while a get: step is blocked waiting on the user) becomes
// ErrInterrupted so callers treat it like any other graceful-shutdown exit,
// rather than an ordinary "aborted" prompt failure.
func wrapPromptErr(varName string, err error) error {
	if tui.IsInterrupted(err) {
		return fmt.Errorf("%w: %v", ErrInterrupted, err)
	}
	return fmt.Errorf("get %s: %w", varName, err)
}

// runningStepMu guards runningStepPID. execRun clears the PID and
// KillRunningStep reads-and-signals it under the same lock so a 2nd CTRL+C
// racing the exact instant a step finishes can't act on a stale PID that the
// OS has since reused for an unrelated process (see setRunningPID /
// clearRunningPID / KillRunningStep in the platform-specific files).
var runningStepMu sync.Mutex
var runningStepPID int

func setRunningPID(pid int) {
	runningStepMu.Lock()
	runningStepPID = pid
	runningStepMu.Unlock()
}

func clearRunningPID() {
	runningStepMu.Lock()
	runningStepPID = 0
	runningStepMu.Unlock()
}

// resolveDirPath returns dir as-is if absolute, else joins it with taskfileDir.
func resolveDirPath(dir, taskfileDir string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(taskfileDir, dir)
}

// displayDirPath returns dir relative to invocationDir when dir is invocationDir
// itself or one of its subdirectories, else returns dir unchanged (full path).
func displayDirPath(dir, invocationDir string) string {
	rel, err := filepath.Rel(invocationDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dir
	}
	if rel == "." {
		return "./"
	}
	return "./" + rel
}

// maskSecrets replaces each secret variable's value in s with "****".
func maskSecrets(s string, scope *cli.Scope) string {
	for name := range scope.Secrets {
		secretVal := scope.Vars[name]
		if secretVal != "" {
			s = strings.ReplaceAll(s, secretVal, "****")
		}
	}
	return s
}

// execCtx bundles the state threaded through every step-execution function.
// scope is kept separate — a call step swaps in a childScope while
// cfg/task/noPrompts/dir change together as a unit.
type execCtx struct {
	ctx       context.Context
	cfg       *config.ConfigFile
	task      string
	noPrompts bool
	dir       string
}

func resolveTask(taskName string, cfg *config.ConfigFile) (config.Task, *config.ConfigFile, error) {
	task, ok := cfg.Tasks[taskName]
	if !ok {
		return config.Task{}, nil, fmt.Errorf("task %q not found", taskName)
	}
	execCfg := cfg
	if task.Cfg != nil {
		execCfg = task.Cfg
	}
	return task, execCfg, nil
}

// ExecuteTask runs taskName using parentDir as the inherited working directory.
// If the task defines a top-level dir:, that overrides parentDir (Priority B).
// For CLI invocations pass invocationDir; execCall passes the resolved child dir.
func ExecuteTask(ctx context.Context, taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, parentDir string) error {
	task, execCfg, err := resolveTask(taskName, cfg)
	if err != nil {
		return err
	}
	if task.Interactive != nil && !*task.Interactive {
		noPrompts = true
	}
	currentDir := parentDir
	if task.Dir != "" {
		resolved, err := eval.EvalTemplate(task.Dir, scope.Vars)
		if err != nil {
			return fmt.Errorf("task %q dir: %w", taskName, err)
		}
		currentDir = resolveDirPath(resolved, execCfg.TaskfileDir)
	}
	if task.IfExpr != "" {
		ok, err := eval.EvalCondition(task.IfExpr, scope.Vars, currentDir)
		if err != nil {
			return fmt.Errorf("task %q if: %w", taskName, err)
		}
		if !ok {
			fmt.Println(tui.SkipLine(taskName))
			return nil
		}
	}
	return executeSteps(execCtx{ctx: ctx, cfg: execCfg, task: taskName, noPrompts: noPrompts, dir: currentDir}, task.Steps, scope)
}

func executeSteps(ec execCtx, steps []config.Step, scope *cli.Scope) error {
	for _, s := range steps {
		if ec.ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, ec.ctx.Err())
		}
		if s.IfExpr != "" {
			ok, err := eval.EvalCondition(s.IfExpr, scope.Vars, ec.dir)
			if err != nil {
				return fmt.Errorf("if condition: %w", err)
			}
			if !ok {
				if s.Kind == config.KindRun {
					fmt.Println(tui.RunSkipLine(ec.task))
				}
				continue
			}
		}

		var err error
		switch s.Kind {
		case config.KindRun:
			err = execRun(ec, s, scope)
		case config.KindSet:
			err = execSet(s, scope)
		case config.KindCall:
			err = execCall(ec, s, scope)
			if err != nil && s.Soft && !errors.Is(err, ErrInterrupted) {
				err = nil
			}
		case config.KindFor:
			err = execFor(ec, s, scope)
		case config.KindGet:
			err = execGet(ec, s, scope)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func execRun(ec execCtx, s config.Step, scope *cli.Scope) error {
	cmd, err := eval.EvalTemplate(s.Command, scope.Vars)
	if err != nil {
		return fmt.Errorf("run template: %w", err)
	}
	runDir := ec.dir
	displayDir := ""
	if s.DirTmpl != "" {
		resolved, err := eval.EvalTemplate(s.DirTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("run dir template: %w", err)
		}
		runDir = resolveDirPath(resolved, ec.cfg.TaskfileDir)
		displayDir = displayDirPath(runDir, scope.Vars["HOBNOB_INVOCATION_DIR"])
	}

	displayCmd := maskSecrets(cmd, scope)
	for _, displayLine := range tui.RunDisplayLines(displayCmd, ec.task, displayDir) {
		fmt.Println(displayLine)
	}
	prefix := tui.TaskPrefix(ec.task)
	stdoutLW := tui.NewLineWriter(os.Stdout, prefix)
	stderrLW := tui.NewLineWriter(os.Stderr, prefix)
	shellCmd := osExec.CommandContext(ec.ctx, "sh", "-c", cmd)
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
	if len(s.IntoEntries) > 0 {
		shellCmd.Stdout = io.MultiWriter(stdoutLW, &stdoutBuf)
		shellCmd.Stderr = io.MultiWriter(stderrLW, &stderrBuf)
	} else {
		shellCmd.Stdout = stdoutLW
		shellCmd.Stderr = stderrLW
	}
	shellCmd.Env = envWithScopeOverrides(scope.Vars)

	err = shellCmd.Start()
	if err == nil {
		setRunningPID(shellCmd.Process.Pid)
		err = shellCmd.Wait()
		clearRunningPID()
	}
	stdoutLW.Flush()
	stderrLW.Flush()
	if err != nil {
		if ec.ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, err)
		}
		return err
	}

	return captureRunInto(s.IntoEntries, scope, stdoutBuf.String(), stderrBuf.String())
}

// envWithScopeOverrides strips any os.Environ() var also present in scope
// before appending scope's vars — os/exec uses first-occurrence-wins for Env,
// and scope vars must win over inherited env.
func envWithScopeOverrides(vars map[string]string) []string {
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
		filtered = append(filtered, varName+"="+varValue)
	}
	return filtered
}

func captureRunInto(entries []config.IntoEntry, scope *cli.Scope, stdout, stderr string) error {
	for _, e := range entries {
		val, err := eval.EvalRunIntoPipe(e.ValueTmpl, stdout, stderr)
		if err != nil {
			return fmt.Errorf("run into %q: %w", e.ParentKey, err)
		}
		scope.Vars[e.ParentKey] = val
	}
	return nil
}

func execSet(s config.Step, scope *cli.Scope) error {
	for _, e := range s.SetEntries {
		val, err := eval.EvalTemplate(e.ValTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("set value for %q: %w", e.Key, err)
		}
		scope.Vars[e.Key] = val
		if e.Secret {
			scope.Secrets[e.Key] = true
		}
	}
	return nil
}

func execGet(ec execCtx, s config.Step, scope *cli.Scope) error {
	for _, e := range s.GetEntries {
		if err := execGetEntry(ec, e, scope); err != nil {
			return err
		}
	}
	return nil
}

func execGetEntry(ec execCtx, e config.GetEntry, scope *cli.Scope) error {
	if _, exists := scope.Vars[e.VarName]; exists {
		if e.Secret {
			scope.Secrets[e.VarName] = true
		}
		if e.Optional && scope.Vars[e.VarName] == "" {
			return nil
		}
		return validateGetValue(e, scope.Vars, ec.noPrompts)
	}
	if ec.noPrompts {
		return getFromNoPrompts(e, scope)
	}
	return getInteractive(ec, e, scope)
}

func getFromNoPrompts(e config.GetEntry, scope *cli.Scope) error {
	if e.Optional {
		if e.Multi {
			scope.Vars[e.VarName] = "[]"
		} else {
			scope.Vars[e.VarName] = ""
		}
		if e.Secret {
			scope.Secrets[e.VarName] = true
		}
		return nil
	}
	if e.DefaultTmpl == "" {
		return fmt.Errorf("--no-input: %s requires input; pass %s=VALUE on the command line (run 'hobnob --help' for details)", e.VarName, e.VarName)
	}
	val, err := eval.EvalTemplate(e.DefaultTmpl, scope.Vars)
	if err != nil {
		return fmt.Errorf("get %s default: %w", e.VarName, err)
	}
	scope.Vars[e.VarName] = val
	if e.Secret {
		scope.Secrets[e.VarName] = true
	}
	return validateGetValue(e, scope.Vars, true)
}

func getInteractive(ec execCtx, e config.GetEntry, scope *cli.Scope) error {
	info, err := eval.EvalTemplate(e.Info, scope.Vars)
	if err != nil {
		return fmt.Errorf("get %s info: %w", e.VarName, err)
	}

	defaultVal := ""
	if e.DefaultTmpl != "" {
		defaultVal, err = eval.EvalTemplate(e.DefaultTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("get %s default: %w", e.VarName, err)
		}
	}

	fromItems, err := eval.ResolveFromItems(e.FromList, e.FromTmpl, scope.Vars, "get "+e.VarName)
	if err != nil {
		return err
	}

	var val string
	if len(fromItems) > 0 {
		for {
			val, err = promptSelectFn(ec.ctx, e.VarName, info, fromItems, e.Multi, defaultVal, ec.task, e.Secret)
			if err != nil {
				return wrapPromptErr(e.VarName, err)
			}
			if e.Check == "" || (e.Optional && val == "") {
				break
			}
			tmp := eval.CopyVars(scope.Vars)
			tmp[e.VarName] = val
			ok, checkErr := eval.EvalCondition(e.Check, tmp, "")
			if checkErr != nil {
				return fmt.Errorf("get %s check: %w", e.VarName, checkErr)
			}
			if ok {
				break
			}
		}
	} else {
		val, err = promptTextFn(ec.ctx, info, e.Check, e.VarName, scope.Vars, defaultVal, ec.task, e.Secret, e.Optional)
		if err != nil {
			return wrapPromptErr(e.VarName, err)
		}
	}

	scope.Vars[e.VarName] = val
	if e.Secret {
		scope.Secrets[e.VarName] = true
	}
	return nil
}

// validateGetValue validates check and (in noPrompts mode) options for a var already in scope.
func validateGetValue(e config.GetEntry, vars map[string]string, noPrompts bool) error {
	if noPrompts && (len(e.FromList) > 0 || e.FromTmpl != "") {
		items, err := eval.ResolveFromItems(e.FromList, e.FromTmpl, vars, "get "+e.VarName)
		if err != nil {
			return err
		}
		optSet := make(map[string]bool, len(items))
		for _, opt := range items {
			optSet[opt] = true
		}
		if e.Multi {
			var selected []string
			if err := json.Unmarshal([]byte(vars[e.VarName]), &selected); err != nil {
				return fmt.Errorf("--no-input: %s value is not a valid JSON array: %w", e.VarName, err)
			}
			for _, sel := range selected {
				if !optSet[sel] {
					return fmt.Errorf("--no-input: %s value %q not in options", e.VarName, sel)
				}
			}
		} else {
			val := vars[e.VarName]
			if !optSet[val] {
				return fmt.Errorf("--no-input: %s value %q not in options", e.VarName, val)
			}
		}
	}
	if e.Check != "" && !(e.Optional && vars[e.VarName] == "") {
		ok, err := eval.EvalCondition(e.Check, vars, "")
		if err != nil {
			return fmt.Errorf("get %s check: %w", e.VarName, err)
		}
		if !ok {
			return fmt.Errorf("get %s: validation failed: %s", e.VarName, e.Check)
		}
	}
	return nil
}

func execCall(ec execCtx, s config.Step, scope *cli.Scope) error {
	noPrompts := ec.noPrompts
	if s.Interactive != nil && !*s.Interactive {
		noPrompts = true
	}

	taskName, err := eval.EvalTemplate(s.CallTarget, scope.Vars)
	if err != nil {
		return fmt.Errorf("call target template: %w", err)
	}

	childScope := scope.Copy()
	for _, e := range s.CallVars {
		val, err := eval.EvalTemplate(e.ValTmpl, childScope.Vars)
		if err != nil {
			return fmt.Errorf("call var %q: %w", e.Key, err)
		}
		childScope.Vars[e.Key] = val
	}

	var callErr error
	if s.DirTmpl != "" {
		// Priority A: call step dir overrides task-level dir
		task, execCfg, err := resolveTask(taskName, ec.cfg)
		if err != nil {
			return err
		}
		resolved, err := eval.EvalTemplate(s.DirTmpl, childScope.Vars)
		if err != nil {
			return fmt.Errorf("call dir template: %w", err)
		}
		childDir := resolveDirPath(resolved, ec.cfg.TaskfileDir)
		callErr = executeSteps(execCtx{ctx: ec.ctx, cfg: execCfg, task: taskName, noPrompts: noPrompts, dir: childDir}, task.Steps, childScope)
	} else {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside ExecuteTask
		callErr = ExecuteTask(ec.ctx, taskName, childScope, ec.cfg, noPrompts, ec.dir)
	}
	if callErr != nil {
		return fmt.Errorf("call %s: %w", taskName, callErr)
	}

	for _, e := range s.IntoEntries {
		parentKey := e.ParentKey

		var val string
		var err error
		if strings.Contains(e.ValueTmpl, "{{") {
			val, err = eval.EvalTemplate(e.ValueTmpl, scope.Vars)
			if err != nil {
				return fmt.Errorf("into value %q: %w", e.ValueTmpl, err)
			}
		} else {
			key := strings.TrimPrefix(e.ValueTmpl, ".")
			val = childScope.Vars[key]
			if childScope.Secrets[key] {
				scope.Secrets[parentKey] = true
			}
		}

		scope.Vars[parentKey] = val
	}

	return nil
}

func scopeSaveRestore(vars map[string]string, name string) func() {
	prev, had := vars[name]
	return func() {
		if had {
			vars[name] = prev
		} else {
			delete(vars, name)
		}
	}
}

func execFor(ec execCtx, s config.Step, scope *cli.Scope) error {
	if len(s.ForMatrix) > 0 {
		return execForMatrix(ec, s.ForMatrix, s.ForSteps, scope)
	}

	// Only the bare-var-reference form (loop: .MY_VAR) can resolve to a JSON
	// object — a literal YAML sequence in loop: can never be a map.
	if len(s.ForList) == 0 && s.ForTarget != "" {
		rendered, err := eval.EvalTemplate(s.ForTarget, scope.Vars)
		if err != nil {
			return fmt.Errorf("loop from template: %w", err)
		}
		if eval.IsJSONObject(rendered) {
			return execForMap(ec, rendered, s.ForSteps, scope)
		}
		items, err := eval.ParseList(rendered)
		if err != nil {
			return fmt.Errorf("loop from list: %w", err)
		}
		return execForList(ec, items, s.ForSteps, scope)
	}

	items, err := eval.ResolveFromItems(s.ForList, s.ForTarget, scope.Vars, "loop")
	if err != nil {
		return err
	}
	// items == nil means the source var/list resolved to empty — zero iterations
	// is the correct semantic result (e.g. loop: .FILES where FILES is empty).
	return execForList(ec, items, s.ForSteps, scope)
}

func execForList(ec execCtx, items []string, steps []config.Step, scope *cli.Scope) error {
	restoreItem := scopeSaveRestore(scope.Vars, "ITEM")
	for _, item := range items {
		scope.Vars["ITEM"] = item
		if err := executeSteps(ec, steps, scope); err != nil {
			restoreItem()
			return err
		}
	}
	restoreItem()
	return nil
}

func execForMap(ec execCtx, rendered string, steps []config.Step, scope *cli.Scope) error {
	keys, values, err := eval.ParseMapEntries(rendered)
	if err != nil {
		return fmt.Errorf("loop: %w", err)
	}

	restoreKey := scopeSaveRestore(scope.Vars, "KEY")
	restoreValue := scopeSaveRestore(scope.Vars, "VALUE")
	for i, key := range keys {
		scope.Vars["KEY"] = key
		scope.Vars["VALUE"] = values[i]
		if err := executeSteps(ec, steps, scope); err != nil {
			restoreValue()
			restoreKey()
			return err
		}
	}
	restoreValue()
	restoreKey()
	return nil
}

func execForMatrix(ec execCtx, matrix []config.ForMatrixEntry, steps []config.Step, scope *cli.Scope) error {
	varNames := make([]string, len(matrix))
	itemLists := make([][]string, len(matrix))
	for i, entry := range matrix {
		items, err := eval.ResolveFromItems(entry.List, entry.ListTmpl, scope.Vars, "loop")
		if err != nil {
			return err
		}
		varNames[i] = entry.VarName
		itemLists[i] = items
	}
	return execCartesian(ec, varNames, itemLists, 0, steps, scope)
}

func execCartesian(ec execCtx, varNames []string, itemLists [][]string, idx int, steps []config.Step, scope *cli.Scope) error {
	if idx == len(varNames) {
		return executeSteps(ec, steps, scope)
	}
	name := varNames[idx]
	restore := scopeSaveRestore(scope.Vars, name)
	for _, item := range itemLists[idx] {
		scope.Vars[name] = item
		if err := execCartesian(ec, varNames, itemLists, idx+1, steps, scope); err != nil {
			restore()
			return err
		}
	}
	restore()
	return nil
}
