package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"hobnob/internal/app"
	"hobnob/internal/runner"
)

var version string // injected at build time via -ldflags="-X main.version=vX.Y.Z"

func main() {
	// signal.NotifyContext's stop() does restore the OS-default disposition
	// (go doc os/signal.NotifyContext) — a 2nd CTRL+C after stop() would go
	// back to killing hobnob outright, not get swallowed. But that default
	// termination happens before we'd get a chance to clean up: a run: step
	// that's ignoring the graceful SIGTERM needs a harder signal sent to its
	// process group first. Track both presses explicitly instead: the 1st
	// cancels ctx for a graceful shutdown, the 2nd force-kills any stuck
	// step's process group before exiting. SIGHUP (the terminal closing) is
	// handled like a CTRL+C, so the run still stops what its steps left running.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigCh
		cancel()
		<-sigCh
		// ctx cancellation already asked the running step to terminate
		// gracefully (SIGTERM to its own process group) and, if hobnob is
		// blocked on a get: prompt instead, already unblocks that prompt too
		// (see tea.WithContext in tui.PromptText/PromptSelect). Only force-exit
		// here if a step is actually running and might be ignoring SIGTERM —
		// otherwise let run() return and unwind normally, so bubbletea gets to
		// restore the terminal before the process exits instead of racing an
		// unconditional os.Exit against its (async) teardown.
		if runner.KillRunningStep() {
			os.Exit(1)
		}
	}()

	if err := app.New(version).Run(ctx, os.Args[1:]); err != nil {
		os.Exit(1)
	}
}
