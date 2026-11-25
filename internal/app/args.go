package app

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"hobnob/internal/eval"
)

// findTaskfile walks startDir and its parents until it finds hobnob.yml or
// hobnob.yaml, returning the absolute path of the first match.
func findTaskfile(startDir string) (string, error) {
	dir := startDir
	for {
		for _, name := range []string{"hobnob.yml", "hobnob.yaml"} {
			path := filepath.Join(dir, name)
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no hobnob.yml or hobnob.yaml in %s or any folder above it", startDir)
		}
		dir = parent
	}
}

// resolveTaskfile returns fileFlag if set, else the auto-discovered taskfile
// found by walking up from invDir.
func resolveTaskfile(fileFlag, invDir string) (string, error) {
	if fileFlag != "" {
		return fileFlag, nil
	}
	return findTaskfile(invDir)
}

// extractDemoFlag scans args for --demo, removes it, and reports whether it
// was present. --demo names the built-in demo taskfile as the source, in place
// of --file's path or the upward search for a hobnob.yml.
func extractDemoFlag(args []string) (bool, []string) {
	for i, arg := range args {
		if arg == "--demo" {
			remaining := make([]string, 0, len(args)-1)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+1:]...)
			return true, remaining
		}
	}
	return false, args
}

// extractFlag scans args for any of names, removes every occurrence, and
// reports whether one was present.
func extractFlag(args []string, names ...string) (bool, []string) {
	found := false
	remaining := make([]string, 0, len(args))
	for _, arg := range args {
		if slices.Contains(names, arg) {
			found = true
			continue
		}
		remaining = append(remaining, arg)
	}
	return found, remaining
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
func (a *App) defaultNoPrompts() bool {
	return os.Getenv("CI") != "" || !a.IsTerminal()
}

// hasNoInputFlag reports whether args contains the --no-input flag.
func hasNoInputFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--no-input" {
			return true
		}
	}
	return false
}

func (a *App) parseTaskArgs(args []string) (noPrompts bool, cliVars map[string]string, err error) {
	noPrompts = a.defaultNoPrompts()
	cliVars = make(map[string]string)
	for _, arg := range args {
		if arg == "--no-input" {
			noPrompts = true
			continue
		}
		key, val, ok := eval.SplitKV(arg)
		if !ok {
			return false, nil, fmt.Errorf("%q (expected KEY=VALUE or --no-input)", arg)
		}
		cliVars[key] = val
	}
	return noPrompts, cliVars, nil
}

// checkWebFlag rejects --web where the page has nothing to do: on Windows,
// and beside --no-input, --list, --select and --help.
func checkWebFlag(goos string, args []string) error {
	if goos == "windows" {
		return fmt.Errorf("--web is not yet supported on Windows")
	}
	for _, arg := range args {
		if arg == "--no-input" || isListingFlag(arg) {
			return fmt.Errorf("--web can't be combined with %s", arg)
		}
	}
	return nil
}
