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
	server, err := web.Listen()
	if err != nil {
		return err
	}
	defer server.Close()
	go server.Serve()

	log := a.newRunLog()
	log.Listen(server)

	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	if err := a.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}

	runErr := runner.ExecuteTask(ctx, taskName, scope, fileScopes, cfg, false, dir, log, browserPrompts(server))
	switch {
	case runErr == nil:
		log.RunSucceeded()
	case errors.Is(runErr, runner.ErrInterrupted):
		log.RunCancelled()
	default:
		log.RunFailed(runErr.Error())
	}
	server.AwaitDelivery(ctx)
	return failSilently(runErr)
}
