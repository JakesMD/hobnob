package app

import (
	"context"
	"fmt"
	"os"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runevents"
	"hobnob/internal/runner"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

// frontEnd is how a run is presented and driven: the terminal prints it and
// asks on the keyboard; the browser shows it on a page. App's dispatch only
// ever talks to this.
type frontEnd interface {
	// AlwaysPicks reports whether the picker comes before everything, so a
	// default task and an empty task list don't short-circuit it.
	AlwaysPicks() bool
	// CanPick reports whether the person can choose a task, given noPrompts.
	CanPick(noPrompts bool) bool
	// Pick offers tasks and returns the chosen one; ok is false when nothing
	// more was chosen.
	Pick(ctx context.Context, tasks []tui.TaskItem, showUsage bool) (name string, ok bool, err error)
	// Begin readies one run of name, refusing it when this front end can't.
	// picked is false when the person named the task on the command line.
	Begin(ctx context.Context, cfg *config.ConfigFile, name string, picked bool) (frontRun, error)
	// Close releases what the front end opened.
	Close()
}

// frontRun is what a front end gives ExecuteTask for one run.
type frontRun struct {
	Ctx     context.Context
	Events  *runevents.Stream
	Log     *tui.RunLog
	Prompts runner.Prompts
	// AlwaysPrompts keeps prompts on even when the run was asked for none.
	AlwaysPrompts bool
	// Done is called once the run has returned and its failure is printed. It
	// waits for the front end to finish with the run and reports whether
	// to pick another task.
	Done func(ctx context.Context) (again bool)
}

// frontEnd chooses the front end the flags ask for.
func (a *App) frontEnd(browserMode bool, port int, quiet bool) (frontEnd, error) {
	if port != 0 && !browserMode {
		return nil, fmt.Errorf("--port needs --browser")
	}
	if browserMode {
		return &browserFrontEnd{app: a, port: port, quiet: quiet}, nil
	}
	return &terminalFrontEnd{app: a, quiet: quiet}, nil
}

func (a *App) selectAndRun(ctx context.Context, fe frontEnd, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, noPrompts bool, showUsage bool) error {
	if !fe.CanPick(noPrompts) {
		if showUsage {
			cli.PrintUsage(os.Stdout, a.Version)
		}
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	tasks := cli.CollectSelectableTasks(cfg, scope)
	if len(tasks) == 0 && !fe.AlwaysPicks() {
		cli.PrintUsage(os.Stdout, a.Version)
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	var last error
	for {
		name, ok, err := fe.Pick(ctx, tasks, showUsage)
		if err != nil {
			return err
		}
		if !ok {
			return last
		}
		again, err := a.runTask(ctx, fe, name, true, scope.Copy(), fileScopes, cfg, false, cfg.TaskfileDir)
		last = err
		if !again {
			return last
		}
	}
}

func (a *App) execTask(ctx context.Context, fe frontEnd, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, noPrompts bool, dir string) error {
	_, err := a.runTask(ctx, fe, taskName, false, scope, fileScopes, cfg, noPrompts, dir)
	return err
}

// runTask runs one task on fe and returns how it ended and whether fe wants
// another pick.
func (a *App) runTask(ctx context.Context, fe frontEnd, taskName string, picked bool, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, noPrompts bool, dir string) (again bool, err error) {
	front, err := fe.Begin(ctx, cfg, taskName, picked)
	if err != nil {
		return false, failSilently(err)
	}
	err = failSilently(runner.ExecuteTask(front.Ctx, taskName, scope, fileScopes, cfg, noPrompts && !front.AlwaysPrompts, dir, front.Events, front.Log, front.Prompts))
	return front.Done(ctx), err
}
