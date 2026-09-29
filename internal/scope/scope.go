// Package scope holds hobnob's runtime variable scope and builds it: Load
// resolves the root taskfile's scope and every module's file scope, and
// FileScopes.Apply lays a module's file scope onto the scope one of its
// tasks runs with.
package scope

import (
	"encoding/json"
	"strings"

	"hobnob/internal/eval"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// Scope is the runtime variable scope. Its maps are private so every write
// goes through Set, SetIfDefault, MarkSecret or Bind, and none can skip the
// secret and ambient bookkeeping those carry.
type Scope struct {
	vars    map[string]value.Value
	secrets map[string]bool
	// ambient holds true for a key whose current value still comes only from
	// the OS-environment base layer — nothing higher in the chain (vars:,
	// env files, CLI args, const:, a module's file scope, a step) has
	// touched it since. Only Load's base layer ever marks a key ambient;
	// every write through Set clears it. This is what lets a module's own
	// env:/vars: block decide whether it's safe to supply a default for a
	// var it inherited, without being able to see which layer actually
	// produced that inherited value.
	ambient map[string]bool
	// carried holds secret values Adopt brought across from a child scope
	// without the child's name for them — a secret leaf of a call:'s into:
	// literal, say — keyed by their string form. Mask redacts them just like
	// a named secret's value.
	carried map[string]value.Value
}

// New returns an empty scope.
func New() *Scope {
	return &Scope{
		vars:    make(map[string]value.Value),
		secrets: make(map[string]bool),
		ambient: make(map[string]bool),
		carried: make(map[string]value.Value),
	}
}

// Vars returns the scope's variables for template evaluation. The map is the
// scope's own, not a copy: callers must treat it as read-only and write only
// through Scope's methods.
func (scope *Scope) Vars() map[string]value.Value {
	return scope.vars
}

// Set assigns val to key and, when secret is true, flags key as a secret —
// the "set a var and propagate its secret flag" idiom shared by every step
// kind and Load that can introduce a new scope var. The key is claimed from
// then on: whatever wrote it, it no longer comes from the OS env.
func (scope *Scope) Set(key string, val value.Value, secret bool) {
	scope.vars[key] = val
	if secret {
		scope.secrets[key] = true
	}
	delete(scope.ambient, key)
}

// SetIfDefault assigns val to key only when key is unclaimed (see claimed) —
// never overwriting a value some higher-priority layer already produced.
// Used for a module's own env:/vars: block, which supplies a default for its
// subtree rather than an override.
func (scope *Scope) SetIfDefault(key string, val value.Value, secret bool) {
	if scope.claimed(key) {
		return
	}
	scope.Set(key, val, secret)
}

// MarkSecret flags an existing key as a secret without changing its value —
// a get: with secret: true whose variable was already in scope.
func (scope *Scope) MarkSecret(key string) {
	scope.secrets[key] = true
}

// IsSecret reports whether key is flagged as a secret — what a call:'s
// plain-key into: carries across to the caller's name for it.
func (scope *Scope) IsSecret(key string) bool {
	return scope.secrets[key]
}

// Bind sets key to val for the duration of a loop: iteration, returning a
// restore func that puts back the key's prior value, secret flag and ambient
// flag — or removes it, if it wasn't in scope — so a loop leaves scope
// exactly as it found it.
func (scope *Scope) Bind(key string, val value.Value) (restore func()) {
	prev, had := scope.vars[key]
	wasSecret, wasAmbient := scope.secrets[key], scope.ambient[key]
	scope.Set(key, val, false)
	return func() {
		if had {
			scope.vars[key] = prev
		} else {
			delete(scope.vars, key)
		}
		if !wasSecret {
			delete(scope.secrets, key)
		}
		if wasAmbient {
			scope.ambient[key] = true
		}
	}
}

// claimed reports whether key holds a value some layer above the OS env
// produced — the one rule every default-tier write (CLI args, env files,
// vars:, a module's env:/vars:) checks before it writes.
func (scope *Scope) claimed(key string) bool {
	_, present := scope.vars[key]
	return present && !scope.ambient[key]
}

// Display renders key's value for terminal output: tui.SecretMask when key
// is a secret, its string form otherwise.
func (scope *Scope) Display(key string) string {
	if scope.secrets[key] {
		return tui.SecretMask
	}
	return scope.vars[key].String()
}

// Mask replaces each secret value in text — every secret variable's, and
// every one Adopt carried across — with tui.SecretMask. It
// matches both the raw value and its JSON-escaped form (quotes/backslashes/
// newlines escaped the way json.Marshal would render them) — a secret
// embedded as a leaf of a set:/into: JSON literal is marshaled, so its
// escaped form can differ from the raw value and would otherwise slip past a
// raw-only match.
//
// A Bool or short Number secret is skipped: masking "true" or "1" as a
// substring would blank out unrelated text (every "true" in the command),
// not just the secret.
func (scope *Scope) Mask(text string) string {
	for _, secretVal := range scope.secretValues() {
		text = maskValue(text, secretVal)
	}
	return text
}

// Adopt carries across every secret of from whose value appears in val, so
// a value a call:'s into: pulls out of a child scope stays masked in scope
// even when scope never had the child's name for the secret inside it.
func (scope *Scope) Adopt(from *Scope, val value.Value) {
	text := val.String()
	for _, secretVal := range from.secretValues() {
		if maskValue(text, secretVal) != text {
			scope.carried[secretVal.String()] = secretVal
		}
	}
}

// secretValues lists every value Mask redacts: each named secret's, then
// each carried one.
func (scope *Scope) secretValues() []value.Value {
	vals := make([]value.Value, 0, len(scope.secrets)+len(scope.carried))
	for name := range scope.secrets {
		vals = append(vals, scope.vars[name])
	}
	for _, secretVal := range scope.carried {
		vals = append(vals, secretVal)
	}
	return vals
}

// maskValue replaces secretVal's raw and JSON-escaped forms in text with
// tui.SecretMask, skipping a value too generic to mask safely.
func maskValue(text string, secretVal value.Value) string {
	if secretVal.Kind() == value.KindBool {
		return text
	}
	if secretVal.Kind() == value.KindNumber && len(secretVal.String()) < 4 {
		return text
	}
	raw := secretVal.String()
	if raw == "" {
		return text
	}
	if escaped, ok := jsonEscapedForm(raw); ok && escaped != raw {
		text = strings.ReplaceAll(text, escaped, tui.SecretMask)
	}
	return strings.ReplaceAll(text, raw, tui.SecretMask)
}

// jsonEscapedForm returns secretVal as it would appear inside a
// json.Marshal-ed string (unquoted) — the form a secret takes once it's a
// leaf of a set:/into: JSON literal.
func jsonEscapedForm(secretVal string) (string, bool) {
	jsonBytes, err := json.Marshal(secretVal)
	if err != nil || len(jsonBytes) < 2 {
		return "", false
	}
	return string(jsonBytes[1 : len(jsonBytes)-1]), true
}

func (scope *Scope) Copy() *Scope {
	return &Scope{
		vars:    eval.CloneMap(scope.vars),
		secrets: eval.CloneMap(scope.secrets),
		ambient: eval.CloneMap(scope.ambient),
		carried: eval.CloneMap(scope.carried),
	}
}
