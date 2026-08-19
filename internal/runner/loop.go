package runner

import (
	"fmt"
	"sort"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

func scopeSaveRestore(vars map[string]value.Value, name string) func() {
	prev, had := vars[name]
	return func() {
		if had {
			vars[name] = prev
		} else {
			delete(vars, name)
		}
	}
}

func execFor(execState execCtx, step config.Step, scope *cli.Scope) error {
	if len(step.ForMatrix) > 0 {
		return execForMatrix(execState, step.ForMatrix, step.ForSteps, scope)
	}

	// Only the bare-var-reference form (loop: .MY_VAR) can resolve to an
	// Object — a literal YAML sequence in loop: can never be a map.
	if len(step.ForList) == 0 && step.ForTarget != "" {
		rendered, err := eval.EvalValue(step.ForTarget, scope.Vars)
		if err != nil {
			return fmt.Errorf("loop from template: %w", err)
		}
		if rendered.Kind() == value.KindObject {
			return execForMap(execState, rendered, step.ForSteps, scope)
		}
		return execForList(execState, eval.ItemsFromValue(rendered), step.ForSteps, scope)
	}

	items, err := eval.ResolveItems(step.ForList, step.ForTarget, scope.Vars, "loop")
	if err != nil {
		return err
	}
	// items == nil means the source var/list resolved to empty — zero iterations
	// is the correct semantic result (e.g. loop: .FILES where FILES is empty).
	return execForList(execState, items, step.ForSteps, scope)
}

func execForList(execState execCtx, items []value.Value, steps []config.Step, scope *cli.Scope) error {
	defer scopeSaveRestore(scope.Vars, "ITEM")()
	for _, item := range items {
		scope.Vars["ITEM"] = item
		if err := executeSteps(execState, steps, scope); err != nil {
			return err
		}
	}
	return nil
}

func execForMap(execState execCtx, obj value.Value, steps []config.Step, scope *cli.Scope) error {
	object := obj.Any().(map[string]any)
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	defer scopeSaveRestore(scope.Vars, "KEY")()
	defer scopeSaveRestore(scope.Vars, "VALUE")()
	for _, key := range keys {
		scope.Vars["KEY"] = value.Str(key)
		scope.Vars["VALUE"] = value.Of(object[key])
		if err := executeSteps(execState, steps, scope); err != nil {
			return err
		}
	}
	return nil
}

func execForMatrix(execState execCtx, matrix []config.ForMatrixEntry, steps []config.Step, scope *cli.Scope) error {
	varNames := make([]string, len(matrix))
	itemLists := make([][]value.Value, len(matrix))
	for i, entry := range matrix {
		items, err := eval.ResolveItems(entry.List, entry.ListTmpl, scope.Vars, "loop")
		if err != nil {
			return err
		}
		varNames[i] = entry.VarName
		itemLists[i] = items
	}
	return execCartesian(execState, varNames, itemLists, 0, steps, scope)
}

func execCartesian(execState execCtx, varNames []string, itemLists [][]value.Value, idx int, steps []config.Step, scope *cli.Scope) error {
	if idx == len(varNames) {
		return executeSteps(execState, steps, scope)
	}
	name := varNames[idx]
	defer scopeSaveRestore(scope.Vars, name)()
	for _, item := range itemLists[idx] {
		scope.Vars[name] = item
		if err := execCartesian(execState, varNames, itemLists, idx+1, steps, scope); err != nil {
			return err
		}
	}
	return nil
}
