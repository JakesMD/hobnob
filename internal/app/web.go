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
	"hobnob/internal/web"
)

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

	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	server.SetInterrupt(stop, func() { runner.KillRunningStep() })

	log := a.newRunLog()
	log.Listen(server)

	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	if err := a.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}

	runErr := runner.ExecuteTask(runCtx, taskName, scope, fileScopes, cfg, false, dir, log, runner.TerminalPrompts())
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
