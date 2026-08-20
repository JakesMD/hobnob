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

	if err := runTaskSteps(execState, taskName, step.DirTmpl, noPrompts, childScope); err != nil {
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

// runTaskSteps executes taskName against runScope (a call:'s isolated
// childScope, or a use:'s shared caller scope), resolving its working
// directory per the dir: priority chain documented on ExecuteTask. Shared by
// execCall and execUse — the only difference between them is which scope
// object this is called with and whether the memo cache is consulted.
func runTaskSteps(execState execCtx, taskName, dirTmpl string, noPrompts bool, runScope *cli.Scope) error {
	if dirTmpl == "" {
		// Priority B (task-level dir) or C (inherit parentDir) — handled inside executeTask
		return executeTask(execCtx{ctx: execState.ctx, cfg: execState.cfg, noPrompts: noPrompts, dir: execState.dir, memo: execState.memo}, taskName, runScope)
	}
	// Priority A: step-level dir overrides task-level dir
	task, execCfg, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}
	resolved, err := eval.EvalTemplate(dirTmpl, runScope.Vars)
	if err != nil {
		return fmt.Errorf("dir template: %w", err)
	}
	childDir := resolveDirPath(resolved, execState.cfg.TaskfileDir)
	return executeSteps(execCtx{ctx: execState.ctx, cfg: execCfg, task: taskName, noPrompts: noPrompts, dir: childDir, memo: execState.memo}, task.Steps, runScope)
}

// captureCallInto pulls into: results back from childScope into the caller's
// scope. Each leaf is one of:
//   - an explicit {{ }} template, evaluated against the CALLER's own
//     (evolving) scope — so later into: entries in the same block can
//     reference earlier ones, e.g. ARCHIVE_PATH: "{{.LOG_PATH}}/archive"
//     where LOG_PATH was itself just set by a prior entry in this call.
//   - a bare key (with or without a leading "."), optionally followed by an
//     accessor ([0].name) and/or " | filter | filter" — read straight out
//     of childScope, typed, then resolved/filtered if there's an accessor
//     or chain.
func captureCallInto(entries []config.IntoEntry, scope, childScope *cli.Scope) error {
	evalLeaf := func(valueTmpl string) (value.Value, error) {
		if strings.Contains(valueTmpl, "{{") {
			return eval.EvalValue(valueTmpl, scope.Vars)
		}
		val, _, err := resolveChildRef(valueTmpl, childScope.Vars)
		return val, err
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

		val, plainKey, err := resolveChildRef(intoEntry.ValueTmpl, childScope.Vars)
		if err != nil {
			return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
		}
		if plainKey != "" {
			scope.Set(parentKey, val, childScope.Secrets[plainKey])
			continue
		}
		scope.Vars[parentKey] = val
	}
	return nil
}

// resolveChildRef evaluates a bare into: leaf (no {{ }}) against childVars:
// "KEY", ".KEY", "KEY.field[0]", "KEY | trim", "KEY[0].name | upper". When
// the leaf is exactly one bare var name with no accessor or filter chain,
// plainKey returns that name so the caller can propagate its secret flag —
// every other shape loses the secret annotation, same as passing a value
// through any filter does.
func resolveChildRef(valueTmpl string, childVars map[string]value.Value) (val value.Value, plainKey string, err error) {
	key, chain, hasChain := strings.Cut(valueTmpl, " | ")
	key = strings.TrimPrefix(strings.TrimSpace(key), ".")
	if !hasChain && isPlainKey(key) {
		return childVars[key], key, nil
	}
	expr := "{{ ." + key
	if hasChain {
		expr += " | " + chain
	}
	val, err = eval.EvalValue(expr+" }}", childVars)
	return val, "", err
}

func isPlainKey(s string) bool {
	if s == "" {
		return false
	}
	return !strings.ContainsAny(s, ".[")
}
