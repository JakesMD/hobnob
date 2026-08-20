package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// ErrInterrupted is returned (wrapped) when a run: step's command is cut short
// by ctx cancellation (first CTRL+C), so callers can distinguish a graceful
// shutdown from an ordinary command failure.
var ErrInterrupted = errors.New("interrupted")

// wrapPromptErr classifies a promptTextFn/promptSelectFn error: one caused by
// ctx cancellation (the prompt was torn down mid-input, e.g. a SIGTERM
// arriving while a get: step is blocked waiting on the user) becomes
// ErrInterrupted so callers treat it like any other graceful-shutdown exit,
// rather than an ordinary "aborted" prompt failure.
func wrapPromptErr(varName string, err error) error {
	if tui.IsInterrupted(err) {
		return fmt.Errorf("%w: %v", ErrInterrupted, err)
	}
	return fmt.Errorf("get %s: %w", varName, err)
}

// runningStepMu guards runningStepPID. execRun clears the PID and
// KillRunningStep reads-and-signals it under the same lock so a 2nd CTRL+C
// racing the exact instant a step finishes can't act on a stale PID that the
// OS has since reused for an unrelated process (see setRunningPID /
// clearRunningPID / KillRunningStep in the platform-specific files).
var runningStepMu sync.Mutex
var runningStepPID int

func setRunningPID(pid int) {
	runningStepMu.Lock()
	runningStepPID = pid
	runningStepMu.Unlock()
}

func clearRunningPID() {
	runningStepMu.Lock()
	runningStepPID = 0
	runningStepMu.Unlock()
}

// resolveDirPath returns dir as-is if absolute, else joins it with taskfileDir.
func resolveDirPath(dir, taskfileDir string) string {
	return eval.ResolvePath(dir, taskfileDir)
}

// displayDirPath returns dir relative to invocationDir when dir is invocationDir
// itself or one of its subdirectories, else returns dir unchanged (full path).
func displayDirPath(dir, invocationDir string) string {
	rel, err := filepath.Rel(invocationDir, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return dir
	}
	if rel == "." {
		return "./"
	}
	return "./" + rel
}

// maskSecrets replaces each secret variable's value in text with
// tui.SecretMask. It matches both the raw value and its JSON-escaped form
// (quotes/backslashes/newlines escaped the way json.Marshal would render
// them) — a secret embedded as a leaf of a set:/into: JSON literal is
// marshaled, so its escaped form can differ from the raw value and would
// otherwise slip past a raw-only match.
//
// A Bool or short Number secret is skipped: masking "true" or "1" as a
// substring would blank out unrelated text (every "true" in the command),
// not just the secret.
func maskSecrets(text string, scope *cli.Scope) string {
	for name := range scope.Secrets {
		secretVal := scope.Vars[name]
		if secretVal.Kind() == value.KindBool {
			continue
		}
		if secretVal.Kind() == value.KindNumber && len(secretVal.String()) < 4 {
			continue
		}
		raw := secretVal.String()
		if raw == "" {
			continue
		}
		if escaped, ok := jsonEscapedForm(raw); ok && escaped != raw {
			text = strings.ReplaceAll(text, escaped, tui.SecretMask)
		}
		text = strings.ReplaceAll(text, raw, tui.SecretMask)
	}
	return text
}

// shellMetachars is the set of characters that would need shell quoting if
// argv were ever pasted back into a shell — displayArgv quotes an element
// only when it contains one of these (or is empty/whitespace), so the common
// case reads as a plain command line.
const shellMetachars = " \t\n\"'$`\\|&;<>()*?[]{}!#~"

// displayArgv renders argv as a shell-quoted string for the run: log line —
// display only, argv itself never touches a shell. An element is wrapped in
// value.ShellQuote only when it's empty or holds a character that would
// otherwise be ambiguous, keeping the common line copy-pasteable.
func displayArgv(argv []string) string {
	parts := make([]string, len(argv))
	for i, arg := range argv {
		if arg == "" || strings.ContainsAny(arg, shellMetachars) {
			parts[i] = value.ShellQuote(arg)
		} else {
			parts[i] = arg
		}
	}
	return strings.Join(parts, " ")
}

// jsonEscapedForm returns secretVal as it would appear inside a
// json.Marshal-ed string (unquoted) — the form a secret takes once it's a
// leaf of a set:/into: JSON literal.
func jsonEscapedForm(secretVal string) (string, bool) {
	jsonBytes, err := json.Marshal(secretVal)
	if err != nil || len(jsonBytes) < 2 {
		return "", false
	}
	return string(jsonBytes[1 : len(jsonBytes)-1]), true
}

// execCtx bundles the state threaded through every step-execution function.
// scope is kept separate — a call step swaps in a childScope while
// cfg/task/noPrompts/dir change together as a unit. memo is shared across the
// whole run (one ExecuteTask call), including through call:'s scope swap —
// that's what lets a use: prologue replay into two sibling sandboxes.
type execCtx struct {
	ctx       context.Context
	cfg       *config.ConfigFile
	task      string
	noPrompts bool
	dir       string
	memo      *useMemo
}

func resolveTask(taskName string, cfg *config.ConfigFile) (config.Task, *config.ConfigFile, error) {
	task, ok := cfg.Tasks[taskName]
	if !ok {
		return config.Task{}, nil, fmt.Errorf("task %q not found", taskName)
	}
	execCfg := cfg
	if task.Cfg != nil {
		execCfg = task.Cfg
	}
	return task, execCfg, nil
}

// ExecuteTask runs taskName using parentDir as the inherited working directory.
// If the task defines a top-level dir:, that overrides parentDir (Priority B).
// For CLI invocations pass invocationDir; execCall passes the resolved child dir.
// This is the entry point for one whole run: it owns the use: memo cache, which
// lives for the lifetime of this call (and everything it recursively executes)
// and no longer.
func ExecuteTask(ctx context.Context, taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, parentDir string) error {
	return executeTask(execCtx{ctx: ctx, cfg: cfg, noPrompts: noPrompts, dir: parentDir, memo: newUseMemo()}, taskName, scope)
}

// executeTask resolves and runs taskName within an already-established
// execCtx, carrying its memo forward. call: uses this (rather than the public
// ExecuteTask) so a used task's memoized results survive the sandbox swap at
// a call: boundary.
func executeTask(execState execCtx, taskName string, scope *cli.Scope) error {
	task, execCfg, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}
	if task.Cfg != nil {
		applyModuleLayer(scope, execCfg)
	}
	noPrompts := execState.noPrompts
	if task.Interactive != nil && !*task.Interactive {
		noPrompts = true
	}
	currentDir := execState.dir
	if task.Dir != "" {
		resolved, err := eval.EvalTemplate(task.Dir, scope.Vars)
		if err != nil {
			return fmt.Errorf("task %q dir: %w", taskName, err)
		}
		currentDir = resolveDirPath(resolved, execCfg.TaskfileDir)
	}
	if task.IfExpr != "" {
		ok, err := eval.EvalCondition(execState.ctx, task.IfExpr, scope.Vars, currentDir)
		if err != nil {
			return fmt.Errorf("task %q if: %w", taskName, err)
		}
		if !ok {
			fmt.Println(tui.SkipLine(taskName))
			return nil
		}
	}
	return executeSteps(execCtx{ctx: execState.ctx, cfg: execCfg, task: taskName, noPrompts: noPrompts, dir: currentDir, memo: execState.memo}, task.Steps, scope)
}

// applyModuleLayer supplies moduleCfg's own env:/vars:/const: vars (see
// config.ConfigFile.ModuleLayer/ModuleConstLayer) into scope before a task
// belonging to that module runs — the fix for a module's own env: block
// never having reached a module task's runtime scope. The default tier
// (env:, vars:) is applied via SetIfDefault, so it only fills a key the
// caller hasn't already set (directly, via with:, or via a higher-priority
// layer of its own); const: is applied unconditionally after it, since a
// module's own const: always wins in its subtree — ordinary lexical
// shadowing, the nearest declaration wins. Re-running this on every nested
// call into the same module is idempotent: once a default-tier key is
// applied it's no longer ambient, so a later call is a no-op for it, and
// const: always writes the same value.
func applyModuleLayer(scope *cli.Scope, moduleCfg *config.ConfigFile) {
	for key, val := range moduleCfg.ModuleLayer {
		scope.SetIfDefault(key, val, moduleCfg.ModuleLayerSecrets[key])
	}
	for key, val := range moduleCfg.ModuleConstLayer {
		scope.Set(key, val, moduleCfg.ModuleConstLayerSecrets[key])
	}
}

func executeSteps(execState execCtx, steps []config.Step, scope *cli.Scope) error {
	for _, step := range steps {
		if execState.ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, execState.ctx.Err())
		}
		if step.IfExpr != "" {
			ok, err := eval.EvalCondition(execState.ctx, step.IfExpr, scope.Vars, execState.dir)
			if err != nil {
				return fmt.Errorf("if condition: %w", err)
			}
			if !ok {
				if step.Kind == config.KindRun {
					fmt.Println(tui.RunSkipLine(execState.task))
				}
				continue
			}
		}

		var err error
		switch step.Kind {
		case config.KindRun:
			err = execRun(execState, step, scope)
		case config.KindSet:
			err = execSet(step, scope)
		case config.KindCall:
			err = execCall(execState, step, scope)
			if err != nil && step.Soft && !errors.Is(err, ErrInterrupted) {
				err = nil
			}
		case config.KindUse:
			err = execUse(execState, step, scope)
			if err != nil && step.Soft && !errors.Is(err, ErrInterrupted) {
				err = nil
			}
		case config.KindFor:
			err = execFor(execState, step, scope)
		case config.KindGet:
			err = execGet(execState, step, scope)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
