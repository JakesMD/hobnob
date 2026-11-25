package app

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"

	"hobnob/internal/config"
	"hobnob/internal/runner"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
	"hobnob/internal/web"
)

// browserPrompts is what a --web run asks its answers through. Menus and
// pickers aren't in the page yet.
func browserPrompts(server *web.Server) runner.Prompts {
	return runner.Prompts{
		Text: server.PromptText,
		Select: func(context.Context, string, string, []string, bool, string, string, bool, bool,
			func(value.Value) (bool, error), string) (value.Value, error) {
			return value.Value{}, errNotInBrowser
		},
		Path: func(context.Context, tui.PathPrompt, string, bool) (value.Value, bool, error) {
			return value.Value{}, false, errNotInBrowser
		},
	}
}

var errNotInBrowser = errors.New("this prompt isn't supported in browser mode yet")

// openBrowser opens url with the platform's opener.
func openBrowser(url string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	return exec.Command(opener, url).Start()
}

// execWebTask runs taskName behind the web page: the run log still prints to
// the terminal, and the page observes the same run. It returns once the page
// has been sent how the run ended.
func (a *App) execWebTask(ctx context.Context, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, dir string) error {
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

// execWebHome is browser mode without a task: the page offers the visible
// tasks, and each run ends on a screen with Home. It returns the last run's
// outcome, nil when nothing ran.
func (a *App) execWebHome(ctx context.Context, tasks []tui.TaskItem, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile) error {
	server, err := a.openPage()
	if err != nil {
		return err
	}
	defer server.Close()

	offered := make([]web.Task, len(tasks))
	for i, task := range tasks {
		offered[i] = web.Task{Name: task.Name, Info: task.Info}
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

// openPage serves the page and opens it in the browser.
func (a *App) openPage() (*web.Server, error) {
	server, err := web.Listen()
	if err != nil {
		return nil, err
	}
	go server.Serve()
	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	if err := a.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}
	return server, nil
}

// runOnPage runs one task with a fresh run log that the page listens to, and
// tells the page how it ended.
func (a *App) runOnPage(ctx context.Context, server *web.Server, taskName string, scope *scope.Scope, fileScopes scope.FileScopes, cfg *config.ConfigFile, dir string) error {
	log := a.newRunLog()
	log.Listen(server)
	runErr := runner.ExecuteTask(ctx, taskName, scope, fileScopes, cfg, false, dir, log, browserPrompts(server))
	switch {
	case runErr == nil:
		log.RunSucceeded()
	case errors.Is(runErr, runner.ErrInterrupted):
		log.RunCancelled()
	default:
		log.RunFailed(runErr.Error())
	}
	return failSilently(runErr)
}
