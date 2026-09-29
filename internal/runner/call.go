package runner

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/scope"
	"hobnob/internal/tui"
	"hobnob/internal/value"
)

// callMemo is the per-run cache backing once: true tasks. It lives for one
// ExecuteTask call (see runner.go) and is threaded through execCtx, including
// across a call:'s scope swap — that's what lets a shared prologue replay
// correctly into two sibling call: sandboxes rather than only working for the
// first one that reaches it.
type callMemo struct {
	// scopes holds the CHILD SCOPE each once: task's first run produced,
	// keyed by callCacheID — not a delta, the whole thing. Each later call:
	// site projects what it wants out of that cached scope through its own
	// into:, so two sites can pull different things from one cached run; a
	// delta would need to already guess what every future site wants.
	scopes map[uintptr]*scope.Scope
	// summaries holds a masked "KEY=val KEY2=val2" rendering of what each
	// once: task's first run actually produced or changed, computed once at
	// write time (before/after aren't available on a later cache hit) — the
	// cache-hit log line's only content.
	summaries map[uintptr]string
	// running guards against a once: task (directly or transitively) calling
	// itself before its first run has completed, which would otherwise
	// recurse forever since nothing is in scopes yet.
	running map[uintptr]bool
}

func newCallMemo() *callMemo {
	return &callMemo{
		scopes:    make(map[uintptr]*scope.Scope),
		summaries: make(map[uintptr]string),
		running:   make(map[uintptr]bool),
	}
}

// callCacheID identifies a task's physical identity for memoization purposes,
// not the name it was reached by: registerModuleTasks can register the same
// Task under several names (a module prefix, a flatten: true bare alias, the
// module's own bare name from inside the module file), and all of those
// registrations share one Steps backing array. Keying on that array's address
// means docker:_setup (called from the parent) and _setup (called from
// inside docker's own file) collapse to one cache entry — calling it from
// either place only ever prompts, or runs its run: steps, once.
func callCacheID(task config.Task) uintptr {
	return reflect.ValueOf(task.Steps).Pointer()
}

func execCall(execState execCtx, step config.Step, scope *scope.Scope) error {
	taskName, err := eval.EvalTemplate(step.CallTarget, scope.Vars())
	if err != nil {
		return fmt.Errorf("call target template: %w", err)
	}

	task, _, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}

	if !task.Once || len(task.Steps) == 0 {
		childScope, err := buildCallScope(scope, step.CallVars)
		if err != nil {
			return err
		}
		if err := executeTask(execState, taskName, step.DirTmpl, childScope); err != nil {
			return fmt.Errorf("call %s: %w", taskName, err)
		}
		return captureCallInto(step.IntoEntries, scope, childScope)
	}

	id := callCacheID(task)
	if cached, ok := execState.memo.scopes[id]; ok {
		fmt.Println(tui.CallCacheHitLine(execState.task, taskName, execState.memo.summaries[id]))
		return captureCallInto(step.IntoEntries, scope, cached)
	}
	if execState.memo.running[id] {
		return fmt.Errorf("call %q: cycle detected — task is already running", taskName)
	}

	childScope, err := buildCallScope(scope, step.CallVars)
	if err != nil {
		return err
	}
	before := childScope.Copy()
	execState.memo.running[id] = true
	err = executeTask(execState, taskName, step.DirTmpl, childScope)
	delete(execState.memo.running, id)
	if err != nil {
		return fmt.Errorf("call %s: %w", taskName, err)
	}

	execState.memo.scopes[id] = childScope
	execState.memo.summaries[id] = summarizeCallDelta(before, childScope)
	return captureCallInto(step.IntoEntries, scope, childScope)
}

// summarizeCallDelta renders the vars a once: task's first run produced or
// changed, relative to before it ran, as a masked "KEY=val KEY2=val2"
// summary for the cache-hit log line — otherwise a replayed call: is
// invisible, the exact complaint the old use: memo drew (see GUIDE.md's
// former "Sharp edge" note). Sorted by key for a stable line across runs.
func summarizeCallDelta(before, after *scope.Scope) string {
	var keys []string
	beforeVars := before.Vars()
	for key, val := range after.Vars() {
		if prior, existed := beforeVars[key]; !existed || !reflect.DeepEqual(prior, val) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + after.Display(key)
	}
	return strings.Join(parts, " ")
}

// buildCallScope deep-copies scope and evaluates with: entries into it. No
// secret flag is set here — config.rejectSecretCallVars rules secret: out of
// with: entirely, because Copy() already brought the parent's secrets across
// and masking matches on value, so a secret passed down stays masked under its
// new name without anything to declare at the call site.
func buildCallScope(scope *scope.Scope, callVars []config.SetEntry) (*scope.Scope, error) {
	childScope := scope.Copy()
	for _, callVar := range callVars {
		val, err := config.EvalSetEntry(callVar, func(tmpl string) (value.Value, error) {
			return eval.EvalValue(tmpl, childScope.Vars())
		})
		if err != nil {
			return nil, fmt.Errorf("call var %q: %w", callVar.Key, err)
		}
		childScope.Set(callVar.Key, val, false)
	}
	return childScope, nil
}

// captureCallInto pulls into: results back from childScope into the caller's
// scope. Each leaf is one of:
//   - an explicit {{ }} template, evaluated against the CALLER's own
//     (evolving) scope — so later into: entries in the same block can
//     reference earlier ones, e.g. ARCHIVE_PATH: "{{.LOG_PATH}}/archive"
//     where LOG_PATH was itself just set by a prior entry in this call.
//   - a bare key (with or without a leading "."), optionally followed by an
//     accessor ([0].name) and/or " | filter | filter" — read straight out
//     of childScope, typed, then resolved/filtered if there's an accessor
//     or chain.
func captureCallInto(entries []config.IntoEntry, scope, childScope *scope.Scope) error {
	evalLeaf := func(valueTmpl string) (value.Value, error) {
		if strings.Contains(valueTmpl, "{{") {
			return eval.EvalValue(valueTmpl, scope.Vars())
		}
		val, _, err := resolveChildRef(valueTmpl, childScope.Vars())
		return val, err
	}
	for _, intoEntry := range entries {
		parentKey := intoEntry.ParentKey

		if intoEntry.ValNode != nil {
			val, err := config.EvalJSONNode(*intoEntry.ValNode, evalLeaf)
			if err != nil {
				return fmt.Errorf("into value %q: %w", parentKey, err)
			}
			scope.Adopt(childScope, val)
			scope.Set(parentKey, val, false)
			continue
		}

		if strings.Contains(intoEntry.ValueTmpl, "{{") {
			val, err := evalLeaf(intoEntry.ValueTmpl)
			if err != nil {
				return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
			}
			scope.Set(parentKey, val, false)
			continue
		}

		val, plainKey, err := resolveChildRef(intoEntry.ValueTmpl, childScope.Vars())
		if err != nil {
			return fmt.Errorf("into value %q: %w", intoEntry.ValueTmpl, err)
		}
		scope.Adopt(childScope, val)
		scope.Set(parentKey, val, plainKey != "" && childScope.IsSecret(plainKey))
	}
	return nil
}

// resolveChildRef evaluates a bare into: leaf (no {{ }}) against childVars:
// "KEY", ".KEY", "KEY.field[0]", "KEY | trim", "KEY[0].name | upper". When
// the leaf is exactly one bare var name with no accessor or filter chain,
// plainKey returns that name so the caller can propagate its secret flag —
// every other shape loses the secret annotation, same as passing a value
// through any filter does.
func resolveChildRef(valueTmpl string, childVars map[string]value.Value) (val value.Value, plainKey string, err error) {
	key, chain, hasChain := strings.Cut(valueTmpl, " | ")
	key = strings.TrimPrefix(strings.TrimSpace(key), ".")
	if !hasChain && isPlainKey(key) {
		return childVars[key], key, nil
	}
	expr := "{{ ." + key
	if hasChain {
		expr += " | " + chain
	}
	val, err = eval.EvalValue(expr+" }}", childVars)
	return val, "", err
}

func isPlainKey(s string) bool {
	if s == "" {
		return false
	}
	return !strings.ContainsAny(s, ".[")
}
