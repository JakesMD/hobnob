// Package scope holds hobnob's runtime variable scope and builds it: Load
// resolves the root taskfile's scope and every module's file scope, and
// FileScopes.Apply lays a module's file scope onto the scope one of its
// tasks runs with.
package scope

import (
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

type Scope struct {
	Vars    map[string]value.Value
	Secrets map[string]bool
	// Ambient holds true for a key whose current value still comes only from
	// the OS-environment base layer — nothing higher in the chain (vars:,
	// env files, CLI args, const:, a module's file scope, a step) has
	// touched it since. A key is absent (false) once any higher layer sets
	// it. This is what lets a module's own env:/vars: block decide whether
	// it's safe to supply a default for a var it inherited, without being
	// able to see which layer actually produced that inherited value.
	Ambient map[string]bool
}

// Set assigns val to key and, when secret is true, flags key as a secret —
// the "set a var and propagate its secret flag" idiom shared by every step
// kind and Load that can introduce a new scope var.
func (scope *Scope) Set(key string, val value.Value, secret bool) {
	scope.Vars[key] = val
	if secret {
		scope.Secrets[key] = true
	}
}

// SetIfDefault assigns val to key only when key is unclaimed (see claimed) —
// never overwriting a value some higher-priority layer already produced.
// Used for a module's own env:/vars: block, which supplies a default for its
// subtree rather than an override. The written value is not itself ambient:
// it came from this module's own file, not the OS environment.
func (scope *Scope) SetIfDefault(key string, val value.Value, secret bool) {
	if scope.claimed(key) {
		return
	}
	scope.Set(key, val, secret)
	delete(scope.Ambient, key)
}

// claimed reports whether key holds a value some layer above the OS env
// produced — the one rule every default-tier write (CLI args, env files,
// vars:, a module's env:/vars:) checks before it writes.
func (scope *Scope) claimed(key string) bool {
	_, present := scope.Vars[key]
	return present && !scope.Ambient[key]
}

func (scope *Scope) Copy() *Scope {
	return &Scope{
		Vars:    eval.CloneMap(scope.Vars),
		Secrets: eval.CloneMap(scope.Secrets),
		Ambient: eval.CloneMap(scope.Ambient),
	}
}
