package cli

import (
	"context"
	"fmt"
	"os"

	"hobnob/internal/config"
	"hobnob/internal/eval"
)

type Scope struct {
	Vars    map[string]string
	Secrets map[string]bool
}

// Set assigns val to key and, when secret is true, flags key as a secret —
// the "set a var and propagate its secret flag" idiom shared by every step
// kind and BuildScope that can introduce a new scope var.
func (scope *Scope) Set(key, val string, secret bool) {
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
// from env: files, then CLI KEY=VALUE args, then global vars evaluated on top
// (highest priority).
// CLI args win over env: files so a caller's explicit override always beats a
// sourced default. Globals win over CLI args because vars: is implementation
// detail — the public API for caller input is get: steps, which are skipped
// when a var is already set.
// Also returns a secrets map for any global vars marked secret: true, and for
// any var sourced from an env: file per its default/override (see config.LoadEnvFiles).
func BuildScope(ctx context.Context, vars []config.SetEntry, envFileEntries []config.EnvFileEntry, cliVars map[string]string, taskfileDir, invocationDir string) (*Scope, error) {
	scope := &Scope{
		Vars:    make(map[string]string),
		Secrets: make(map[string]bool),
	}

	for _, envEntry := range os.Environ() {
		if key, value, ok := eval.SplitKV(envEntry); ok {
			scope.Vars[key] = value
		}
	}

	scope.Vars["HOBNOB_FILE_DIR"] = taskfileDir
	scope.Vars["HOBNOB_INVOCATION_DIR"] = invocationDir

	envFileVars, envFileSecrets, err := config.LoadEnvFiles(ctx, envFileEntries, taskfileDir, scope.Vars)
	if err != nil {
		return nil, err
	}
	for key, value := range envFileVars {
		scope.Set(key, value, envFileSecrets[key])
	}

	for key, value := range cliVars {
		scope.Vars[key] = value
	}

	for _, globalVar := range vars {
		val, err := eval.EvalTemplate(globalVar.ValTmpl, scope.Vars)
		if err != nil {
			return nil, fmt.Errorf("global var %q: %w", globalVar.Key, err)
		}
		scope.Set(globalVar.Key, val, globalVar.Secret)
	}

	return scope, nil
}
