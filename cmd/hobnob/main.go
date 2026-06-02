package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/runner"
)

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

func main() {
	fileFlag, args, err := extractFileFlag(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]\n       hobnob [--file <path>] --list\n       hobnob [--file <path>] --help\n")
		os.Exit(1)
	}

	if args[0] == "completion" {
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "usage: hobnob completion [bash|zsh|fish]\n")
			os.Exit(1)
		}
		script, err := cli.CompletionScript(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(script)
		return
	}

	if args[0] != "--list" && strings.HasPrefix(args[0], "_") {
		fmt.Fprintf(os.Stderr, "task %q is internal and cannot be called directly\n", args[0])
		os.Exit(1)
	}

	invocationDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	var taskfilePath string
	if fileFlag != "" {
		taskfilePath = fileFlag
	} else {
		taskfilePath, err = findTaskfile(invocationDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	cfg, err := config.ParseConfig(taskfilePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if args[0] == "--list" || args[0] == "--help" {
		scope, _, err := cli.BuildScope(cfg.Vars, nil, cfg.TaskfileDir, invocationDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := config.LoadModules(cfg, scope); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if args[0] == "--help" {
			if err := cli.PrintHelp(cfg, scope, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		} else {
			if err := cli.ListTasks(cfg, scope, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
		}
		return
	}

	taskName := args[0]
	noPrompts, cliVars, err := parseTaskArgs(args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid argument %v\n", err)
		os.Exit(1)
	}

	scope, secrets, err := cli.BuildScope(cfg.Vars, cliVars, cfg.TaskfileDir, invocationDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := config.LoadModules(cfg, scope); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if err := runner.ExecuteTask(taskName, scope, cfg, noPrompts, invocationDir, secrets); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
