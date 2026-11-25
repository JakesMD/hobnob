package runner

import (
	"context"
	"fmt"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/glob"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// promptTextFn, promptSelectFn and promptPathFn are package-level so SetPrompts (testseam.go)
// can substitute fakes.
var promptTextFn TextPromptFunc = tui.PromptText
var promptSelectFn SelectPromptFunc = tui.PromptSelect
var promptPathFn PathPromptFunc = tui.PromptPath

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
		return acceptAnswer(execState, getEntry, scope, existing, scope.FromInvocation(getEntry.VarName))
	}
	if execState.noPrompts {
		return getFromNoPrompts(execState, getEntry, scope)
	}
	if getEntry.IsPath() {
		return getPathInteractive(execState, getEntry, scope)
	}
	return getInteractive(execState, getEntry, scope)
}

func getFromNoPrompts(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	if getEntry.DefaultTmpl.IsZero() {
		if getEntry.Optional {
			return acceptAnswer(execState, getEntry, scope, value.Nil(), false)
		}
		return fmt.Errorf("--no-input: %s requires input; pass %s=VALUE on the command line (run 'hobnob --help' for details)", getEntry.VarName, getEntry.VarName)
	}
	val, err := getEntry.DefaultTmpl.Value(scope.Vars())
	if err != nil {
		return fmt.Errorf("get %s default: %w", getEntry.VarName, err)
	}
	if isDirDefault(getEntry, val, execState.dir) {
		// A directory only says where a picker would start: with prompts
		// off there's no default.
		if getEntry.Optional {
			return acceptAnswer(execState, getEntry, scope, value.Nil(), false)
		}
		return fmt.Errorf("--no-input: %s requires input; its default is a directory and %s needs a file; pass %s=VALUE on the command line (run 'hobnob --help' for details)", getEntry.VarName, getEntry.VarName, getEntry.VarName)
	}
	if err := takeAnswer(execState, getEntry, scope, val, false); err != nil {
		return err
	}
	logVar(execState, getEntry, scope, "default")
	return nil
}

// acceptAnswer takes an answer nobody typed at a prompt (a value already in
// scope, a default, or no answer to an optional entry), sets it and
// validates it. Setting a value already in scope claims it, so a later
// module vars: default can't swap it out from under the task. fromInvocation
// says val came from the command line or the shell, which decides what a
// relative path in it is relative to; it also puts the note on the variable
// line saying where the value came from.
func acceptAnswer(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, val value.Value, fromInvocation bool) error {
	source := ""
	if fromInvocation {
		source = getSource(scope, getEntry.VarName)
	}
	if err := takeAnswer(execState, getEntry, scope, val, fromInvocation); err != nil {
		return err
	}
	logVar(execState, getEntry, scope, source)
	return nil
}

func takeAnswer(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, val value.Value, fromInvocation bool) error {
	if getEntry.Multi {
		list, err := multiList(getEntry.VarName, val)
		if err != nil {
			return err
		}
		val = list
	}
	if getEntry.IsPath() {
		base := execState.dir
		if fromInvocation {
			base = scope.InvocationDir()
		}
		return acceptPath(execState, getEntry, scope, val, base)
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

	matcher, patterns, err := resolveMatch(getEntry, scope.Vars())
	if err != nil {
		return err
	}

	var val value.Value
	if len(fromItems) > 0 {
		fromItems = filterMatching(fromItems, matcher)
		if len(fromItems) == 0 {
			if getEntry.Optional {
				scope.Set(getEntry.VarName, value.Nil(), getEntry.Secret)
				logVar(execState, getEntry, scope, "")
				return nil
			}
			return fmt.Errorf("get %s: no options match %s", getEntry.VarName, patterns)
		}
		resume := execState.log.Pause()
		val, err = promptSelectFn(execState.ctx, getEntry.VarName, info, fromItems, getEntry.Multi, defaultVal, execState.task, getEntry.Secret,
			getEntry.Optional, selectValidator(execState.ctx, getEntry, scope.Vars()), getEntry.Check)
		resume()
		if err != nil {
			return wrapPromptErr(getEntry.VarName, err)
		}
	} else {
		checkValidate := checkValidator(execState.ctx, getEntry, scope.Vars(), matcher)
		resume := execState.log.Pause()
		text, err := promptTextFn(execState.ctx, info, getEntry.VarName, checkValidate, validatorDesc(getEntry, matcher, patterns), defaultVal, execState.task, getEntry.Secret, getEntry.Optional)
		resume()
		if err != nil {
			return wrapPromptErr(getEntry.VarName, err)
		}
		val = value.Str(text)
	}

	scope.Set(getEntry.VarName, val, getEntry.Secret)
	logVar(execState, getEntry, scope, "")
	return nil
}

// getSource says where a value already in scope came from, for the note on
// its variable line.
func getSource(scope *scope.Scope, name string) string {
	switch {
	case scope.FromCLI(name):
		return "from the command line"
	case scope.FromEnv(name):
		return "from the environment"
	}
	return ""
}

// logVar logs the value a get: entry ended up with, or that an optional
// entry got none.
func logVar(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, source string) {
	val := scope.Vars()[getEntry.VarName]
	if getEntry.Optional && (val.IsEmpty() || emptyList(val)) {
		execState.log.VarSkipped(execState.task, getEntry.VarName)
		return
	}
	execState.log.Var(execState.task, getEntry.VarName, scope.Display(getEntry.VarName), source)
}

// resolveMatch renders getEntry's match: patterns and compiles them. patterns
// is their text, for error messages.
func resolveMatch(getEntry config.GetEntry, vars map[string]value.Value) (glob.Matcher, string, error) {
	ctxName := "get " + getEntry.VarName + " match"
	patterns, err := eval.ResolveItemStrings(getEntry.MatchList, getEntry.MatchTmpl.Value, vars, ctxName)
	if err != nil {
		return glob.Matcher{}, "", err
	}
	matcher, err := glob.New(patterns)
	if err != nil {
		return glob.Matcher{}, "", fmt.Errorf("%s: %w", ctxName, err)
	}
	return matcher, strings.Join(patterns, ", "), nil
}

func filterMatching(items []string, matcher glob.Matcher) []string {
	if matcher.Empty() {
		return items
	}
	kept := make([]string, 0, len(items))
	for _, item := range items {
		if matcher.Text(item) {
			kept = append(kept, item)
		}
	}
	return kept
}

// validatorDesc is what tui shows when a text answer is refused: the check:
// expression, the match: patterns, or both.
func validatorDesc(getEntry config.GetEntry, matcher glob.Matcher, patterns string) string {
	var parts []string
	if getEntry.Check != "" {
		parts = append(parts, getEntry.Check)
	}
	if !matcher.Empty() {
		parts = append(parts, "must match "+patterns)
	}
	return strings.Join(parts, "; ")
}

// checkValidator returns a candidate-validating closure for tui.PromptText,
// or nil when getEntry has neither check: nor match: — tui calls it only if
// non-nil, so it never needs to know how either is evaluated.
func checkValidator(ctx context.Context, getEntry config.GetEntry, vars map[string]value.Value, matcher glob.Matcher) func(string) (bool, error) {
	if getEntry.Check == "" && matcher.Empty() {
		return nil
	}
	return func(candidate string) (bool, error) {
		if !matcher.Empty() && !matcher.Text(candidate) {
			return false, nil
		}
		if getEntry.Check == "" {
			return true, nil
		}
		return eval.EvalCheckWithOverride(ctx, getEntry.Check, vars, getEntry.VarName, value.Str(candidate))
	}
}

// selectValidator returns the check: judge handed to tui.PromptSelect, or nil
// when there is no check. The prompt shows its refusal in place, so it never
// reopens.
func selectValidator(ctx context.Context, getEntry config.GetEntry, vars map[string]value.Value) func(value.Value) (bool, error) {
	if getEntry.Check == "" {
		return nil
	}
	return func(candidate value.Value) (bool, error) {
		return eval.EvalCheckWithOverride(ctx, getEntry.Check, vars, getEntry.VarName, candidate)
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
	if getEntry.Optional && vars[getEntry.VarName].IsEmpty() {
		return nil
	}
	if !getEntry.IsPath() { // a path's match: ran with its other checks
		if err := validateMatch(getEntry, vars); err != nil {
			return err
		}
	}
	if getEntry.Check != "" {
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

// validateMatch checks a value nobody typed at a prompt against match:, each
// item on its own under multi:.
func validateMatch(getEntry config.GetEntry, vars map[string]value.Value) error {
	matcher, patterns, err := resolveMatch(getEntry, vars)
	if err != nil || matcher.Empty() {
		return err
	}
	items := []string{vars[getEntry.VarName].String()}
	if getEntry.Multi {
		items = items[:0]
		selected, _ := vars[getEntry.VarName].Any().([]any) // a list after multiList
		for _, sel := range selected {
			items = append(items, value.Of(sel).String())
		}
	}
	for _, item := range items {
		if !matcher.Text(item) {
			return fmt.Errorf("get %s: %q doesn't match %s", getEntry.VarName, item, patterns)
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
