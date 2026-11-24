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
	if getEntry.Multi {
		items, _ := val.Any().([]any)
		abs := make([]any, 0, len(items))
		for _, item := range items {
			path, err := checker.item(value.Of(item).String(), base)
			if err != nil {
				return err
			}
			abs = append(abs, path)
		}
		val = value.Of(abs)
	} else {
		path, err := checker.item(val.String(), base)
		if err != nil {
			return err
		}
		val = value.Str(path)
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

// getPathInteractive is the text-prompt fallback for a type: file|dir entry
// until the picker is wired in: the typed answer goes through the same
// checks as a preset one, relative to where hobnob was run.
func getPathInteractive(execState execCtx, getEntry config.GetEntry, scope *scope.Scope) error {
	name := getEntry.VarName
	info, err := eval.EvalTemplate(getEntry.Info, scope.Vars())
	if err != nil {
		return fmt.Errorf("get %s info: %w", name, err)
	}
	defaultVal := ""
	if !getEntry.DefaultTmpl.IsZero() {
		if defaultVal, err = getEntry.DefaultTmpl.Text(scope.Vars()); err != nil {
			return fmt.Errorf("get %s default: %w", name, err)
		}
		if isDirDefault(getEntry, value.Str(defaultVal), execState.dir) {
			defaultVal = ""
		}
	}
	checker, err := newPathChecker(getEntry, scope.Vars(), execState.dir)
	if err != nil {
		return err
	}
	invocationDir := scope.InvocationDir()
	vars := scope.Vars()
	validate := func(candidate string) (bool, error) {
		if candidate == "" {
			return true, nil
		}
		abs, err := checker.item(candidate, invocationDir)
		if err != nil {
			return false, nil
		}
		if getEntry.Check == "" {
			return true, nil
		}
		return eval.EvalCheckWithOverride(execState.ctx, getEntry.Check, vars, name, pathAnswer(getEntry, abs))
	}
	desc := validatorDesc(getEntry, checker.matcher, checker.patterns)
	text, err := promptTextFn(execState.ctx, info, name, validate, desc, defaultVal, execState.task, getEntry.Secret, getEntry.Optional)
	if err != nil {
		return wrapPromptErr(name, err)
	}
	return acceptPath(execState, getEntry, scope, pathAnswerRaw(getEntry, text), invocationDir)
}

// pathAnswer is a single typed path as the value the entry holds.
func pathAnswer(getEntry config.GetEntry, abs string) value.Value {
	if getEntry.Multi {
		return value.Of([]any{abs})
	}
	return value.Str(abs)
}

// pathAnswerRaw is typed text as the value acceptPath takes: empty is no
// answer, anything else one path.
func pathAnswerRaw(getEntry config.GetEntry, text string) value.Value {
	if text == "" {
		if getEntry.Multi {
			return value.Of([]any{})
		}
		return value.Str("")
	}
	return pathAnswer(getEntry, text)
}

// emptyList reports whether val is a list with nothing in it.
func emptyList(val value.Value) bool {
	items, ok := val.Any().([]any)
	return ok && len(items) == 0
}
