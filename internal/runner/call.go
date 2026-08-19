package runner

import (
	"fmt"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
)

func execCall(execState execCtx, step config.Step, scope *cli.Scope) error {
	noPrompts := execState.noPrompts
	if step.Interactive != nil && !*step.Interactive {
		noPrompts = true
	}

	taskName, err := eval.EvalTemplate(step.CallTarget, scope.Vars)
	if err != nil {
		return fmt.Errorf("call target template: %w", err)
	}

	childScope, err := buildCallScope(scope, step.CallVars)
	if err != nil {
		return err
	}

	if err := runCallSteps(execState, taskName, step.DirTmpl, noPrompts, childScope); err != nil {
		return fmt.Errorf("call %s: %w", taskName, err)
	}

	return captureCallInto(step.IntoEntries, scope, childScope)
}

// buildCallScope deep-copies scope and evaluates with: entries into it. No
// secret flag is set here — config.rejectSecretCallVars rules secret: out of
// with: entirely, because Copy() already brought the parent's secrets across
// and masking matches on value, so a secret passed down stays masked under its
// new name without anything to declare at the call site.
func buildCallScope(scope *cli.Scope, callVars []config.SetEntry) (*cli.Scope, error) {
	childScope := scope.Copy()
	for _, callVar := range callVars {
		var val string
		var err error
		if callVar.ValNode != nil {
			val, err = evalJSONNodeToJSON(*callVar.ValNode, func(tmpl string) (string, error) {
				return eval.EvalTemplate(tmpl, childScope.Vars)
			})
		} else {
			val, err = eval.EvalTemplate(callVar.ValTmpl, childScope.Vars)
		}
		if err != nil {
			return nil, fmt.Errorf("call var %q: %w", callVar.Key, err)
		}
		childScope.Vars[callVar.Key] = val
	}
	return childScope, nil
}

// runCallSteps executes the called task in childScope, resolving its working
// directory per the dir: priority chain documented on ExecuteTask.
func runCallSteps(execState execCtx, taskName, dirTmpl string, noPrompts bool, childScope *cli.Scope) error {
	if dirTmpl == "" {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside ExecuteTask
		return ExecuteTask(execState.ctx, taskName, childScope, execState.cfg, noPrompts, execState.dir)
	}
	// Priority A: call step dir overrides task-level dir
	task, execCfg, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}
	resolved, err := eval.EvalTemplate(dirTmpl, childScope.Vars)
	if err != nil {
		return fmt.Errorf("call dir template: %w", err)
	}
	childDir := resolveDirPath(resolved, execState.cfg.TaskfileDir)
	return executeSteps(execCtx{ctx: execState.ctx, cfg: execCfg, task: taskName, noPrompts: noPrompts, dir: childDir}, task.Steps, childScope)
}

// captureCallInto pulls into: results back from childScope into the caller's
// scope, either by re-evaluating a template against the caller's own vars or
// by reading a bare .KEY reference straight out of childScope.
func captureCallInto(entries []config.IntoEntry, scope, childScope *cli.Scope) error {
	evalLeaf := func(valueTmpl string) (string, error) {
		if strings.Contains(valueTmpl, "{{") {
			return eval.EvalTemplate(valueTmpl, scope.Vars)
		}
		key := strings.TrimPrefix(valueTmpl, ".")
		return childScope.Vars[key], nil
	}
	for _, intoEntry := range entries {
		parentKey := intoEntry.ParentKey

		if intoEntry.ValNode != nil {
			val, err := evalJSONNodeToJSON(*intoEntry.ValNode, evalLeaf)
			if err != nil {
				return fmt.Errorf("into value %q: %w", parentKey, err)
			}
			scope.Vars[parentKey] = val
			continue
		}

		if strings.Contains(intoEntry.ValueTmpl, "{{") {
			val, err := eval.EvalTemplate(intoEntry.ValueTmpl, scope.Vars)
			if err != nil {
				return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
			}
			scope.Vars[parentKey] = val
			continue
		}
		key := strings.TrimPrefix(intoEntry.ValueTmpl, ".")
		scope.Set(parentKey, childScope.Vars[key], childScope.Secrets[key])
	}
	return nil
}
