package runner

import (
	"fmt"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
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
		val, err := config.EvalSetEntry(callVar, func(tmpl string) (value.Value, error) {
			return eval.EvalValue(tmpl, childScope.Vars)
		})
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
// scope. Each leaf is one of:
//   - an explicit {{ }} template, evaluated against the CALLER's own
//     (evolving) scope — so later into: entries in the same block can
//     reference earlier ones, e.g. ARCHIVE_PATH: "{{.LOG_PATH}}/archive"
//     where LOG_PATH was itself just set by a prior entry in this call.
//   - a bare key (with or without a leading "."), optionally followed by
//     " | filter | filter" — read straight out of childScope, typed, then
//     run through the filter chain if there is one.
func captureCallInto(entries []config.IntoEntry, scope, childScope *cli.Scope) error {
	evalLeaf := func(valueTmpl string) (value.Value, error) {
		if strings.Contains(valueTmpl, "{{") {
			return eval.EvalValue(valueTmpl, scope.Vars)
		}
		return resolveChildRef(valueTmpl, childScope.Vars)
	}
	for _, intoEntry := range entries {
		parentKey := intoEntry.ParentKey

		if intoEntry.ValNode != nil {
			val, err := config.EvalJSONNode(*intoEntry.ValNode, evalLeaf)
			if err != nil {
				return fmt.Errorf("into value %q: %w", parentKey, err)
			}
			scope.Vars[parentKey] = val
			continue
		}

		if strings.Contains(intoEntry.ValueTmpl, "{{") {
			val, err := evalLeaf(intoEntry.ValueTmpl)
			if err != nil {
				return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
			}
			scope.Vars[parentKey] = val
			continue
		}

		key, chain, hasChain := strings.Cut(intoEntry.ValueTmpl, " | ")
		key = strings.TrimPrefix(strings.TrimSpace(key), ".")
		if !hasChain {
			scope.Set(parentKey, childScope.Vars[key], childScope.Secrets[key])
			continue
		}
		val, err := eval.EvalValue("."+key+" | "+chain, childScope.Vars)
		if err != nil {
			return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
		}
		scope.Vars[parentKey] = val
	}
	return nil
}

// resolveChildRef evaluates a bare into: leaf (no {{ }}) against childVars: a
// plain key ("KEY" or ".KEY") is a direct typed lookup, a key followed by
// " | filter | ..." runs that chain typed via EvalValue.
func resolveChildRef(valueTmpl string, childVars map[string]value.Value) (value.Value, error) {
	key, chain, hasChain := strings.Cut(valueTmpl, " | ")
	key = strings.TrimPrefix(strings.TrimSpace(key), ".")
	if !hasChain {
		return childVars[key], nil
	}
	return eval.EvalValue("."+key+" | "+chain, childVars)
}
