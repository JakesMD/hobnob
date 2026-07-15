package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

// findTaskfile walks startDir and its parents until it finds hobnob.yml or
// hobnob.yaml, returning the absolute path of the first match.
func findTaskfile(startDir string) (string, error) {
	dir := startDir
	for {
		for _, name := range []string{"hobnob.yml", "hobnob.yaml"} {
			p := filepath.Join(dir, name)
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no hobnob.yml or hobnob.yaml found in %s or any parent directory", startDir)
		}
		dir = parent
	}
}

// extractFileFlag scans args for --file <value>, removes both tokens, and
// returns the value alongside the remaining args. Returns ("", args, nil) if
// the flag is absent.
func extractFileFlag(args []string) (string, []string, error) {
	for i, arg := range args {
		if arg == "--file" {
			if i+1 >= len(args) {
				return "", nil, fmt.Errorf("flag --file requires an argument")
			}
			remaining := make([]string, 0, len(args)-2)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+2:]...)
			return args[i+1], remaining, nil
		}
	}
	return "", args, nil
}

// defaultNoPrompts reports whether prompts should be skipped based on the
// environment alone (CI env var set, or stdin not a terminal), before any
// --no-input flag is factored in. Every place that computes noPrompts must
// go through this so CI/terminal detection can't drift between them.
func defaultNoPrompts() bool {
	return os.Getenv("CI") != "" || !isTerminalFn()
}

func parseTaskArgs(args []string) (noPrompts bool, cliVars map[string]string, err error) {
	noPrompts = defaultNoPrompts()
	cliVars = make(map[string]string)
	for _, arg := range args {
		if arg == "--no-input" {
			noPrompts = true
			continue
		}
		idx := strings.IndexByte(arg, '=')
		if idx <= 0 {
			return false, nil, fmt.Errorf("%q (expected KEY=VALUE or --no-input)", arg)
		}
		cliVars[arg[:idx]] = arg[idx+1:]
	}
	return noPrompts, cliVars, nil
}

func loadConfig(path string, cliVars map[string]string, invDir string) (*config.ConfigFile, *cli.Scope, error) {
	cfg, err := config.ParseConfig(path)
	if err != nil {
		return nil, nil, err
	}
	scope, err := cli.BuildScope(cfg.Vars, cliVars, cfg.TaskfileDir, invDir)
	if err != nil {
		return nil, nil, err
	}
	if err := config.LoadModules(cfg, scope.Vars); err != nil {
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
		fmt.Fprintln(os.Stdout, "No tasks available.")
		return nil
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

	if len(args) > 0 && args[0] == "--version" {
		v := version
		if v == "" {
			v = "dev"
		}
		fmt.Fprintln(os.Stdout, "hobnob "+v)
		return nil
	}

	if len(args) > 0 && args[0] == "--upgrade" {
		return selfUpgrade()
	}

	if len(args) > 0 && args[0] == "completion" {
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

	if len(args) > 0 && args[0] != "--list" && strings.HasPrefix(args[0], "_") {
		return fmt.Errorf("task %q is internal and cannot be called directly", args[0])
	}

	invDir, err := os.Getwd()
	if err != nil {
		return err
	}

	if len(args) < 1 {
		var tfPath string
		if fileFlag != "" {
			tfPath = fileFlag
		} else {
			tfPath, _ = findTaskfile(invDir) // error = "not found"; tfPath=="" handles it below
		}
		if tfPath != "" {
			cfg, scope, err := loadConfig(tfPath, nil, invDir)
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

	var taskfilePath string
	if fileFlag != "" {
		taskfilePath = fileFlag
	} else {
		taskfilePath, err = findTaskfile(invDir)
		if err != nil {
			return err
		}
	}

	if args[0] == "--list" || args[0] == "--help" || args[0] == "--select" {
		cfg, scope, err := loadConfig(taskfilePath, nil, invDir)
		if err != nil {
			return err
		}
		switch args[0] {
		case "--help":
			return cli.PrintHelp(cfg, scope, os.Stdout, version)
		case "--select":
			noPrompts := defaultNoPrompts()
			for _, arg := range args[1:] {
				if arg == "--no-input" {
					noPrompts = true
				}
			}
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

	cfg, scope, err := loadConfig(taskfilePath, cliVars, invDir)
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
