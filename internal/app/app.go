// Package app holds hobnob's CLI body — everything cmd/hobnob's main() used
// to do beyond signal handling and os.Exit. It exists so tests (in
// internal/e2e) can drive a real, repeatable run via App.Run without an
// os/exec subprocess, and so the two seams that need faking in tests
// (terminal detection, the interactive task picker) are struct fields instead
// of package-level vars.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runner"
	"hobnob/internal/tui"

	cterm "github.com/charmbracelet/x/term"
)

// ErrSilent signals the caller to exit 1 without printing — Run already
// printed the failure itself.
var ErrSilent = errors.New("")

// App is the CLI body. Construct with New for real use; tests build one
// directly to substitute IsTerminal/SelectTask.
type App struct {
	Version    string
	IsTerminal func() bool
	SelectTask func(ctx context.Context, tasks []tui.TaskItem) (string, error)
}

// New builds an App wired to the real terminal and the real interactive task
// picker.
func New(version string) *App {
	return &App{
		Version: version,
		IsTerminal: func() bool {
			return cterm.IsTerminal(os.Stdin.Fd())
		},
		SelectTask: tui.PromptTaskSelect,
	}
}

func loadConfig(ctx context.Context, path string, cliVars map[string]string, invDir string) (*config.ConfigFile, *cli.Scope, error) {
	cfg, err := config.ParseConfig(path)
	if err != nil {
		return nil, nil, err
	}
	scope, err := cli.BuildScope(ctx, cfg.EnvFileTmpls, cfg.ConstEntries, cfg.VarEntries, cliVars, cfg.TaskfileDir, invDir)
	if err != nil {
		return nil, nil, err
	}
	if err := config.LoadModules(ctx, cfg, scope.Vars, scope.Secrets); err != nil {
		return nil, nil, err
	}
	return cfg, scope, nil
}

func (a *App) selectAndRun(ctx context.Context, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, showUsage bool) error {
	if noPrompts || !a.IsTerminal() {
		if showUsage {
			cli.PrintUsage(os.Stdout, a.Version)
		}
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	tasks := cli.CollectSelectableTasks(cfg, scope)
	if len(tasks) == 0 {
		cli.PrintUsage(os.Stdout, a.Version)
		return cli.ListTasks(cfg, scope, os.Stdout)
	}
	if showUsage {
		cli.PrintUsage(os.Stdout, a.Version)
	}
	selected, err := a.SelectTask(ctx, tasks)
	if err != nil {
		return err
	}
	return a.execTask(ctx, selected, scope, cfg, false, cfg.TaskfileDir)
}

func (a *App) execTask(ctx context.Context, taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, dir string) error {
	if err := runner.ExecuteTask(ctx, taskName, scope, cfg, noPrompts, dir); err != nil {
		if errors.Is(err, runner.ErrInterrupted) {
			fmt.Fprintln(os.Stderr, tui.SError.Render("✗")+" "+tui.TaskPrefix(taskName)+tui.SError.Render("interrupted"))
			return ErrSilent
		}
		fmt.Fprintln(os.Stderr, tui.SError.Render("✗")+" "+tui.TaskPrefix(taskName)+tui.SError.Render(err.Error()))
		return ErrSilent
	}
	fmt.Println(tui.SChecked.Render("✓") + " " + tui.TaskPrefix(taskName) + tui.SChecked.Render("done"))
	return nil
}

// Run is the CLI body: parse args, load the taskfile, dispatch. It never
// reads os.Args or calls os.Exit, so it's safe to call repeatedly from a
// single test process.
func (a *App) Run(ctx context.Context, args []string) error {
	fileFlag, args, err := extractFileFlag(args)
	if err != nil {
		return err
	}

	if len(args) > 0 {
		switch args[0] {
		case "--version":
			fmt.Fprintln(os.Stdout, "hobnob "+cli.DisplayVersion(a.Version))
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
			noPrompts := a.defaultNoPrompts()
			if _, ok := cfg.Tasks["default"]; ok {
				return a.execTask(ctx, "default", scope, cfg, noPrompts, cfg.TaskfileDir)
			}
			return a.selectAndRun(ctx, scope, cfg, noPrompts, true)
		}
		cli.PrintUsage(os.Stdout, a.Version)
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
			return cli.PrintHelp(cfg, scope, os.Stdout, a.Version)
		case "--select":
			noPrompts := a.defaultNoPrompts() || hasNoInputFlag(args[1:])
			return a.selectAndRun(ctx, scope, cfg, noPrompts, false)
		default:
			return cli.ListTasks(cfg, scope, os.Stdout)
		}
	}

	taskName := args[0]
	noPrompts, cliVars, err := a.parseTaskArgs(args[1:])
	if err != nil {
		return fmt.Errorf("invalid argument %w", err)
	}

	cfg, scope, err := loadConfig(ctx, taskfilePath, cliVars, invDir)
	if err != nil {
		return err
	}

	return a.execTask(ctx, taskName, scope, cfg, noPrompts, cfg.TaskfileDir)
}
