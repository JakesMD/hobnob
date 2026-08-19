package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runner"
	"hobnob/internal/tui"

	cterm "github.com/charmbracelet/x/term"
)

var version string // injected at build time via -ldflags="-X main.version=vX.Y.Z"

// errSilent signals main to exit 1 without printing — run() already printed.
var errSilent = errors.New("")

func loadConfig(ctx context.Context, path string, cliVars map[string]string, invDir string) (*config.ConfigFile, *cli.Scope, error) {
	cfg, err := config.ParseConfig(path)
	if err != nil {
		return nil, nil, err
	}
	scope, err := cli.BuildScope(ctx, cfg.Vars, cfg.EnvFileTmpls, cliVars, cfg.TaskfileDir, invDir)
	if err != nil {
		return nil, nil, err
	}
	if err := config.LoadModules(ctx, cfg, scope.Vars, scope.Secrets); err != nil {
		return nil, nil, err
	}
	return cfg, scope, nil
}

func selectAndRun(ctx context.Context, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, showUsage bool) error {
	if noPrompts || !isTerminalFn() {
		if showUsage {
			cli.PrintUsage(os.Stdout, version)
		}
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	tasks := cli.CollectSelectableTasks(cfg, scope)
	if len(tasks) == 0 {
		cli.PrintUsage(os.Stdout, version)
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	if showUsage {
		cli.PrintUsage(os.Stdout, version)
	}
	selected, err := tui.PromptTaskSelect(ctx, tasks)
	if err != nil {
		return err
	}
	return execTask(ctx, selected, scope, cfg, false, cfg.TaskfileDir)
}

// isTerminalFn reports whether stdin is an interactive terminal. It's a
// package-level var so tests can fake it — swapped out the same way
// promptTextFn/promptSelectFn are in the runner package.
var isTerminalFn = func() bool {
	return cterm.IsTerminal(os.Stdin.Fd())
}

func execTask(ctx context.Context, taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, dir string) error {
	if err := runner.ExecuteTask(ctx, taskName, scope, cfg, noPrompts, dir); err != nil {
		if errors.Is(err, runner.ErrInterrupted) {
			fmt.Fprintln(os.Stderr, tui.SError.Render("✗")+" "+tui.TaskPrefix(taskName)+tui.SError.Render("interrupted"))
			return errSilent
		}
		fmt.Fprintln(os.Stderr, tui.SError.Render("✗")+" "+tui.TaskPrefix(taskName)+tui.SError.Render(err.Error()))
		return errSilent
	}
	fmt.Println(tui.SChecked.Render("✓") + " " + tui.TaskPrefix(taskName) + tui.SChecked.Render("done"))
	return nil
}

func run(ctx context.Context, args []string) error {
	fileFlag, args, err := extractFileFlag(args)
	if err != nil {
		return err
	}

	if len(args) > 0 {
		switch args[0] {
		case "--version":
			fmt.Fprintln(os.Stdout, "hobnob "+cli.DisplayVersion(version))
			return nil
		case "--upgrade":
			return selfUpgrade()
		case "completion":
			if len(args) < 2 {
				return fmt.Errorf("usage: hobnob completion [bash|zsh|fish]")
			}
			script, err := cli.CompletionScript(args[1])
			if err != nil {
				return err
			}
			fmt.Print(script)
			return nil
		}
		if args[0] != "--list" && strings.HasPrefix(args[0], "_") {
			return fmt.Errorf("task %q is internal and cannot be called directly", args[0])
		}
	}

	invDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) < 1 {
		tfPath, _ := resolveTaskfile(fileFlag, invDir) // error = "not found"; tfPath=="" handles it below
		if tfPath != "" {
			cfg, scope, err := loadConfig(ctx, tfPath, nil, invDir)
			if err != nil {
				return err
			}
			noPrompts := defaultNoPrompts()
			if _, ok := cfg.Tasks["default"]; ok {
				return execTask(ctx, "default", scope, cfg, noPrompts, cfg.TaskfileDir)
			}
			return selectAndRun(ctx, scope, cfg, noPrompts, true)
		}
		cli.PrintUsage(os.Stdout, version)
		return nil
	}

	taskfilePath, err := resolveTaskfile(fileFlag, invDir)
	if err != nil {
		return err
	}

	if args[0] == "--list" || args[0] == "--help" || args[0] == "--select" {
		cfg, scope, err := loadConfig(ctx, taskfilePath, nil, invDir)
		if err != nil {
			return err
		}
		switch args[0] {
		case "--help":
			return cli.PrintHelp(cfg, scope, os.Stdout, version)
		case "--select":
			noPrompts := defaultNoPrompts() || hasNoInputFlag(args[1:])
			return selectAndRun(ctx, scope, cfg, noPrompts, false)
		default:
			return cli.ListTasks(cfg, scope, os.Stdout)
		}
	}

	taskName := args[0]
	noPrompts, cliVars, err := parseTaskArgs(args[1:])
	if err != nil {
		return fmt.Errorf("invalid argument %w", err)
	}

	cfg, scope, err := loadConfig(ctx, taskfilePath, cliVars, invDir)
	if err != nil {
		return err
	}

	return execTask(ctx, taskName, scope, cfg, noPrompts, cfg.TaskfileDir)
}

func main() {
	// signal.NotifyContext's stop() does restore the OS-default disposition
	// (go doc os/signal.NotifyContext) — a 2nd CTRL+C after stop() would go
	// back to killing hobnob outright, not get swallowed. But that default
	// termination happens before we'd get a chance to clean up: a run: step
	// that's ignoring the graceful SIGTERM needs a harder signal sent to its
	// process group first. Track both presses explicitly instead: the 1st
	// cancels ctx for a graceful shutdown, the 2nd force-kills any stuck
	// step's process group before exiting.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-sigCh
		fmt.Fprintln(os.Stderr, tui.SStep.Render("!")+" shutting down, press CTRL+C again to force")
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

	if err := run(ctx, os.Args[1:]); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
