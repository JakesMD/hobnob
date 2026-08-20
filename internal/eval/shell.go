package eval

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"

	"hobnob/internal/value"
)

// EvalCondition renders condTmpl then passes the result directly to sh -c.
// dir sets the working directory the condition runs in; empty uses the
// current process's working directory. Returns true if the shell command
// exits 0. ctx cancellation kills the shell process outright (no graceful
// SIGTERM/process-group handling here, unlike run: steps — see runner.execRun)
// so an if:/check: expression can't hang past a CTRL+C.
func EvalCondition(ctx context.Context, condTmpl string, vars map[string]value.Value, dir string) (bool, error) {
	rendered, err := EvalTemplate(condTmpl, vars)
	if err != nil {
		return false, err
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", rendered)
	cmd.Dir = dir
	err = cmd.Run()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// EvalCheckWithOverride evaluates check against vars with key temporarily set
// to val, without mutating vars — the shared "does this candidate value pass
// check:" pattern used by interactive re-prompt loops (runner.getInteractive)
// ahead of committing the value to scope.
func EvalCheckWithOverride(ctx context.Context, check string, vars map[string]value.Value, key string, val value.Value) (bool, error) {
	merged := CloneMap(vars)
	merged[key] = val
	return EvalCondition(ctx, check, merged, "")
}

// SourceShellFile sources path in a subshell (dir sets its working directory)
// and returns the vars it set or changed, compared against a baseline `env`
// snapshot taken the same way. This filters out ambient shell noise (e.g.
// SHLVL) that a plain post-source `env` dump would otherwise include.
func SourceShellFile(ctx context.Context, path, dir string) (map[string]string, error) {
	baseline, err := captureEnv(ctx, dir, "env -0")
	if err != nil {
		return nil, fmt.Errorf("capture baseline env: %w", err)
	}
	sourced, err := captureEnv(ctx, dir, fmt.Sprintf("set -a; . %s; env -0", value.ShellQuote(path)))
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	diff := make(map[string]string)
	for key, value := range sourced {
		if baseline[key] != value {
			diff[key] = value
		}
	}
	return diff, nil
}

func captureEnv(ctx context.Context, dir, script string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, err
	}
	vars := make(map[string]string)
	for _, entry := range strings.Split(stdout.String(), "\x00") {
		if key, value, ok := SplitKV(entry); ok {
			vars[key] = value
		}
	}
	return vars, nil
}
