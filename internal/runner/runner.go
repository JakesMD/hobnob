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
	return executeSteps(ctx, task.Steps, scope, execCfg, taskName, noPrompts, currentDir)
}

func executeSteps(ctx context.Context, steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	for _, s := range steps {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, ctx.Err())
		}
		if s.IfExpr != "" {
			ok, err := eval.EvalCondition(s.IfExpr, scope.Vars, currentDir)
			if err != nil {
				return fmt.Errorf("if condition: %w", err)
			}
			if !ok {
				if s.Kind == config.KindRun {
					fmt.Println(tui.RunSkipLine(task))
				}
				continue
			}
		}

		var err error
		switch s.Kind {
		case config.KindRun:
			err = execRun(ctx, s, scope, task, cfg.TaskfileDir, currentDir)
		case config.KindSet:
			err = execSet(s, scope)
		case config.KindCall:
			err = execCall(ctx, s, scope, currentDir, cfg, noPrompts)
			if err != nil && s.Soft && !errors.Is(err, ErrInterrupted) {
				err = nil
			}
		case config.KindFor:
			err = execFor(ctx, s, scope, cfg, task, noPrompts, currentDir)
		case config.KindGet:
			err = execGet(ctx, s, scope, task, noPrompts)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func execRun(ctx context.Context, s config.Step, scope *cli.Scope, task, taskfileDir, currentDir string) error {
	cmd, err := eval.EvalTemplate(s.Command, scope.Vars)
	if err != nil {
		return fmt.Errorf("run template: %w", err)
	}
	runDir := currentDir
	displayDir := ""
	if s.DirTmpl != "" {
		resolved, err := eval.EvalTemplate(s.DirTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("run dir template: %w", err)
		}
		runDir = resolveDirPath(resolved, taskfileDir)
		displayDir = displayDirPath(runDir, scope.Vars["HOBNOB_INVOCATION_DIR"])
	}

	displayCmd := maskSecrets(cmd, scope)
	for _, displayLine := range tui.RunDisplayLines(displayCmd, task, displayDir) {
		fmt.Println(displayLine)
	}
	prefix := tui.TaskPrefix(task)
	stdoutLW := tui.NewLineWriter(os.Stdout, prefix)
	stderrLW := tui.NewLineWriter(os.Stderr, prefix)
	shellCmd := osExec.CommandContext(ctx, "sh", "-c", cmd)
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
	// Scope vars must win over inherited env. Build a map of scope keys so we
	// can filter os.Environ() before appending scope vars (first-occurrence wins
	// in os/exec).
	scopeKeys := make(map[string]bool, len(scope.Vars))
	for varName := range scope.Vars {
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
	shellCmd.Env = filtered
	for varName, varValue := range scope.Vars {
		shellCmd.Env = append(shellCmd.Env, varName+"="+varValue)
	}
	err = shellCmd.Start()
	if err == nil {
		setRunningPID(shellCmd.Process.Pid)
		err = shellCmd.Wait()
		clearRunningPID()
	}
	stdoutLW.Flush()
	stderrLW.Flush()
	if err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, err)
		}
		return err
	}

	for _, e := range s.IntoEntries {
		val, err := eval.EvalRunIntoPipe(e.ValueTmpl, stdoutBuf.String(), stderrBuf.String())
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

func execGet(ctx context.Context, s config.Step, scope *cli.Scope, task string, noPrompts bool) error {
	for _, e := range s.GetEntries {
		if err := execGetEntry(ctx, e, scope, task, noPrompts); err != nil {
			return err
		}
	}
	return nil
}

func execGetEntry(ctx context.Context, e config.GetEntry, scope *cli.Scope, task string, noPrompts bool) error {
	if _, exists := scope.Vars[e.VarName]; exists {
		if e.Secret {
			scope.Secrets[e.VarName] = true
		}
		if e.Optional && scope.Vars[e.VarName] == "" {
			return nil
		}
		return validateGetValue(e, scope.Vars, noPrompts)
	}

	if noPrompts {
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
			val, err = promptSelectFn(ctx, e.VarName, info, fromItems, e.Multi, defaultVal, task, e.Secret)
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
		val, err = promptTextFn(ctx, info, e.Check, e.VarName, scope.Vars, defaultVal, task, e.Secret, e.Optional)
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
		if e.Multi {
			var selected []string
			if err := json.Unmarshal([]byte(vars[e.VarName]), &selected); err != nil {
				return fmt.Errorf("--no-input: %s value is not a valid JSON array: %w", e.VarName, err)
			}
			optSet := make(map[string]bool, len(items))
			for _, opt := range items {
				optSet[opt] = true
			}
			for _, sel := range selected {
				if !optSet[sel] {
					return fmt.Errorf("--no-input: %s value %q not in options", e.VarName, sel)
				}
			}
		} else {
			val := vars[e.VarName]
			optSet := make(map[string]bool, len(items))
			for _, opt := range items {
				optSet[opt] = true
			}
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

func execCall(ctx context.Context, s config.Step, scope *cli.Scope, parentDir string, cfg *config.ConfigFile, noPrompts bool) error {
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
		task, execCfg, err := resolveTask(taskName, cfg)
		if err != nil {
			return err
		}
		resolved, err := eval.EvalTemplate(s.DirTmpl, childScope.Vars)
		if err != nil {
			return fmt.Errorf("call dir template: %w", err)
		}
		childDir := resolveDirPath(resolved, cfg.TaskfileDir)
		callErr = executeSteps(ctx, task.Steps, childScope, execCfg, taskName, noPrompts, childDir)
	} else {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside ExecuteTask
		callErr = ExecuteTask(ctx, taskName, childScope, cfg, noPrompts, parentDir)
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

func execFor(ctx context.Context, s config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	if len(s.ForMatrix) > 0 {
		return execForMatrix(ctx, s.ForMatrix, s.ForSteps, scope, cfg, task, noPrompts, currentDir)
	}

	items, err := eval.ResolveFromItems(s.ForList, s.ForTarget, scope.Vars, "loop")
	if err != nil {
		return err
	}
	// items == nil means the source var/list resolved to empty — zero iterations
	// is the correct semantic result (e.g. loop: .FILES where FILES is empty).

	restoreItem := scopeSaveRestore(scope.Vars, "ITEM")
	for _, item := range items {
		scope.Vars["ITEM"] = item
		if err := executeSteps(ctx, s.ForSteps, scope, cfg, task, noPrompts, currentDir); err != nil {
			restoreItem()
			return err
		}
	}
	restoreItem()
	return nil
}

func execForMatrix(ctx context.Context, matrix []config.ForMatrixEntry, steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
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
	return execCartesian(ctx, varNames, itemLists, 0, steps, scope, cfg, task, noPrompts, currentDir)
}

func execCartesian(ctx context.Context, varNames []string, itemLists [][]string, idx int, steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	if idx == len(varNames) {
		return executeSteps(ctx, steps, scope, cfg, task, noPrompts, currentDir)
	}
	name := varNames[idx]
	restore := scopeSaveRestore(scope.Vars, name)
	for _, item := range itemLists[idx] {
		scope.Vars[name] = item
		if err := execCartesian(ctx, varNames, itemLists, idx+1, steps, scope, cfg, task, noPrompts, currentDir); err != nil {
			restore()
			return err
		}
	}
	restore()
	return nil
}
