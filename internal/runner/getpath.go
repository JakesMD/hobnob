package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/glob"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// pathKind names what a type: file|dir entry needs, for error messages.
func pathKind(getEntry config.GetEntry) string {
	if getEntry.Type == config.GetTypeDir {
		return "directory"
	}
	return "file"
}

// absPath makes raw absolute: a leading ~ is the home directory, a relative
// path hangs off base, and the result is cleaned. Symlinks are left alone.
func absPath(raw, base string) (string, error) {
	if raw == "~" || strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("can't expand ~ in %s: %w", raw, err)
		}
		raw = filepath.Join(home, raw[1:])
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(base, raw)
	}
	return filepath.Clean(raw), nil
}

// pathChecker runs the path pipeline for one get: entry: absolute, exists,
// right kind, match:. check: is the caller's, since it sees the whole answer.
type pathChecker struct {
	getEntry config.GetEntry
	matcher  glob.Matcher
	patterns string
	workDir  string
	home     string
}

func newPathChecker(getEntry config.GetEntry, vars map[string]value.Value, workDir string) (pathChecker, error) {
	matcher, patterns, err := resolveMatch(getEntry, vars)
	if err != nil {
		return pathChecker{}, err
	}
	home, _ := os.UserHomeDir()
	return pathChecker{getEntry: getEntry, matcher: matcher, patterns: patterns, workDir: workDir, home: home}, nil
}

// item turns raw into an absolute path, resolving a relative one against
// base, and checks it exists, is the right kind and matches match:.
func (pc pathChecker) item(raw, base string) (string, error) {
	name := pc.getEntry.VarName
	abs, err := absPath(raw, base)
	if err != nil {
		return "", fmt.Errorf("get %s: %w", name, err)
	}
	info, err := os.Stat(abs) // follows a symlink: its kind is its target's
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("get %s: %s doesn't exist", name, abs)
		}
		return "", fmt.Errorf("get %s: can't read %s: %w", name, abs, err)
	}
	switch {
	case pc.getEntry.Type == config.GetTypeFile && info.IsDir():
		return "", fmt.Errorf("get %s: %s is a directory; %s needs a file", name, abs, name)
	case pc.getEntry.Type == config.GetTypeDir && !info.IsDir():
		return "", fmt.Errorf("get %s: %s is a file; %s needs a directory", name, abs, name)
	}
	if !pc.matcher.Empty() && !pc.matcher.Path(abs, pc.workDir, pc.home) {
		return "", fmt.Errorf("get %s: %s doesn't match %s", name, abs, pc.patterns)
	}
	return abs, nil
}

// resolvePaths turns val (empty already handled) into absolute path(s),
// each item checked on its own under multi:.
func resolvePaths(checker pathChecker, getEntry config.GetEntry, val value.Value, base string) (value.Value, error) {
	if !getEntry.Multi {
		path, err := checker.item(val.String(), base)
		if err != nil {
			return value.Value{}, err
		}
		return value.Str(path), nil
	}
	items, _ := val.Any().([]any)
	abs := make([]any, 0, len(items))
	for _, item := range items {
		path, err := checker.item(value.Of(item).String(), base)
		if err != nil {
			return value.Value{}, err
		}
		abs = append(abs, path)
	}
	return value.Of(abs), nil
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
	checker, err := newPathChecker(getEntry, scope.Vars(), execState.dir)
	if err != nil {
		return err
	}
	val, err = resolvePaths(checker, getEntry, val, base)
	if err != nil {
		return err
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
	abs, err := absPath(val.String(), base)
	if err != nil {
		return false
	}
	info, err := os.Stat(abs)
	return err == nil && info.IsDir()
}

// getPathInteractive opens the picker for a type: file|dir entry. A picked
// path goes through the same checks as a preset one; a failing check:
// re-opens the picker, showing the evaluation error when the check errored.
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
	checker, err := newPathChecker(getEntry, scope.Vars(), execState.dir)
	if err != nil {
		return err
	}
	prompt := tui.PathPrompt{
		Type:     string(getEntry.Type),
		VarName:  name,
		Info:     info,
		Invoke:   scope.InvocationDir(),
		WorkDir:  execState.dir,
		Default:  defaults,
		Multi:    getEntry.Multi,
		Optional: getEntry.Optional,
	}
	if !checker.matcher.Empty() {
		prompt.Included = func(abs string, _ bool) bool { return checker.matcher.Included(abs, checker.workDir, checker.home) }
		prompt.Excluded = func(abs string, _ bool) bool { return checker.matcher.Excluded(abs, checker.workDir, checker.home) }
	}
	for {
		picked, answered, err := promptPathFn(execState.ctx, prompt, execState.task, getEntry.Secret)
		if err != nil {
			return wrapPromptErr(name, err)
		}
		if !answered {
			if getEntry.Multi {
				picked = value.Of([]any{})
			} else {
				picked = value.Str("")
			}
			scope.Set(name, picked, getEntry.Secret)
			logVar(execState, getEntry, scope, "")
			return nil
		}
		picked, err = resolvePaths(checker, getEntry, picked, prompt.Invoke)
		if err != nil {
			return err
		}
		if getEntry.Check != "" {
			ok, err := eval.EvalCheckWithOverride(execState.ctx, getEntry.Check, scope.Vars(), name, picked)
			if err != nil {
				prompt.Error = fmt.Sprintf("check failed: %v", err)
				continue
			}
			if !ok {
				prompt.Error = ""
				continue
			}
		}
		scope.Set(name, picked, getEntry.Secret)
		logVar(execState, getEntry, scope, "")
		return nil
	}
}

// emptyList reports whether val is a list with nothing in it.
func emptyList(val value.Value) bool {
	items, ok := val.Any().([]any)
	return ok && len(items) == 0
}
