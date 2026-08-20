package runner

import (
	"fmt"
	"reflect"

	"hobnob/internal/cli"
	"hobnob/internal/config"
	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// useMemo is the per-run cache backing use:. It lives for one ExecuteTask
// call (see runner.go) and is threaded through execCtx, including across a
// call:'s scope swap — that's what lets a shared prologue replay correctly
// into two sibling call: sandboxes rather than only working for the first one
// that reaches it.
type useMemo struct {
	// snapshots holds the delta each cache id produced on its first run —
	// only the vars that are new or changed, not the whole scope, so replaying
	// one use: can never clobber a caller's unrelated variables.
	snapshots map[uintptr]useSnapshot
	// running guards against a use: cycle: a task that (directly or
	// transitively) uses itself before its first run has completed would
	// recurse forever, since nothing is in snapshots yet.
	running map[uintptr]bool
}

type useSnapshot struct {
	vars    map[string]value.Value
	secrets map[string]bool
}

func newUseMemo() *useMemo {
	return &useMemo{snapshots: make(map[uintptr]useSnapshot), running: make(map[uintptr]bool)}
}

// useCacheID identifies a task's physical identity for memoization purposes,
// not the name it was reached by: registerModuleTasks can register the same
// Task under several names (a module prefix, a flatten: true bare alias, the
// module's own bare name from inside the module file), and all of those
// registrations share one Steps backing array. Keying on that array's address
// means docker:_setup (called from the parent) and _setup (called from
// inside docker's own file) collapse to one cache entry — using it from
// either place only ever prompts, or runs its run: steps, once.
func useCacheID(task config.Task) uintptr {
	return reflect.ValueOf(task.Steps).Pointer()
}

func execUse(execState execCtx, step config.Step, scope *cli.Scope) error {
	noPrompts := execState.noPrompts
	if step.Interactive != nil && !*step.Interactive {
		noPrompts = true
	}

	taskName, err := eval.EvalTemplate(step.CallTarget, scope.Vars)
	if err != nil {
		return fmt.Errorf("use target template: %w", err)
	}

	task, _, err := resolveTask(taskName, execState.cfg)
	if err != nil {
		return err
	}

	if step.Rerun || len(task.Steps) == 0 {
		return runTaskSteps(execState, taskName, step.DirTmpl, noPrompts, scope)
	}

	id := useCacheID(task)
	if snapshot, ok := execState.memo.snapshots[id]; ok {
		applyUseSnapshot(scope, snapshot)
		return nil
	}
	if execState.memo.running[id] {
		return fmt.Errorf("use %q: cycle detected — task is already running", taskName)
	}

	before := scope.Copy()
	execState.memo.running[id] = true
	err = runTaskSteps(execState, taskName, step.DirTmpl, noPrompts, scope)
	delete(execState.memo.running, id)
	if err != nil {
		return fmt.Errorf("use %s: %w", taskName, err)
	}

	execState.memo.snapshots[id] = diffUseSnapshot(before, scope)
	return nil
}

// diffUseSnapshot captures only what changed in scope relative to before —
// keys that are new, or whose value differs. Caching the whole scope would
// let a later, unrelated use: caller's variables be clobbered by the first
// caller's snapshot; capturing only the delta means replay only ever writes
// what the used task actually asserts.
func diffUseSnapshot(before, after *cli.Scope) useSnapshot {
	snapshot := useSnapshot{vars: make(map[string]value.Value), secrets: make(map[string]bool)}
	for key, val := range after.Vars {
		if prior, existed := before.Vars[key]; !existed || !reflect.DeepEqual(prior, val) {
			snapshot.vars[key] = val
			if after.Secrets[key] {
				snapshot.secrets[key] = true
			}
		}
	}
	return snapshot
}

func applyUseSnapshot(scope *cli.Scope, snapshot useSnapshot) {
	for key, val := range snapshot.vars {
		scope.Set(key, val, snapshot.secrets[key])
	}
}
