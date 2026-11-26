package runner

import (
	"fmt"
	"os"

	"hobnob/internal/ask"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/glob"
	"hobnob/internal/scope"
	"hobnob/internal/value"
)

// pathKind names what a type: file|dir entry needs, for error messages.
func pathKind(getEntry config.GetEntry) string {
	if getEntry.Type == config.GetTypeDir {
		return "directory"
	}
	return "file"
}

// pathRules builds the rules a type: file|dir entry's answer is judged by,
// match: becoming the rules' predicates.
func pathRules(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, defaults []string) (ask.PathRules, error) {
	matcher, patterns, err := resolveMatch(getEntry, scope.Vars())
	if err != nil {
		return ask.PathRules{}, err
	}
	rules := ask.PathRules{
		Dir:      getEntry.Type == config.GetTypeDir,
		Invoke:   scope.InvocationDir(),
		WorkDir:  execState.dir,
		Defaults: defaults,
		Match:    patterns,
	}
	if !matcher.Empty() {
		home, _ := os.UserHomeDir()
		rules.Included = func(abs string, _ bool) bool { return matcher.Included(abs, execState.dir, home) }
		rules.Excluded = func(abs string, _ bool) bool { return matcher.Excluded(abs, execState.dir, home) }
	}
	return rules, nil
}

// acceptPath is acceptAnswer for a type: file|dir entry. val is a path
// nobody picked; relative paths resolve against base. It rewrites the value
// in place as absolute path(s), then runs check:.
func acceptPath(execState execCtx, getEntry config.GetEntry, scope *scope.Scope, val value.Value, base string) error {
	name := getEntry.VarName
	if val.IsEmpty() || (getEntry.Multi && emptyList(val)) {
		if getEntry.Optional {
			scope.Set(name, val, getEntry.Secret)
			return nil
		}
		return fmt.Errorf("get %s: no %s given; %s needs a %s", name, pathKind(getEntry), name, pathKind(getEntry))
	}
	rules, err := pathRules(execState, getEntry, scope, nil)
	if err != nil {
		return err
	}
	written := []any{val.String()}
	if getEntry.Multi {
		written, _ = val.Any().([]any)
	}
	abs := make([]any, 0, len(written))
	for _, item := range written {
		path, refusal := rules.Verify(value.Of(item).String(), base)
		if refusal != "" {
			return fmt.Errorf("get %s: %s", name, refusal)
		}
		abs = append(abs, path)
	}
	val = value.Of(abs[0])
	if getEntry.Multi {
		val = value.Of(abs)
	}
	scope.Set(name, val, getEntry.Secret)
	return validateGetValue(execState, getEntry, scope.Vars())
}

// isDirDefault reports whether a type: file entry's default names a
// directory, which means "start browsing here" and not an answer.
func isDirDefault(getEntry config.GetEntry, val value.Value, base string) bool {
	if getEntry.Type != config.GetTypeFile || val.IsEmpty() {
		return false
	}
	abs, err := ask.PathRules{}.Resolve(val.String(), base)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	return err == nil && info.IsDir()
}

// getPathInteractive puts a type: file|dir entry to the picker as a question
// whose judge verifies each picked path and runs check:.
func getPathInteractive(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	name := getEntry.VarName
	info, err := eval.EvalTemplate(getEntry.Info, scope.Vars())
	if err != nil {
		return fmt.Errorf("get %s info: %w", name, err)
	}
	var defaults []string
	if !getEntry.DefaultTmpl.IsZero() {
		val, err := getEntry.DefaultTmpl.Value(scope.Vars())
		if err != nil {
			return fmt.Errorf("get %s default: %w", name, err)
		}
		if !val.IsEmpty() {
			if items, ok := val.Any().([]any); ok {
				for _, item := range items {
					defaults = append(defaults, value.Of(item).String())
				}
			} else {
				defaults = []string{val.String()}
			}
		}
	}
	rules, err := pathRules(execState, getEntry, scope, defaults)
	if err != nil {
		return err
	}
	question := ask.Question{
		VarName: name, Info: info, Task: execState.task,
		Multi: getEntry.Multi, Secret: getEntry.Secret, Optional: getEntry.Optional,
		Path: &rules,
	}
	if getEntry.Check != "" {
		question.Check = checkCandidate(execState.ctx, getEntry, scope.Vars(), glob.Matcher{})
		question.CheckDesc = getEntry.Check
	}
	resume := execState.log.Pause()
	val, err := execState.prompts.Ask(execState.ctx, question)
	resume()
	if err != nil {
		return wrapPromptErr("get "+name, err)
	}
	scope.Set(name, val, getEntry.Secret)
	logVar(execState, getEntry, scope, "")
	return nil
}

// emptyList reports whether val is a list with nothing in it.
func emptyList(val value.Value) bool {
	items, ok := val.Any().([]any)
	return ok && len(items) == 0
}
