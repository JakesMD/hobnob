package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	osExec "os/exec"
	"path/filepath"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
)

// promptTextFn and promptSelectFn are package-level so tests can substitute fakes.
var promptTextFn = tui.PromptText
var promptSelectFn = tui.PromptSelect

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
		v := scope.Vars[name]
		if v != "" {
			s = strings.ReplaceAll(s, v, "****")
		}
	}
	return s
}

func resolveTask(taskName string, cfg *config.ConfigFile) (config.Task, *config.ConfigFile, error) {
	t, ok := cfg.Tasks[taskName]
	if !ok {
		return config.Task{}, nil, fmt.Errorf("task %q not found", taskName)
	}
	execCfg := cfg
	if t.Cfg != nil {
		execCfg = t.Cfg
	}
	return t, execCfg, nil
}

// ExecuteTask runs taskName using parentDir as the inherited working directory.
// If the task defines a top-level dir:, that overrides parentDir (Priority B).
// For CLI invocations pass invocationDir; execCall passes the resolved child dir.
func ExecuteTask(taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, parentDir string) error {
	t, execCfg, err := resolveTask(taskName, cfg)
	if err != nil {
		return err
	}
	currentDir := parentDir
	if t.Dir != "" {
		resolved, err := eval.EvalTemplate(t.Dir, scope.Vars)
		if err != nil {
			return fmt.Errorf("task %q dir: %w", taskName, err)
		}
		currentDir = resolveDirPath(resolved, execCfg.TaskfileDir)
	}
	return executeSteps(t.Steps, scope, execCfg, taskName, noPrompts, currentDir)
}

func executeSteps(steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	for _, s := range steps {
		if s.IfExpr != "" {
			ok, err := eval.EvalCondition(s.IfExpr, scope.Vars)
			if err != nil {
				return fmt.Errorf("if condition: %w", err)
			}
			if !ok {
				continue
			}
		}

		var err error
		switch s.Kind {
		case config.KindRun:
			err = execRun(s, scope, task, cfg.TaskfileDir, currentDir)
		case config.KindSet:
			err = execSet(s, scope)
		case config.KindCall:
			err = execCall(s, scope, currentDir, cfg, noPrompts)
			if err != nil && s.Soft {
				err = nil
			}
		case config.KindFor:
			err = execFor(s, scope, cfg, task, noPrompts, currentDir)
		case config.KindGet:
			err = execGet(s, scope, task, noPrompts)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func execRun(s config.Step, scope *cli.Scope, task, taskfileDir, currentDir string) error {
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
	c := osExec.Command("sh", "-c", cmd)
	c.Dir = runDir

	var stdoutBuf, stderrBuf bytes.Buffer
	if len(s.IntoEntries) > 0 {
		c.Stdout = io.MultiWriter(stdoutLW, &stdoutBuf)
		c.Stderr = io.MultiWriter(stderrLW, &stderrBuf)
	} else {
		c.Stdout = stdoutLW
		c.Stderr = stderrLW
	}
	// Scope vars must win over inherited env. Build a map of scope keys so we
	// can filter os.Environ() before appending scope vars (first-occurrence wins
	// in os/exec).
	scopeKeys := make(map[string]bool, len(scope.Vars))
	for k := range scope.Vars {
		scopeKeys[k] = true
	}
	env := os.Environ()
	filtered := env[:0:len(env)]
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !scopeKeys[k] {
			filtered = append(filtered, e)
		}
	}
	c.Env = filtered
	for k, v := range scope.Vars {
		c.Env = append(c.Env, k+"="+v)
	}
	err = c.Run()
	stdoutLW.Flush()
	stderrLW.Flush()
	if err != nil {
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

func execGet(s config.Step, scope *cli.Scope, task string, noPrompts bool) error {
	for _, e := range s.GetEntries {
		if err := execGetEntry(e, scope, task, noPrompts); err != nil {
			return err
		}
	}
	return nil
}

func execGetEntry(e config.GetEntry, scope *cli.Scope, task string, noPrompts bool) error {
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
			return fmt.Errorf("--no-input: %s requires input", e.VarName)
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
			val, err = promptSelectFn(e.VarName, info, fromItems, e.Multi, defaultVal, task, e.Secret)
			if err != nil {
				return fmt.Errorf("get %s: %w", e.VarName, err)
			}
			if e.Check == "" || (e.Optional && val == "") {
				break
			}
			tmp := eval.CopyVars(scope.Vars)
			tmp[e.VarName] = val
			ok, checkErr := eval.EvalCondition(e.Check, tmp)
			if checkErr != nil {
				return fmt.Errorf("get %s check: %w", e.VarName, checkErr)
			}
			if ok {
				break
			}
		}
	} else {
		val, err = promptTextFn(info, e.Check, e.VarName, scope.Vars, defaultVal, task, e.Secret, e.Optional)
		if err != nil {
			return fmt.Errorf("get %s: %w", e.VarName, err)
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
		ok, err := eval.EvalCondition(e.Check, vars)
		if err != nil {
			return fmt.Errorf("get %s check: %w", e.VarName, err)
		}
		if !ok {
			return fmt.Errorf("get %s: validation failed: %s", e.VarName, e.Check)
		}
	}
	return nil
}

func execCall(s config.Step, scope *cli.Scope, parentDir string, cfg *config.ConfigFile, noPrompts bool) error {
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
		t, execCfg, err := resolveTask(taskName, cfg)
		if err != nil {
			return err
		}
		resolved, err := eval.EvalTemplate(s.DirTmpl, childScope.Vars)
		if err != nil {
			return fmt.Errorf("call dir template: %w", err)
		}
		childDir := resolveDirPath(resolved, cfg.TaskfileDir)
		callErr = executeSteps(t.Steps, childScope, execCfg, taskName, noPrompts, childDir)
	} else {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside ExecuteTask
		callErr = ExecuteTask(taskName, childScope, cfg, noPrompts, parentDir)
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

func execFor(s config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	if len(s.ForMatrix) > 0 {
		return execForMatrix(s.ForMatrix, s.ForSteps, scope, cfg, task, noPrompts, currentDir)
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
		if err := executeSteps(s.ForSteps, scope, cfg, task, noPrompts, currentDir); err != nil {
			restoreItem()
			return err
		}
	}
	restoreItem()
	return nil
}

func execForMatrix(matrix []config.ForMatrixEntry, steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
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
	return execCartesian(varNames, itemLists, 0, steps, scope, cfg, task, noPrompts, currentDir)
}

func execCartesian(varNames []string, itemLists [][]string, idx int, steps []config.Step, scope *cli.Scope, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string) error {
	if idx == len(varNames) {
		return executeSteps(steps, scope, cfg, task, noPrompts, currentDir)
	}
	name := varNames[idx]
	restore := scopeSaveRestore(scope.Vars, name)
	for _, item := range itemLists[idx] {
		scope.Vars[name] = item
		if err := execCartesian(varNames, itemLists, idx+1, steps, scope, cfg, task, noPrompts, currentDir); err != nil {
			restore()
			return err
		}
	}
	restore()
	return nil
}
