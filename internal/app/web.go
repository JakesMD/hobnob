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
	server, err := web.Listen()
	if err != nil {
		return err
	}
	defer server.Close()
	go server.Serve()

	log := tui.NewRunLog(os.Stdout, os.Stderr)
	log.SetQuiet(a.quiet)
	log.Listen(server)
	if a.Screen != nil {
		log.Watch(a.Screen())
	}

	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	if err := a.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}

	runErr := runner.ExecuteTask(ctx, taskName, scope, fileScopes, cfg, false, dir, log, runner.TerminalPrompts())
	switch {
	case runErr == nil:
		log.RunSucceeded()
	case errors.Is(runErr, runner.ErrInterrupted):
		log.RunCancelled()
	default:
		log.RunFailed(runErr.Error())
	}
	server.AwaitDelivery(ctx)
	if runErr == nil {
		return nil
	}
	if !runner.Reported(runErr) {
		tui.HobnobError(os.Stderr, runErr.Error())
	}
	return ErrSilent
}
