package runner

import (
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
)

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

func execFor(execState execCtx, step config.Step, scope *cli.Scope) error {
	if len(step.ForMatrix) > 0 {
		return execForMatrix(execState, step.ForMatrix, step.ForSteps, scope)
	}

	// Only the bare-var-reference form (loop: .MY_VAR) can resolve to a JSON
	// object — a literal YAML sequence in loop: can never be a map.
	if len(step.ForList) == 0 && step.ForTarget != "" {
		rendered, err := eval.EvalTemplate(step.ForTarget, scope.Vars)
		if err != nil {
			return fmt.Errorf("loop from template: %w", err)
		}
		if eval.IsJSONObject(rendered) {
			return execForMap(execState, rendered, step.ForSteps, scope)
		}
		items, err := eval.ParseList(rendered)
		if err != nil {
			return fmt.Errorf("loop from list: %w", err)
		}
		return execForList(execState, items, step.ForSteps, scope)
	}

	items, err := eval.ResolveFromItems(step.ForList, step.ForTarget, scope.Vars, "loop")
	if err != nil {
		return err
	}
	// items == nil means the source var/list resolved to empty — zero iterations
	// is the correct semantic result (e.g. loop: .FILES where FILES is empty).
	return execForList(execState, items, step.ForSteps, scope)
}

func execForList(execState execCtx, items []string, steps []config.Step, scope *cli.Scope) error {
	defer scopeSaveRestore(scope.Vars, "ITEM")()
	for _, item := range items {
		scope.Vars["ITEM"] = item
		if err := executeSteps(execState, steps, scope); err != nil {
			return err
		}
	}
	return nil
}

func execForMap(execState execCtx, rendered string, steps []config.Step, scope *cli.Scope) error {
	keys, values, err := eval.ParseMapEntries(rendered)
	if err != nil {
		return fmt.Errorf("loop: %w", err)
	}

	defer scopeSaveRestore(scope.Vars, "KEY")()
	defer scopeSaveRestore(scope.Vars, "VALUE")()
	for i, key := range keys {
		scope.Vars["KEY"] = key
		scope.Vars["VALUE"] = values[i]
		if err := executeSteps(execState, steps, scope); err != nil {
			return err
		}
	}
	return nil
}

func execForMatrix(execState execCtx, matrix []config.ForMatrixEntry, steps []config.Step, scope *cli.Scope) error {
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
	return execCartesian(execState, varNames, itemLists, 0, steps, scope)
}

func execCartesian(execState execCtx, varNames []string, itemLists [][]string, idx int, steps []config.Step, scope *cli.Scope) error {
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
