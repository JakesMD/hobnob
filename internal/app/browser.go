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

// browserFrontEnd shows the run on a page: the run log still prints to the
// terminal, and the page observes the same run. The page opens on first use.
type browserFrontEnd struct {
	app    *App
	port   int
	quiet  bool
	server *browser.Server
}

func (b *browserFrontEnd) AlwaysPicks() bool { return true }

func (b *browserFrontEnd) CanPick(bool) bool { return true }

func (b *browserFrontEnd) Pick(ctx context.Context, tasks []tui.TaskItem, _ bool) (string, bool, error) {
	server, err := b.page()
	if err != nil {
		return "", false, err
	}
	offered := make([]browser.Task, len(tasks))
	for i, task := range tasks {
		offered[i] = browser.Task{Name: task.Name, Info: task.Info}
	}
	name, err := server.PickTask(ctx, offered, true)
	return name, err == nil, nil
}

// Begin gives the run a context the page can interrupt, and waits after it:
// for the page to receive how the run ended, or for Home when the page chose
// the task.
func (b *browserFrontEnd) Begin(ctx context.Context, cfg *config.ConfigFile, name string, picked bool) (frontRun, error) {
	if !picked {
		if b.port != 0 {
			return frontRun{}, errors.New("--port can't be used with a task; leave the task out and choose it on the page")
		}
		if err := runner.CheckTask(name, cfg); err != nil {
			return frontRun{}, err
		}
	}
	server, err := b.page()
	if err != nil {
		return frontRun{}, err
	}
	runCtx, stop := context.WithCancel(ctx)
	server.SetInterrupt(stop, func() { runner.KillRunningStep() })
	log := b.app.newRunLog(b.quiet)
	return frontRun{
		Ctx:           runCtx,
		Events:        runevents.New(log, server),
		Log:           log,
		Prompts:       browserPrompts(server),
		AlwaysPrompts: true,
		Done: func(ctx context.Context) bool {
			server.ClearInterrupt()
			stop()
			if picked {
				return server.AwaitHome(ctx)
			}
			server.AwaitDelivery(ctx)
			return false
		},
	}, nil
}

func (b *browserFrontEnd) Close() {
	if b.server != nil {
		b.server.Close()
	}
}

// page serves the page and opens it in the browser, unless --port chose
// the port: then the page is left for a bookmark or a forwarded port.
func (b *browserFrontEnd) page() (*browser.Server, error) {
	if b.server != nil {
		return b.server, nil
	}
	server, err := browser.Listen(b.port)
	if err != nil {
		return nil, err
	}
	go server.Serve()
	tui.HobnobNote(os.Stdout, "Browser mode: "+server.URL())
	b.server = server
	if b.port != 0 {
		return server, nil
	}
	if err := b.app.OpenBrowser(server.URL()); err != nil {
		tui.HobnobWarning(os.Stderr, "couldn't open the browser: "+err.Error())
	}
	return server, nil
}
