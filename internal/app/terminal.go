package app

import (
	"context"
	"os"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runevents"
	"hobnob/internal/runner"
	"hobnob/internal/tui"
)

// terminalFrontEnd prints the run on the terminal and asks on the keyboard.
type terminalFrontEnd struct {
	app   *App
	quiet bool
}

func (t *terminalFrontEnd) AlwaysPicks() bool { return false }

func (t *terminalFrontEnd) CanPick(noPrompts bool) bool {
	return !noPrompts && t.app.IsTerminal()
}

func (t *terminalFrontEnd) Pick(ctx context.Context, tasks []tui.TaskItem, showUsage bool) (string, bool, error) {
	if showUsage {
		cli.PrintUsage(os.Stdout, t.app.Version)
	}
	name, err := t.app.SelectTask(ctx, tasks)
	return name, err == nil, err
}

func (t *terminalFrontEnd) Begin(ctx context.Context, _ *config.ConfigFile, _ string, _ bool) (frontRun, error) {
	log := t.app.newRunLog(t.quiet)
	return frontRun{
		Ctx:     ctx,
		Events:  runevents.New(log),
		Log:     log,
		Prompts: runner.TerminalPrompts(),
		Done:    func(context.Context) bool { return false },
	}, nil
}

func (t *terminalFrontEnd) Close() {}

// newRunLog is the run's log on this terminal, quiet if asked.
func (a *App) newRunLog(quiet bool) *tui.RunLog {
	log := tui.NewRunLog(os.Stdout, os.Stderr)
	log.SetQuiet(quiet)
	if a.Screen != nil {
		log.Watch(a.Screen())
	}
	return log
}
