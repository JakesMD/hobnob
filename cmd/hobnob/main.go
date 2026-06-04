package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runner"
	"hobnob/internal/tui"
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

func parseTaskArgs(args []string) (noPrompts bool, cliVars map[string]string, err error) {
	noPrompts = os.Getenv("CI") != ""
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

func execTask(taskName string, scope *cli.Scope, cfg *config.ConfigFile, noPrompts bool, dir string) error {
	if err := runner.ExecuteTask(taskName, scope, cfg, noPrompts, dir); err != nil {
		fmt.Fprintln(os.Stderr, tui.SError.Render("✗")+" "+tui.TaskPrefix(taskName)+tui.SError.Render(err.Error()))
		return errSilent
	}
	fmt.Println(tui.SChecked.Render("✓") + " " + tui.TaskPrefix(taskName) + tui.SChecked.Render("done"))
	return nil
}

func run(args []string) error {
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

	if len(args) < 1 {
		invDir, err := os.Getwd()
		if err != nil {
			return err
		}
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
			if _, ok := cfg.Tasks["default"]; ok {
				return execTask("default", scope, cfg, os.Getenv("CI") != "", invDir)
			}
			fmt.Fprintf(os.Stdout, "Tip: name a task \"default\" to run it when no task is specified.\n\n")
			return cli.PrintHelp(cfg, scope, os.Stdout, version)
		}
		fmt.Fprintf(os.Stdout, "Tip: name a task \"default\" to run it when no task is specified.\n\n")
		cli.PrintUsage(os.Stdout, version)
		return nil
	}

	if args[0] == "completion" {
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

	invocationDir, err := os.Getwd()
	if err != nil {
		return err
	}

	var taskfilePath string
	if fileFlag != "" {
		taskfilePath = fileFlag
	} else {
		taskfilePath, err = findTaskfile(invocationDir)
		if err != nil {
			return err
		}
	}

	if args[0] == "--list" || args[0] == "--help" {
		cfg, scope, err := loadConfig(taskfilePath, nil, invocationDir)
		if err != nil {
			return err
		}
		if args[0] == "--help" {
			return cli.PrintHelp(cfg, scope, os.Stdout, version)
		}
		return cli.ListTasks(cfg, scope, os.Stdout)
	}

	taskName := args[0]
	noPrompts, cliVars, err := parseTaskArgs(args[1:])
	if err != nil {
		return fmt.Errorf("invalid argument %w", err)
	}

	cfg, scope, err := loadConfig(taskfilePath, cliVars, invocationDir)
	if err != nil {
		return err
	}

	return execTask(taskName, scope, cfg, noPrompts, invocationDir)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, errSilent) {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
		os.Exit(1)
	}
}
