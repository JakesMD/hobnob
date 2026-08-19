package runner

import (
	"context"
	"encoding/json"
	"fmt"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
	"hobnob/internal/value"
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
	if existing, exists := scope.Vars[getEntry.VarName]; exists {
		if getEntry.Secret {
			scope.Secrets[getEntry.VarName] = true
		}
		if getEntry.Optional && existing.IsEmpty() {
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
			scope.Set(getEntry.VarName, value.Of([]any{}), getEntry.Secret)
		} else {
			scope.Set(getEntry.VarName, value.Nil(), getEntry.Secret)
		}
		return nil
	}
	if getEntry.DefaultTmpl == "" {
		return fmt.Errorf("--no-input: %s requires input; pass %s=VALUE on the command line (run 'hobnob --help' for details)", getEntry.VarName, getEntry.VarName)
	}
	val, err := eval.EvalValue(getEntry.DefaultTmpl, scope.Vars)
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

	fromItems, err := eval.ResolveItemStrings(getEntry.FromList, getEntry.FromTmpl, scope.Vars, "get "+getEntry.VarName)
	if err != nil {
		return err
	}

	var val value.Value
	if len(fromItems) > 0 {
		val, err = promptSelectUntilValid(execState, getEntry, fromItems, info, defaultVal, scope.Vars)
		if err != nil {
			return err
		}
	} else {
		checkValidate := checkValidator(execState.ctx, getEntry, scope.Vars)
		text, err := promptTextFn(execState.ctx, info, getEntry.VarName, checkValidate, getEntry.Check, defaultVal, execState.task, getEntry.Secret, getEntry.Optional)
		if err != nil {
			return wrapPromptErr(getEntry.VarName, err)
		}
		val = value.Str(text)
	}

	scope.Set(getEntry.VarName, val, getEntry.Secret)
	return nil
}

// checkValidator returns a candidate-validating closure for tui.PromptText,
// or nil when getEntry has no check: — tui calls it only if non-nil, so it
// never needs to know how a check is evaluated.
func checkValidator(ctx context.Context, getEntry config.GetEntry, vars map[string]value.Value) func(string) (bool, error) {
	if getEntry.Check == "" {
		return nil
	}
	return func(candidate string) (bool, error) {
		return eval.EvalCheckWithOverride(ctx, getEntry.Check, vars, getEntry.VarName, value.Str(candidate))
	}
}

// promptSelectUntilValid re-prompts the select menu until the chosen value
// passes getEntry.Check (or there's no check to pass). Errors come back ready
// to return: a prompt failure through wrapPromptErr, a check failure with its
// own context — wrapping again at the call site would double the "get X:" prefix.
func promptSelectUntilValid(execState execCtx, getEntry config.GetEntry, fromItems []string, info, defaultVal string, vars map[string]value.Value) (value.Value, error) {
	for {
		val, err := promptSelectFn(execState.ctx, getEntry.VarName, info, fromItems, getEntry.Multi, defaultVal, execState.task, getEntry.Secret)
		if err != nil {
			return value.Value{}, wrapPromptErr(getEntry.VarName, err)
		}
		if getEntry.Check == "" || (getEntry.Optional && val.IsEmpty()) {
			return val, nil
		}
		ok, err := eval.EvalCheckWithOverride(execState.ctx, getEntry.Check, vars, getEntry.VarName, val)
		if err != nil {
			return value.Value{}, fmt.Errorf("get %s check: %w", getEntry.VarName, err)
		}
		if ok {
			return val, nil
		}
	}
}

// validateGetValue validates check and (in noPrompts mode) options for a var already in scope.
func validateGetValue(execState execCtx, getEntry config.GetEntry, vars map[string]value.Value) error {
	if execState.noPrompts && (len(getEntry.FromList) > 0 || getEntry.FromTmpl != "") {
		items, err := eval.ResolveItemStrings(getEntry.FromList, getEntry.FromTmpl, vars, "get "+getEntry.VarName)
		if err != nil {
			return err
		}
		optSet := make(map[string]bool, len(items))
		for _, opt := range items {
			optSet[opt] = true
		}
		if getEntry.Multi {
			selected, err := multiSelectedStrings(vars[getEntry.VarName])
			if err != nil {
				return fmt.Errorf("--no-input: %s value is not a valid JSON array: %w", getEntry.VarName, err)
			}
			for _, sel := range selected {
				if !optSet[sel] {
					return fmt.Errorf("--no-input: %s value %q not in options", getEntry.VarName, sel)
				}
			}
		} else {
			val := vars[getEntry.VarName].String()
			if !optSet[val] {
				return fmt.Errorf("--no-input: %s value %q not in options", getEntry.VarName, val)
			}
		}
	}
	if getEntry.Check != "" && !(getEntry.Optional && vars[getEntry.VarName].IsEmpty()) {
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

// multiSelectedStrings reads a multi: get value's selections as strings — an
// Array (the typed shape a real select produces) is read directly; a String
// falls back to strict JSON-array decoding, since that's how a CI caller
// passes multi: values on the command line (KEY=["a","b"]), which are always
// plain strings, never sniffed into structure.
func multiSelectedStrings(v value.Value) ([]string, error) {
	if v.Kind() == value.KindArray {
		arr := v.Any().([]any)
		selected := make([]string, len(arr))
		for i, elem := range arr {
			selected[i] = value.Of(elem).String()
		}
		return selected, nil
	}
	var selected []string
	err := json.Unmarshal([]byte(v.String()), &selected)
	return selected, err
}
