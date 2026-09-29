package runner

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
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

// execCtx bundles the state threaded through every step-execution function.
// scope is kept separate — a call step swaps in a childScope while
// cfg/task/noPrompts/dir change together as a unit. memo is shared across the
// whole run (one ExecuteTask call), including through call:'s scope swap —
// that's what lets a once: true prologue replay into two sibling sandboxes.
// fileScopes is likewise fixed for the whole run: every module's file scope,
// looked up by Task.Cfg whenever a module task starts.
type execCtx struct {
	ctx        context.Context
	cfg        *config.ConfigFile
	fileScopes scope.FileScopes
	task       string
	noPrompts  bool
	dir        string
	memo       *callMemo
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
// For CLI invocations pass invocationDir; call: goes through executeTask.
// This is the entry point for one whole run: it owns the once: memo cache,
// which lives for the lifetime of this call (and everything it recursively
// executes) and no longer.
func ExecuteTask(ctx context.Context, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, noPrompts bool, parentDir string) error {
	return executeTask(execCtx{ctx: ctx, cfg: cfg, fileScopes: fileScopes, noPrompts: noPrompts, dir: parentDir, memo: newCallMemo()}, taskName, "", scope)
}

// executeTask is the one task-invocation path, shared by the CLI entry point
// and every call:. It owns the whole sequence — resolve the task and its
// owning file, apply that file's file scope, resolve the working directory,
// evaluate the task's own if:, then run its steps — so no caller can skip a
// part of it. callDirTmpl is a call: step's own dir: ("" for none), resolved
// against the caller's taskfile dir; the priority chain is call-site dir:
// (Priority A) > task dir: (B) > inherited execState.dir (C). It runs within
// an already-established execCtx, carrying its memo forward, so a once:
// task's memoized results survive the sandbox swap at a call: boundary.
func executeTask(execState execCtx, taskName, callDirTmpl string, scope *scope.Scope) error {
	task, execCfg, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}
	if task.Cfg != nil {
		execState.fileScopes.Apply(scope, task.Cfg)
	}
	currentDir := execState.dir
	switch {
	case callDirTmpl != "":
		resolved, err := eval.EvalTemplate(callDirTmpl, scope.Vars())
		if err != nil {
			return fmt.Errorf("dir template: %w", err)
		}
		currentDir = resolveDirPath(resolved, execState.cfg.TaskfileDir)
	case task.Dir != "":
		resolved, err := eval.EvalTemplate(task.Dir, scope.Vars())
		if err != nil {
			return fmt.Errorf("task %q dir: %w", taskName, err)
		}
		currentDir = resolveDirPath(resolved, execCfg.TaskfileDir)
	}
	if task.IfExpr != "" {
		ok, err := eval.EvalCondition(execState.ctx, task.IfExpr, scope.Vars(), currentDir)
		if err != nil {
			return fmt.Errorf("task %q if: %w", taskName, err)
		}
		if !ok {
			fmt.Println(tui.SkipLine(taskName))
			return nil
		}
	}
	return executeSteps(execCtx{ctx: execState.ctx, cfg: execCfg, fileScopes: execState.fileScopes, task: taskName, noPrompts: execState.noPrompts, dir: currentDir, memo: execState.memo}, task.Steps, scope)
}

func executeSteps(execState execCtx, steps []config.Step, scope *scope.Scope) error {
	for _, step := range steps {
		if execState.ctx.Err() != nil {
			return fmt.Errorf("%w: %v", ErrInterrupted, execState.ctx.Err())
		}
		if step.IfExpr != "" {
			ok, err := eval.EvalCondition(execState.ctx, step.IfExpr, scope.Vars(), execState.dir)
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
			if err != nil && step.Soft && !errors.Is(err, ErrInterrupted) {
				err = nil
			}
		case config.KindSet:
			err = execSet(step, scope)
		case config.KindCall:
			err = execCall(execState, step, scope)
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
