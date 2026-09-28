package cli

import (
	"context"
	"fmt"
	"os"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

type Scope struct {
	Vars    map[string]value.Value
	Secrets map[string]bool
	// Ambient holds true for a key whose current value still comes only from
	// the OS-environment base layer — nothing higher in BuildScope's chain
	// (vars:, env files, CLI args, const:) has touched it since. A key is
	// absent (false) once any higher layer sets it. This is what lets a
	// module's own env:/vars: block decide whether it's safe to supply a
	// default for a var it inherited from the parent scope, without being
	// able to see which layer actually produced that inherited value.
	Ambient map[string]bool
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

// SetIfDefault assigns val to key only when key is unset or still ambient
// (see Scope.Ambient) — never overwriting a value some higher-priority layer
// already produced. Used for a module's own env:/vars: block, which supplies
// a default for its subtree rather than an override. The written value is
// not itself ambient: it came from this module's own file, not the OS
// environment.
func (scope *Scope) SetIfDefault(key string, val value.Value, secret bool) {
	if _, present := scope.Vars[key]; present && !scope.Ambient[key] {
		return
	}
	scope.Set(key, val, secret)
	delete(scope.Ambient, key)
}

func (scope *Scope) Copy() *Scope {
	return &Scope{
		Vars:    eval.CloneMap(scope.Vars),
		Secrets: eval.CloneMap(scope.Secrets),
		Ambient: eval.CloneMap(scope.Ambient),
	}
}

// BuildScope constructs the initial variable scope: env vars < vars: < env:
// files < CLI KEY=VALUE args < const:, per docs/adr/0001. Per that ADR, a
// layer's templates read the final values of every layer above it — so
// layers are *resolved* top-down, the reverse of precedence order: const:
// first (closed world — only earlier const: entries and the builtins),
// then CLI args (plain values, no templates), then env: file paths (which
// may read CLI args, const:, the OS env, and the builtins, but never a
// vars: name — enforced at load time by config.checkEnvPathsDontReferenceVars
// since it would otherwise surface as a confusing runtime "path not found"),
// then vars: last, since it's the fallback layer and so needs to see
// everything above it to build a real default from. A vars: entry whose
// name a higher layer (env file, CLI arg, const:) already set is skipped
// entirely — not evaluated — since it already lost; only a name still on the
// OS-env base layer (Scope.Ambient) is fair game for it to overwrite.
// Above const:, precedence is no longer a rule but execution order: a task's
// own set:/get:/loop:/call: steps run after BuildScope and see everything it
// produced.
// Also returns a secrets map for any var sourced from an env: file or const:/
// vars: entry per its own secret: flag (see config.LoadEnvFiles,
// config.EvalSetEntry).
//
// Every source but const:/vars: is wrapped in value.Str, never sniffed for
// JSON shape — env vars, CLI args, and env-file values are always plain
// strings, no matter how JSON-shaped their text looks. const:/vars: entries
// go through config.EvalSetEntry, same as set:, so a map/list literal stays
// typed. Real structure otherwise only ever enters scope from a set:/with:
// literal, a run: into: capture, or the explicit json filter.
func BuildScope(ctx context.Context, envFileEntries []config.EnvFileEntry, constEntries, varEntries []config.SetEntry, cliVars map[string]string, taskfileDir, invocationDir string) (*Scope, error) {
	scope := &Scope{
		Vars:    make(map[string]value.Value),
		Secrets: make(map[string]bool),
		Ambient: make(map[string]bool),
	}

	for _, envEntry := range os.Environ() {
		if key, val, ok := eval.SplitKV(envEntry); ok {
			scope.Vars[key] = value.Str(val)
			scope.Ambient[key] = true
		}
	}

	scope.Vars["HOBNOB_FILE_DIR"] = value.Str(taskfileDir)
	scope.Vars["HOBNOB_INVOCATION_DIR"] = value.Str(invocationDir)

	if err := evalSetEntriesInto(scope, constEntries, "const", nil); err != nil {
		return nil, err
	}

	for key, val := range cliVars {
		if _, present := scope.Vars[key]; present && !scope.Ambient[key] {
			continue // const: already claimed this name
		}
		scope.Vars[key] = value.Str(val)
		delete(scope.Ambient, key)
	}

	envFileVars, envFileSecrets, err := config.LoadEnvFiles(ctx, envFileEntries, taskfileDir, scope.Vars)
	if err != nil {
		return nil, err
	}
	for key, val := range envFileVars {
		if _, present := scope.Vars[key]; present && !scope.Ambient[key] {
			continue // const: or a CLI arg already claimed this name
		}
		scope.Set(key, value.Str(val), envFileSecrets[key])
		delete(scope.Ambient, key)
	}

	skipClaimed := func(key string) bool {
		_, present := scope.Vars[key]
		return present && !scope.Ambient[key]
	}
	if err := evalSetEntriesInto(scope, varEntries, "vars", skipClaimed); err != nil {
		return nil, err
	}

	return scope, nil
}

// evalSetEntriesInto evaluates entries top-to-bottom into scope, each seeing
// everything set before it (by an earlier entry in the same block, or by a
// lower layer already applied) — the same sequential rule set: follows at
// step scope. label names the block in a wrapped error. skip, when non-nil,
// is checked before evaluating each entry; a true result skips the entry
// entirely, without evaluating its template — used by vars: to leave a
// template that'd only make sense standing alone (e.g. it'd error against
// the scope) unevaluated when a higher layer already claimed its name.
func evalSetEntriesInto(scope *Scope, entries []config.SetEntry, label string, skip func(key string) bool) error {
	for _, entry := range entries {
		if skip != nil && skip(entry.Key) {
			continue
		}
		val, err := config.EvalSetEntry(entry, func(tmpl string) (value.Value, error) {
			return eval.EvalValue(tmpl, scope.Vars)
		})
		if err != nil {
			return fmt.Errorf("%s: %s: %w", label, entry.Key, err)
		}
		scope.Set(entry.Key, val, entry.Secret)
		delete(scope.Ambient, entry.Key)
	}
	return nil
}
