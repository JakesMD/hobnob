package runner

import (
	"encoding/json"
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
)

// promptTextFn and promptSelectFn are package-level so tests can substitute fakes.
var promptTextFn = tui.PromptText
var promptSelectFn = tui.PromptSelect

func execGet(execState execCtx, step config.Step, scope *cli.Scope) error {
	for _, getEntry := range step.GetEntries {
		if err := execGetEntry(execState, getEntry, scope); err != nil {
			return err
		}
	}
	return nil
}

func execGetEntry(execState execCtx, getEntry config.GetEntry, scope *cli.Scope) error {
	if _, exists := scope.Vars[getEntry.VarName]; exists {
		if getEntry.Secret {
			scope.Secrets[getEntry.VarName] = true
		}
		if getEntry.Optional && scope.Vars[getEntry.VarName] == "" {
			return nil
		}
		return validateGetValue(execState, getEntry, scope.Vars)
	}
	if execState.noPrompts {
		return getFromNoPrompts(execState, getEntry, scope)
	}
	return getInteractive(execState, getEntry, scope)
}

func getFromNoPrompts(execState execCtx, getEntry config.GetEntry, scope *cli.Scope) error {
	if getEntry.Optional {
		if getEntry.Multi {
			scope.Set(getEntry.VarName, "[]", getEntry.Secret)
		} else {
			scope.Set(getEntry.VarName, "", getEntry.Secret)
		}
		return nil
	}
	if getEntry.DefaultTmpl == "" {
		return fmt.Errorf("--no-input: %s requires input; pass %s=VALUE on the command line (run 'hobnob --help' for details)", getEntry.VarName, getEntry.VarName)
	}
	val, err := eval.EvalTemplate(getEntry.DefaultTmpl, scope.Vars)
	if err != nil {
		return fmt.Errorf("get %s default: %w", getEntry.VarName, err)
	}
	scope.Set(getEntry.VarName, val, getEntry.Secret)
	return validateGetValue(execState, getEntry, scope.Vars)
}

func getInteractive(execState execCtx, getEntry config.GetEntry, scope *cli.Scope) error {
	info, err := eval.EvalTemplate(getEntry.Info, scope.Vars)
	if err != nil {
		return fmt.Errorf("get %s info: %w", getEntry.VarName, err)
	}

	defaultVal := ""
	if getEntry.DefaultTmpl != "" {
		defaultVal, err = eval.EvalTemplate(getEntry.DefaultTmpl, scope.Vars)
		if err != nil {
			return fmt.Errorf("get %s default: %w", getEntry.VarName, err)
		}
	}

	fromItems, err := eval.ResolveFromItems(getEntry.FromList, getEntry.FromTmpl, scope.Vars, "get "+getEntry.VarName)
	if err != nil {
		return err
	}

	var val string
	if len(fromItems) > 0 {
		val, err = promptSelectUntilValid(execState, getEntry, fromItems, info, defaultVal, scope.Vars)
		if err != nil {
			return err
		}
	} else {
		val, err = promptTextFn(execState.ctx, info, getEntry.Check, getEntry.VarName, scope.Vars, defaultVal, execState.task, getEntry.Secret, getEntry.Optional)
		if err != nil {
			return wrapPromptErr(getEntry.VarName, err)
		}
	}

	scope.Set(getEntry.VarName, val, getEntry.Secret)
	return nil
}

// promptSelectUntilValid re-prompts the select menu until the chosen value
// passes getEntry.Check (or there's no check to pass). Errors come back ready
// to return: a prompt failure through wrapPromptErr, a check failure with its
// own context — wrapping again at the call site would double the "get X:" prefix.
func promptSelectUntilValid(execState execCtx, getEntry config.GetEntry, fromItems []string, info, defaultVal string, vars map[string]string) (string, error) {
	for {
		val, err := promptSelectFn(execState.ctx, getEntry.VarName, info, fromItems, getEntry.Multi, defaultVal, execState.task, getEntry.Secret)
		if err != nil {
			return "", wrapPromptErr(getEntry.VarName, err)
		}
		if getEntry.Check == "" || (getEntry.Optional && val == "") {
			return val, nil
		}
		ok, err := eval.EvalCheckWithOverride(execState.ctx, getEntry.Check, vars, getEntry.VarName, val)
		if err != nil {
			return "", fmt.Errorf("get %s check: %w", getEntry.VarName, err)
		}
		if ok {
			return val, nil
		}
	}
}

// validateGetValue validates check and (in noPrompts mode) options for a var already in scope.
func validateGetValue(execState execCtx, getEntry config.GetEntry, vars map[string]string) error {
	if execState.noPrompts && (len(getEntry.FromList) > 0 || getEntry.FromTmpl != "") {
		items, err := eval.ResolveFromItems(getEntry.FromList, getEntry.FromTmpl, vars, "get "+getEntry.VarName)
		if err != nil {
			return err
		}
		optSet := make(map[string]bool, len(items))
		for _, opt := range items {
			optSet[opt] = true
		}
		if getEntry.Multi {
			var selected []string
			if err := json.Unmarshal([]byte(vars[getEntry.VarName]), &selected); err != nil {
				return fmt.Errorf("--no-input: %s value is not a valid JSON array: %w", getEntry.VarName, err)
			}
			for _, sel := range selected {
				if !optSet[sel] {
					return fmt.Errorf("--no-input: %s value %q not in options", getEntry.VarName, sel)
				}
			}
		} else {
			val := vars[getEntry.VarName]
			if !optSet[val] {
				return fmt.Errorf("--no-input: %s value %q not in options", getEntry.VarName, val)
			}
		}
	}
	if getEntry.Check != "" && !(getEntry.Optional && vars[getEntry.VarName] == "") {
		ok, err := eval.EvalCondition(execState.ctx, getEntry.Check, vars, "")
		if err != nil {
			return fmt.Errorf("get %s check: %w", getEntry.VarName, err)
		}
		if !ok {
			return fmt.Errorf("get %s: validation failed: %s", getEntry.VarName, getEntry.Check)
		}
	}
	return nil
}
