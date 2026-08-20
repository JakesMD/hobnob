package e2e

import (
	"path/filepath"
	"testing"
)

// once: true memoizes a task: it runs at most once per hobnob invocation,
// keyed on the task's physical identity (not the name it was reached by via
// call:), and a later call: to it replays the scope its first run produced
// rather than re-executing — each call: site still projects what it wants
// out of that scope through its own into:, same as an unmemoized call:.
// Without once:, a task runs every time it's called, same as before this
// existed.

func TestE2E_Once_MemoizedSecondCallSkipsRerun(t *testing.T) {
	// given the same once: task called: twice, when the second call: is
	// reached, then its run: step does not execute again (why: once: means
	// "ensure this has run", not "run this again" — a shared prologue's side
	// effects, e.g. an API call, must happen once)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - call: setup
			      - call: setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    once: true
			    steps:
			      - run: printf x >> count.txt
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Once_ReplaysAcrossCallSandboxes(t *testing.T) {
	// given two sibling call:s that each call: the same once: prologue, when
	// both complete, then the prologue ran once and both children got its
	// result via their own into: (why: caching only the fact something ran
	// would leave the second sandbox with nothing — the memo must replay the
	// actual scope, and each into: projects from it independently)
	res := Yml(t, `
		tasks:
		  main:
		    steps:
		      - call: a
		        into:
		          - A_TOKEN: .TOKEN
		      - call: b
		        into:
		          - B_TOKEN: .TOKEN
		      - run: wc -c < count.txt
		        into:
		          - COUNT: stdout | trim
		      - run: echo a={{.A_TOKEN}} b={{.B_TOKEN}} count={{.COUNT}}
		  a:
		    steps:
		      - call: setup
		        into:
		          - TOKEN: .TOKEN
		  b:
		    steps:
		      - call: setup
		        into:
		          - TOKEN: .TOKEN
		  setup:
		    once: true
		    steps:
		      - run: printf x >> count.txt && wc -c < count.txt
		        into:
		          - TOKEN: stdout | trim
	`, "main")
	res.OK(t)
	res.Out(t, "a=1 b=1 count=1")
}

func TestE2E_Once_WithoutOnceRunsEveryLoopIteration(t *testing.T) {
	// given a call: step to a task with no once: inside a loop:, when it
	// runs, then it re-executes on every iteration — the default, unlike the
	// old use:/rerun: pairing this replaces (why: once: is now a property of
	// the callee, not something a call site opts into per-site)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - loop: [a, b, c]
			        steps:
			          - call: setup
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

func TestE2E_Once_TrueInLoopRunsOnlyOnce(t *testing.T) {
	// given a call: step to a once: true task inside a loop:, when it runs,
	// then it executes on the first iteration only, memoized on the rest
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - loop: [a, b, c]
			        steps:
			          - call: setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    once: true
			    steps:
			      - run: printf x >> count.txt
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Once_UseRejectedAtParseTime(t *testing.T) {
	// given a use: step, when parsed, then it's rejected naming call: and
	// once: as the replacement (why: use: was removed in favor of call: +
	// once: + into:)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - use: setup
		  setup:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "use:", "call:", "once:")
}

func TestE2E_Once_RerunRejectedAtParseTime(t *testing.T) {
	// given rerun: on a call: step, when parsed, then it's rejected naming
	// once: as the replacement (why: rerun: was removed along with use: —
	// whether a task memoizes is now the target task's own once:, not a flag
	// at the call site)
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
	res.Err(t, "rerun:", "once:")
}

func TestE2E_Once_TasksOwnIfFalseSkipsAndCaches(t *testing.T) {
	// given a once: task whose own if: is false, when call:ed twice, then
	// both calls are silent no-ops (why: a skip is authored and
	// deterministic — the first attempt gives the later caller what it got,
	// not a re-evaluated condition)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: setup
		        into:
		          - RAN: .RAN
		      - call: setup
		        into:
		          - RAN2: .RAN
		      - run: echo ran={{.RAN | default "no"}} ran2={{.RAN2 | default "no"}}
		  setup:
		    once: true
		    if: '[ 1 -eq 2 ]'
		    steps:
		      - set:
		          - RAN: yes
	`, "t")
	res.OK(t)
	res.Lines(t, "ran=no ran2=no")
}

func TestE2E_Once_StepIfFalseDoesNotCache(t *testing.T) {
	// given a call: step itself skipped by a false if:, when a later plain
	// call: to the same once: task is reached, then it still runs (why: a
	// step skipped by its own if: never even reaches execCall — it asserts
	// nothing about the task, and must not be confused with the task having
	// already run)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: setup
		        if: '[ 1 -eq 2 ]'
		        into:
		          - RAN: .RAN
		      - call: setup
		        into:
		          - RAN2: .RAN
		      - run: echo ran={{.RAN | default "no"}} ran2={{.RAN2 | default "no"}}
		  setup:
		    once: true
		    steps:
		      - set:
		          - RAN: yes
	`, "t")
	res.OK(t)
	res.Lines(t, "ran=no ran2=yes")
}

func TestE2E_Once_DirDoesNotLeakToCaller(t *testing.T) {
	// given a once: task with its own dir:, when the caller's later steps
	// run, then they are unaffected by it — same dir: scoping an unmemoized
	// call: already has, still true through the cache path
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - call: setup
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
				  setup:
				    once: true
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

func TestE2E_Once_SoftContinuesPastFailureWithoutCaching(t *testing.T) {
	// given soft: true on a failing call: step to a once: task, called
	// twice, when both run, then both attempts retry from scratch (why: a
	// transient failure is neither a completion nor a deterministic skip —
	// caching a half-finished scope would silently hand a later caller a
	// broken result, so nothing is cached on a soft-swallowed failure)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - call: setup
			        soft: true
			      - call: setup
			        soft: true
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
			  setup:
			    once: true
			    steps:
			      - run: printf x >> count.txt
			      - run: exit 1
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "count=2")
}

func TestE2E_Once_ModuleTaskIdentitySharesMemoAcrossQualifiedAndBareNames(t *testing.T) {
	// given the same once: module task called: once by its parent-qualified
	// name and once by its bare name from inside the module's own file, when
	// both run, then they share one memo entry (why: the cache key is the
	// task's physical identity, not the name it was reached by — a shared
	// module prologue must run once regardless of which caller reaches it
	// first)
	files := Files{
		"hobnob.yml": `
			modules:
			  - mod: mod.yml
			tasks:
			  t:
			    steps:
			      - call: mod:run_a
			      - call: mod:setup
			      - run: wc -c < count.txt
			        into:
			          - COUNT: stdout | trim
			      - run: echo count={{.COUNT}}
		`,
		"mod.yml": `
			tasks:
			  run_a:
			    steps:
			      - call: setup
			  setup:
			    once: true
			    steps:
			      - run: printf x >> count.txt
		`,
	}
	res := Run(t, Case{Files: files, Args: []string{"t"}})
	res.OK(t)
	res.Out(t, "count=1")
}

func TestE2E_Once_ReplayOverwritesLaterSetWhenIntoNamesIt(t *testing.T) {
	// given call:/set:/call: on the same var, with into: naming that var at
	// both call: sites, when the second call: replays, then it overwrites
	// the set: in between (why: once: asserts a task's results are in scope
	// — it is not "maybe run something"; unlike the old use: version of this
	// sharp edge, the overwrite is now visible at the call site that asks
	// for it, via into:, rather than happening silently to the whole scope)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: setup
		        into:
		          - TOKEN: .TOKEN
		      - set:
		          - TOKEN: xyz
		      - call: setup
		        into:
		          - TOKEN: .TOKEN
		      - run: echo token={{.TOKEN}}
		  setup:
		    once: true
		    steps:
		      - set:
		          - TOKEN: abc
	`, "t")
	res.OK(t)
	res.Lines(t, "token=abc")
}

func TestE2E_Once_SelfCycleErrors(t *testing.T) {
	// given a once: task that calls: itself, when run, then it fails naming
	// the cycle rather than recursing forever (why: the first attempt hasn't
	// finished — and so hasn't reached the cache — when the nested call:
	// tries to run it again)
	res := Yml(t, `
		tasks:
		  t:
		    once: true
		    steps:
		      - call: t
	`, "t")
	res.Fails(t)
	res.Err(t, "cycle")
}

func TestE2E_Once_CacheHitIsLoggedWithProducedVars(t *testing.T) {
	// given a once: task called: twice, when the second call: hits the
	// cache, then the run announces it, naming the vars the first run
	// produced (why: otherwise a memoized call: is silent — no prompt, no
	// command output — and there'd be no way to tell it happened short of a
	// debugger)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - call: setup
			      - call: setup
			  setup:
			    once: true
			    steps:
			      - set:
			          - ACCOUNT: "123456789"
		`,
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "cached", "ACCOUNT=123456789")
}
