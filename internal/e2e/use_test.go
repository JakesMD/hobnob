package e2e

import (
	"path/filepath"
	"testing"
)

// use: runs another task's steps directly against the caller's scope — no
// Scope.Copy(), no with:/into:. It is memoized: a task runs at most once per
// hobnob invocation, keyed on the task's physical identity (not the name it
// was reached by), and a later use: replays the variables the first run
// produced rather than re-executing. rerun: true opts a single use: step out
// of that.

func TestE2E_Use_SharesCallerScope(t *testing.T) {
	// given a use: step, when the used task sets a var, then the caller's
	// later steps see it directly (why: use: shares scope — no sandbox, no
	// into: needed, unlike call:)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		      - run: echo host={{.HOST}}
		  setup:
		    steps:
		      - set:
		          - HOST: localhost
	`, "t")
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Use_MemoizedSecondUseSkipsRerun(t *testing.T) {
	// given the same task used: twice, when the second use: is reached, then
	// its run: step does not execute again (why: use: means "ensure this has
	// run", not "run this again" — a shared prologue's side effects, e.g. an
	// API call, must happen once)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - use: setup
			      - use: setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    steps:
			      - run: printf x >> count.txt
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Use_ReplaysAcrossCallSandboxes(t *testing.T) {
	// given two sibling call:s that each use: the same prologue, when both
	// complete, then the prologue ran once and both children got its result
	// (why: caching only the fact something ran would leave the second
	// sandbox with nothing — the memo must replay the actual snapshot)
	res := Yml(t, `
		tasks:
		  main:
		    steps:
		      - call: a
		        into:
		          - A_TOKEN: TOKEN
		      - call: b
		        into:
		          - B_TOKEN: TOKEN
		      - run: wc -c < count.txt
		        into:
		          - COUNT: stdout | trim
		      - run: echo a={{.A_TOKEN}} b={{.B_TOKEN}} count={{.COUNT}}
		  a:
		    steps:
		      - use: setup
		  b:
		    steps:
		      - use: setup
		  setup:
		    steps:
		      - run: printf x >> count.txt && wc -c < count.txt
		        into:
		          - TOKEN: stdout | trim
	`, "main")
	res.OK(t)
	res.Out(t, "a=1 b=1 count=1")
}

func TestE2E_Use_RerunTrueInLoopRunsEveryIteration(t *testing.T) {
	// given rerun: true on a use: step inside a loop:, when it runs, then it
	// re-executes on every iteration (why: without rerun:, a loop body's
	// use: would run on the first iteration only and silently do nothing on
	// the rest)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - loop: [a, b, c]
			        steps:
			          - use: setup
			            rerun: true
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    steps:
			      - run: printf x >> count.txt
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=3")
}

func TestE2E_Use_WithoutRerunInLoopRunsOnlyOnce(t *testing.T) {
	// given a use: step inside a loop: with no rerun:, when it runs, then it
	// executes on the first iteration only, memoized on the rest
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - loop: [a, b, c]
			        steps:
			          - use: setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    steps:
			      - run: printf x >> count.txt
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Use_WithRejectedAtParseTime(t *testing.T) {
	// given a use: step carrying with:, when parsed, then it's rejected
	// (why: use: shares the caller's scope — there is nothing to pass in;
	// call: is the tool for an isolated task)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		        with:
		          - X: 1
		  setup:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "with")
}

func TestE2E_Use_IntoRejectedAtParseTime(t *testing.T) {
	// given a use: step carrying into:, when parsed, then it's rejected
	// (why: use:'s results are already in the caller's scope — into: would
	// be a no-op at best, misleading at worst)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		        into:
		          - X: Y
		  setup:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "into")
}

func TestE2E_Use_RerunRejectedOnCallStep(t *testing.T) {
	// given rerun: on a call: step, when parsed, then it's rejected (why:
	// rerun: only ever means something on use: — call: always re-executes,
	// so a silently-ignored rerun: there would read as working when it does
	// nothing)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: child
		        rerun: true
		  child:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "rerun")
}

func TestE2E_Use_UsedTasksOwnIfFalseSkipsAndCaches(t *testing.T) {
	// given a used task whose own if: is false, when used: twice, then both
	// uses are silent no-ops (why: a skip is authored and deterministic —
	// caching it as "nothing" is what DESIGN-USE.md calls the first attempt
	// giving the later caller what it got, not re-evaluating the condition)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		      - use: setup
		      - run: echo ran={{.RAN | default "no"}}
		  setup:
		    if: '[ 1 -eq 2 ]'
		    steps:
		      - set:
		          - RAN: yes
	`, "t")
	res.OK(t)
	res.Lines(t, "ran=no")
}

func TestE2E_Use_StepIfFalseDoesNotCache(t *testing.T) {
	// given a use: step itself skipped by a false if:, when a later plain
	// use: of the same task is reached, then it still runs (why: a step
	// skipped by its own if: asserted nothing about the task — it must not
	// be confused with the task having already run)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		        if: '[ 1 -eq 2 ]'
		      - use: setup
		      - run: echo ran={{.RAN | default "no"}}
		  setup:
		    steps:
		      - set:
		          - RAN: yes
	`, "t")
	res.OK(t)
	res.Lines(t, "ran=yes")
}

func TestE2E_Use_DirDoesNotLeakToCaller(t *testing.T) {
	// given a used task with its own dir:, when the caller's later steps
	// run, then they are unaffected by it (why: unlike a call: step's own
	// dir:, the used task's dir: applies only to its own run: steps)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - use: setup
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
				  setup:
				    dir: sub
				    steps:
				      - run: pwd
			`,
			"sub/.keep": "",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	want := res.Dir
	res.Lines(t, filepath.Join(want, "sub"), want, "pwd="+want)
}

func TestE2E_Use_StepDirOverridesUsedTasksOwnDir(t *testing.T) {
	// given a use: step with its own dir: and the used task also declaring
	// one, when it runs, then the step's dir: wins — the same Priority-A
	// chain as call:
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - use: setup
				        dir: other
				  setup:
				    dir: sub
				    steps:
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"other/.keep": "",
			"sub/.keep":   "",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "other")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Use_SoftContinuesPastFailureWithoutCaching(t *testing.T) {
	// given soft: true on a failing use: step used twice, when both run,
	// then both attempts retry from scratch (why: a transient failure is
	// neither a completion nor a deterministic skip — a half-written
	// snapshot would silently hand a later caller half a scope, so nothing
	// is cached)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - use: setup
			        soft: true
			      - use: setup
			        soft: true
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    steps:
			      - run: printf x >> count.txt
			      - run: exit 1
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=2")
}

func TestE2E_Use_ModuleTaskIdentitySharesMemoAcrossQualifiedAndBareNames(t *testing.T) {
	// given the same module task used: once by its parent-qualified name and
	// once by its bare name from inside the module's own file, when both
	// run, then they share one memo entry (why: the cache key is the task's
	// physical identity, not the name it was reached by — a shared module
	// prologue must run once regardless of which caller reaches it first)
	files := Files{
		"hobnob.yml": `
			modules:
			  - mod: mod.yml
			tasks:
			  t:
			    steps:
			      - call: mod:run_a
			      - use: mod:setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
		`,
		"mod.yml": `
			tasks:
			  run_a:
			    steps:
			      - use: setup
			  setup:
			    steps:
			      - run: printf x >> count.txt
		`,
	}
	res := Run(t, Case{Files: files, Args: []string{"t"}})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Use_TaskInInternalModuleIsUsable(t *testing.T) {
	// given a task in a `_`-prefixed (internal) module, when use:d from the
	// parent, then it runs (why: use: follows the same module visibility
	// rule as call: — internal just means hidden from --list)
	files := Files{
		"hobnob.yml": `
			modules:
			  - _infra: infra.yml
			tasks:
			  t:
			    steps:
			      - use: _infra:prep
			      - run: echo ready={{.READY}}
		`,
		"infra.yml": `
			tasks:
			  prep:
			    steps:
			      - set:
			          - READY: yes
		`,
	}
	res := Run(t, Case{Files: files, Args: []string{"t"}})
	res.OK(t)
	res.Lines(t, "ready=yes")
}

func TestE2E_Use_ReplayOverwritesLaterSet(t *testing.T) {
	// given use:/set:/use: on the same var, when the second use: replays,
	// then it overwrites the set: in between (why: use: asserts a task's
	// results are in scope — it is not "maybe run something"; this is the
	// documented sharp edge, not a bug)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		      - set:
		          - TOKEN: xyz
		      - use: setup
		      - run: echo token={{.TOKEN}}
		  setup:
		    steps:
		      - set:
		          - TOKEN: abc
	`, "t")
	res.OK(t)
	res.Lines(t, "token=abc")
}

func TestE2E_Use_SelfCycleErrors(t *testing.T) {
	// given a task that use:s itself, when run, then it fails naming the
	// cycle rather than recursing forever (why: the first attempt hasn't
	// finished — and so hasn't reached the cache — when the nested use:
	// tries to run it again)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: t
	`, "t")
	res.Fails(t)
	res.Err(t, "cycle")
}
