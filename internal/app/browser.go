package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"

	"hobnob/internal/browser"
	"hobnob/internal/config"
	"hobnob/internal/runevents"
	"hobnob/internal/runner"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
)

// browserPrompts is what a --browser run asks its answers through.
func browserPrompts(server *browser.Server) runner.Prompts {
	return runner.Prompts{
		Ask:  server.PromptAsk,
		Path: server.PromptPath,
		Say:  server.PromptSay,
	}
}

// openBrowser opens url with the platform's opener.
func openBrowser(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

// execBrowserTask runs taskName behind the page: the run log still prints to
// the terminal, and the page observes the same run. It returns once the page
// has been sent how the run ended.
func (a *App) execBrowserTask(ctx context.Context, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, dir string) error {
	if a.port != 0 {
		return errors.New("--port can't be used with a task; leave the task out and choose it on the page")
	}
	if err := runner.CheckTask(taskName, cfg); err != nil {
		return failSilently(err)
	}
	server, err := a.openPage()
	if err != nil {
		return err
	}
	defer server.Close()

	err = a.runOnPage(ctx, server, taskName, scope, fileScopes, cfg, dir)
	server.AwaitDelivery(ctx)
	return err
}

// execBrowserHome is browser mode without a task: the page offers the visible
// tasks, and each run ends on a screen with Home. It returns the last run's
// outcome, nil when nothing ran.
func (a *App) execBrowserHome(ctx context.Context, tasks []tui.TaskItem, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile) error {
	server, err := a.openPage()
	if err != nil {
		return err
	}
	defer server.Close()

	offered := make([]browser.Task, len(tasks))
	for i, task := range tasks {
		offered[i] = browser.Task{Name: task.Name, Info: task.Info}
	}
	var last error
	for {
		taskName, err := server.PickTask(ctx, offered, true)
		if err != nil {
			return last
		}
		last = a.runOnPage(ctx, server, taskName, scope.Copy(), fileScopes, cfg, cfg.TaskfileDir)
		if !server.AwaitHome(ctx) {
			return last
		}
	}
}

// openPage serves the page and opens it in the browser, unless --port chose
// the port: then the page is left for a bookmark or a forwarded port.
func (a *App) openPage() (*browser.Server, error) {
	server, err := browser.Listen(a.port)
	if err != nil {
		return nil, err
	}
	go server.Serve()
	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	if a.port != 0 {
		return server, nil
	}
	if err := a.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}
	return server, nil
}

// runOnPage runs one task with a fresh run log that the page listens to, and
// tells the page how it ended.
func (a *App) runOnPage(ctx context.Context, server *browser.Server, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, dir string) error {
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	server.SetInterrupt(stop, func() { runner.KillRunningStep() })
	defer server.ClearInterrupt()

	log := a.newRunLog()
	events := runevents.New(log, server)
	runErr := runner.ExecuteTask(runCtx, taskName, scope, fileScopes, cfg, false, dir, events, log, browserPrompts(server))
	return failSilently(runErr)
}
