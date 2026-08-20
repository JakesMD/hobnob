package cli

import (
	"context"
	"os"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

type Scope struct {
	Vars    map[string]value.Value
	Secrets map[string]bool
}

// Set assigns val to key and, when secret is true, flags key as a secret —
// the "set a var and propagate its secret flag" idiom shared by every step
// kind and BuildScope that can introduce a new scope var.
func (scope *Scope) Set(key string, val value.Value, secret bool) {
	scope.Vars[key] = val
	if secret {
		scope.Secrets[key] = true
	}
}

func (scope *Scope) Copy() *Scope {
	return &Scope{Vars: eval.CloneMap(scope.Vars), Secrets: eval.CloneMap(scope.Secrets)}
}

// BuildScope constructs the initial variable scope: env vars as the base,
// then system vars (HOBNOB_FILE_DIR, HOBNOB_INVOCATION_DIR), then vars sourced
// from env: files, then CLI KEY=VALUE args (highest priority).
// CLI args win over env: files so a caller's explicit override always beats a
// sourced default. Above this, precedence is no longer a rule but execution
// order: a task's own set:/get:/loop/use: steps run after BuildScope and see
// everything it produced.
// Also returns a secrets map for any var sourced from an env: file per its
// default/override (see config.LoadEnvFiles).
//
// Every source here is wrapped in value.Str, never sniffed for JSON shape —
// env vars, CLI args, and env-file values are always plain strings, no matter
// how JSON-shaped their text looks. Real structure only ever enters scope from
// a set:/with: literal, a run: into: capture, or the explicit json filter.
func BuildScope(ctx context.Context, envFileEntries []config.EnvFileEntry, cliVars map[string]string, taskfileDir, invocationDir string) (*Scope, error) {
	scope := &Scope{
		Vars:    make(map[string]value.Value),
		Secrets: make(map[string]bool),
	}

	for _, envEntry := range os.Environ() {
		if key, val, ok := eval.SplitKV(envEntry); ok {
			scope.Vars[key] = value.Str(val)
		}
	}

	scope.Vars["HOBNOB_FILE_DIR"] = value.Str(taskfileDir)
	scope.Vars["HOBNOB_INVOCATION_DIR"] = value.Str(invocationDir)

	envFileVars, envFileSecrets, err := config.LoadEnvFiles(ctx, envFileEntries, taskfileDir, scope.Vars)
	if err != nil {
		return nil, err
	}
	for key, val := range envFileVars {
		scope.Set(key, value.Str(val), envFileSecrets[key])
	}

	for key, val := range cliVars {
		scope.Vars[key] = value.Str(val)
	}

	return scope, nil
}
