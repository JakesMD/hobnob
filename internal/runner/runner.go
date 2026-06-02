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

// maskSecrets replaces each secret variable's value in s with "****".
func maskSecrets(s string, vars map[string]string, secrets map[string]bool) string {
	for name := range secrets {
		v := vars[name]
		if v != "" {
			s = strings.ReplaceAll(s, v, "****")
		}
	}
	return s
}

func copySecrets(src map[string]bool) map[string]bool {
	dst := make(map[string]bool, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// ExecuteTask runs taskName using parentDir as the inherited working directory.
// If the task defines a top-level dir:, that overrides parentDir (Priority B).
// For CLI invocations pass invocationDir; execCall passes the resolved child dir.
func ExecuteTask(taskName string, vars map[string]string, cfg *config.ConfigFile, noPrompts bool, parentDir string, secrets map[string]bool) error {
	t, ok := cfg.Tasks[taskName]
	if !ok {
		return fmt.Errorf("task %q not found", taskName)
	}
	execCfg := cfg
	if t.Cfg != nil {
		execCfg = t.Cfg
	}
	currentDir := parentDir
	if t.Dir != "" {
		resolved, err := eval.EvalTemplate(t.Dir, vars)
		if err != nil {
			return fmt.Errorf("task %q dir: %w", taskName, err)
		}
		currentDir = resolveDirPath(resolved, execCfg.TaskfileDir)
	}
	return executeSteps(t.Steps, vars, execCfg, taskName, noPrompts, currentDir, secrets)
}

// executeTaskAtDir runs taskName at a pre-resolved dir, skipping any task-level dir:
// (used by execCall Priority A where the call step's dir overrides the task's own dir).
func executeTaskAtDir(taskName string, vars map[string]string, cfg *config.ConfigFile, noPrompts bool, dir string, secrets map[string]bool) error {
	t, ok := cfg.Tasks[taskName]
	if !ok {
		return fmt.Errorf("task %q not found", taskName)
	}
	execCfg := cfg
	if t.Cfg != nil {
		execCfg = t.Cfg
	}
	return executeSteps(t.Steps, vars, execCfg, taskName, noPrompts, dir, secrets)
}

func executeSteps(steps []config.Step, vars map[string]string, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string, secrets map[string]bool) error {
	for _, s := range steps {
		if s.IfExpr != "" {
			ok, err := eval.EvalCondition(s.IfExpr, vars)
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
			err = execRun(s, vars, task, cfg.TaskfileDir, currentDir, secrets)
		case config.KindSet:
			err = execSet(s, vars, secrets)
		case config.KindCall:
			err = execCall(s, vars, currentDir, cfg, noPrompts, secrets)
			if err != nil && s.Soft {
				err = nil
			}
		case config.KindFor:
			err = execFor(s, vars, cfg, task, noPrompts, currentDir, secrets)
		case config.KindGet:
			err = execGet(s, vars, task, noPrompts, secrets)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func execRun(s config.Step, vars map[string]string, task, taskfileDir, currentDir string, secrets map[string]bool) error {
	cmd, err := eval.EvalTemplate(s.Command, vars)
	if err != nil {
		return fmt.Errorf("run template: %w", err)
	}
	displayCmd := maskSecrets(cmd, vars, secrets)
	for _, displayLine := range cli.RunDisplayLines(displayCmd, task) {
		fmt.Println(displayLine)
	}
	prefix := tui.TaskPrefix(task)
	stdoutLW := cli.NewLineWriter(os.Stdout, prefix)
	stderrLW := cli.NewLineWriter(os.Stderr, prefix)
	c := osExec.Command("sh", "-c", cmd)

	runDir := currentDir
	if s.DirTmpl != "" {
		resolved, err := eval.EvalTemplate(s.DirTmpl, vars)
		if err != nil {
			return fmt.Errorf("run dir template: %w", err)
		}
		runDir = resolveDirPath(resolved, taskfileDir)
	}
	c.Dir = runDir

	var stdoutBuf, stderrBuf bytes.Buffer
	if len(s.IntoEntries) > 0 {
		c.Stdout = io.MultiWriter(stdoutLW, &stdoutBuf)
		c.Stderr = io.MultiWriter(stderrLW, &stderrBuf)
	} else {
		c.Stdout = stdoutLW
		c.Stderr = stderrLW
	}
	c.Env = os.Environ()
	for k, v := range vars {
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
		vars[e.ParentKey] = val
	}
	return nil
}

func execSet(s config.Step, vars map[string]string, secrets map[string]bool) error {
	for _, e := range s.SetEntries {
		val, err := eval.EvalTemplate(e.ValTmpl, vars)
		if err != nil {
			return fmt.Errorf("set value for %q: %w", e.Key, err)
		}
		vars[e.Key] = val
		if e.Secret {
			secrets[e.Key] = true
		}
	}
	return nil
}

func execGet(s config.Step, vars map[string]string, task string, noPrompts bool, secrets map[string]bool) error {
	for _, e := range s.GetEntries {
		if _, exists := vars[e.VarName]; exists {
			if e.Secret {
				secrets[e.VarName] = true
			}
			if e.Optional && vars[e.VarName] == "" {
				continue
			}
			if err := validateGetValue(e, vars, noPrompts); err != nil {
				return err
			}
			continue
		}

		if noPrompts {
			if e.Optional {
				vars[e.VarName] = ""
				if e.Secret {
					secrets[e.VarName] = true
				}
				continue
			}
			if e.DefaultTmpl == "" {
				return fmt.Errorf("--no-input: %s requires input", e.VarName)
			}
			val, err := eval.EvalTemplate(e.DefaultTmpl, vars)
			if err != nil {
				return fmt.Errorf("get %s default: %w", e.VarName, err)
			}
			vars[e.VarName] = val
			if e.Secret {
				secrets[e.VarName] = true
			}
			if err := validateGetValue(e, vars, true); err != nil {
				return err
			}
			continue
		}

		info, err := eval.EvalTemplate(e.Info, vars)
		if err != nil {
			return fmt.Errorf("get %s info: %w", e.VarName, err)
		}

		defaultVal := ""
		if e.DefaultTmpl != "" {
			defaultVal, err = eval.EvalTemplate(e.DefaultTmpl, vars)
			if err != nil {
				return fmt.Errorf("get %s default: %w", e.VarName, err)
			}
		}

		fromItems, err := eval.ResolveFromItems(e.FromList, e.FromTmpl, vars, "get "+e.VarName)
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
				tmp := eval.CopyVars(vars)
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
			val, err = promptTextFn(info, e.Check, e.VarName, vars, defaultVal, task, e.Secret, e.Optional)
			if err != nil {
				return fmt.Errorf("get %s: %w", e.VarName, err)
			}
		}

		vars[e.VarName] = val
		if e.Secret {
			secrets[e.VarName] = true
		}
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
			found := false
			for _, opt := range items {
				if opt == val {
					found = true
					break
				}
			}
			if !found {
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

func execCall(s config.Step, parentVars map[string]string, parentDir string, cfg *config.ConfigFile, noPrompts bool, secrets map[string]bool) error {
	taskName, err := eval.EvalTemplate(s.CallTarget, parentVars)
	if err != nil {
		return fmt.Errorf("call target template: %w", err)
	}

	childVars := eval.CopyVars(parentVars)
	for _, e := range s.CallVars {
		val, err := eval.EvalTemplate(e.ValTmpl, childVars)
		if err != nil {
			return fmt.Errorf("call var %q: %w", e.Key, err)
		}
		childVars[e.Key] = val
	}

	childSecrets := copySecrets(secrets)

	var callErr error
	if s.DirTmpl != "" {
		// Priority A: call step dir overrides task-level dir
		execCfg := cfg
		if t, ok := cfg.Tasks[taskName]; ok && t.Cfg != nil {
			execCfg = t.Cfg
		}
		resolved, err := eval.EvalTemplate(s.DirTmpl, parentVars)
		if err != nil {
			return fmt.Errorf("call dir template: %w", err)
		}
		childDir := resolveDirPath(resolved, execCfg.TaskfileDir)
		callErr = executeTaskAtDir(taskName, childVars, cfg, noPrompts, childDir, childSecrets)
	} else {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside ExecuteTask
		callErr = ExecuteTask(taskName, childVars, cfg, noPrompts, parentDir, childSecrets)
	}
	if callErr != nil {
		return fmt.Errorf("call %s: %w", taskName, callErr)
	}

	for _, e := range s.IntoEntries {
		parentKey := e.ParentKey

		var val string
		var err error
		if strings.Contains(e.ValueTmpl, "{{") {
			val, err = eval.EvalTemplate(e.ValueTmpl, parentVars)
			if err != nil {
				return fmt.Errorf("into value %q: %w", e.ValueTmpl, err)
			}
		} else {
			key := strings.TrimPrefix(e.ValueTmpl, ".")
			val = childVars[key]
			if childSecrets[key] {
				secrets[parentKey] = true
			}
		}

		parentVars[parentKey] = val
	}

	return nil
}

func execFor(s config.Step, vars map[string]string, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string, secrets map[string]bool) error {
	if len(s.ForMatrix) > 0 {
		return execForMatrix(s.ForMatrix, s.ForSteps, vars, cfg, task, noPrompts, currentDir, secrets)
	}

	items, err := eval.ResolveFromItems(s.ForList, s.ForTarget, vars, "loop")
	if err != nil {
		return err
	}

	prevVal, hadPrev := vars["ITEM"]
	for _, item := range items {
		vars["ITEM"] = item
		if err := executeSteps(s.ForSteps, vars, cfg, task, noPrompts, currentDir, secrets); err != nil {
			return err
		}
	}
	if hadPrev {
		vars["ITEM"] = prevVal
	} else {
		delete(vars, "ITEM")
	}

	return nil
}

func execForMatrix(matrix []config.ForMatrixEntry, steps []config.Step, vars map[string]string, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string, secrets map[string]bool) error {
	varNames := make([]string, len(matrix))
	itemLists := make([][]string, len(matrix))
	for i, entry := range matrix {
		items, err := eval.ResolveFromItems(entry.List, entry.ListTmpl, vars, "loop")
		if err != nil {
			return err
		}
		varNames[i] = entry.VarName
		itemLists[i] = items
	}
	return execCartesian(varNames, itemLists, 0, steps, vars, cfg, task, noPrompts, currentDir, secrets)
}

func execCartesian(varNames []string, itemLists [][]string, idx int, steps []config.Step, vars map[string]string, cfg *config.ConfigFile, task string, noPrompts bool, currentDir string, secrets map[string]bool) error {
	if idx == len(varNames) {
		return executeSteps(steps, vars, cfg, task, noPrompts, currentDir, secrets)
	}
	name := varNames[idx]
	prevVal, hadPrev := vars[name]
	for _, item := range itemLists[idx] {
		vars[name] = item
		if err := execCartesian(varNames, itemLists, idx+1, steps, vars, cfg, task, noPrompts, currentDir, secrets); err != nil {
			return err
		}
	}
	if hadPrev {
		vars[name] = prevVal
	} else {
		delete(vars, name)
	}
	return nil
}
