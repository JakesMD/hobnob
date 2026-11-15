package runner

import (
	"context"
	"fmt"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// promptTextFn and promptSelectFn are package-level so SetPrompts (testseam.go)
// can substitute fakes.
var promptTextFn TextPromptFunc = tui.PromptText
var promptSelectFn SelectPromptFunc = tui.PromptSelect

func execGet(execState execCtx, step config.Step, scope *scope.Scope) error {
	for _, getEntry := range step.GetEntries {
		if err := execGetEntry(execState, getEntry, scope); err != nil {
			return err
		}
	}
	return nil
}

func execGetEntry(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	if existing, exists := scope.Vars()[getEntry.VarName]; exists {
		return acceptAnswer(execState, getEntry, scope, existing)
	}
	if execState.noPrompts {
		return getFromNoPrompts(execState, getEntry, scope)
	}
	return getInteractive(execState, getEntry, scope)
}

func getFromNoPrompts(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	if getEntry.Optional {
		return acceptAnswer(execState, getEntry, scope, value.Nil())
	}
	if getEntry.DefaultTmpl.IsZero() {
		return fmt.Errorf("--no-input: %s requires input; pass %s=VALUE on the command line (run 'hobnob --help' for details)", getEntry.VarName, getEntry.VarName)
	}
	val, err := getEntry.DefaultTmpl.Value(scope.Vars())
	if err != nil {
		return fmt.Errorf("get %s default: %w", getEntry.VarName, err)
	}
	return acceptAnswer(execState, getEntry, scope, val)
}

// acceptAnswer takes an answer nobody typed at a prompt (a value already in
// scope, a default, or no answer to an optional entry), sets it and
// validates it. Setting a value already in scope claims it, so a later
// module vars: default can't swap it out from under the task.
func acceptAnswer(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, val value.Value) error {
	if getEntry.Multi {
		list, err := multiList(getEntry.VarName, val)
		if err != nil {
			return err
		}
		val = list
	}
	scope.Set(getEntry.VarName, val, getEntry.Secret)
	if getEntry.Optional && val.IsEmpty() {
		return nil
	}
	return validateGetValue(execState, getEntry, scope.Vars())
}

func getInteractive(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	info, err := eval.EvalTemplate(getEntry.Info, scope.Vars())
	if err != nil {
		return fmt.Errorf("get %s info: %w", getEntry.VarName, err)
	}

	defaultVal := ""
	if !getEntry.DefaultTmpl.IsZero() {
		defaultVal, err = getEntry.DefaultTmpl.Text(scope.Vars())
		if err != nil {
			return fmt.Errorf("get %s default: %w", getEntry.VarName, err)
		}
	}

	fromItems, err := eval.ResolveItemStrings(getEntry.FromList, getEntry.FromTmpl.Value, scope.Vars(), "get "+getEntry.VarName)
	if err != nil {
		return err
	}

	var val value.Value
	if len(fromItems) > 0 {
		val, err = promptSelectUntilValid(execState, getEntry, fromItems, info, defaultVal, scope.Vars())
		if err != nil {
			return err
		}
	} else {
		checkValidate := checkValidator(execState.ctx, getEntry, scope.Vars())
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
	if execState.noPrompts && (len(getEntry.FromList) > 0 || !getEntry.FromTmpl.IsZero()) {
		items, err := eval.ResolveItemStrings(getEntry.FromList, getEntry.FromTmpl.Value, vars, "get "+getEntry.VarName)
		if err != nil {
			return err
		}
		optSet := make(map[string]bool, len(items))
		for _, opt := range items {
			optSet[opt] = true
		}
		if getEntry.Multi {
			selected, _ := vars[getEntry.VarName].Any().([]any) // a list after multiList
			for _, sel := range selected {
				sel := value.Of(sel).String()
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

// multiList returns a multi: answer that nobody picked at a prompt as a
// list. A list is taken as is, empty is no picks, and text must parse
// strictly as a JSON list: env files and the shell's environment only give
// text, and multi: already declares a list, so this is no guess. Any other
// value is an error rather than quietly becoming a list of one.
func multiList(name string, v value.Value) (value.Value, error) {
	switch {
	case v.Kind() == value.KindArray:
		return v, nil
	case v.IsEmpty():
		return value.Of([]any{}), nil
	case v.Kind() == value.KindString:
		if parsed, err := value.Parse(v.String()); err == nil && parsed.Kind() == value.KindArray {
			return parsed, nil
		}
	}
	return value.Value{}, fmt.Errorf("get %s: multi: %s must be a JSON list, like %s='[\"a\",\"b\"]', got %q", name, name, name, v.String())
}
