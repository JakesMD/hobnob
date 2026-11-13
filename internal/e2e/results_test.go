package e2e

import "testing"

func TestE2E_Results_BareCallPullsResults(t *testing.T) {
	// given a prompt task declaring results: [DISK], when called with no
	// into:, then DISK lands in the caller (why: results: is what a bare
	// call hands back, so a prompt task needs no into: at every call site)
	res := Run(t, Case{
		Yml: `
			tasks:
			  flash:
			    steps:
			      - call: _pick-disk
			      - run: echo disk={{.DISK}}
			  _pick-disk:
			    results: [DISK]
			    steps:
			      - get:
			          - DISK:
			              options: [/dev/disk4, /dev/disk5]
		`,
		Args:    []string{"flash"},
		Answers: map[string][]string{"DISK": {"/dev/disk5"}},
		TTY:     true,
	})
	res.OK(t)
	res.Lines(t, "disk=/dev/disk5")
	res.Prompted(t, "DISK")
}

func TestE2E_Results_BareCallPullsOnlyResults(t *testing.T) {
	// given a task with two results and a non-result var, when called bare,
	// then exactly the results come back and the rest stays sandboxed (why:
	// results: is the whole contract, not a hint to copy the whole scope)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		      - run: echo a={{.A}} b={{.B}} scratch={{.SCRATCH | default "no"}}
		  child:
		    results: [A, B]
		    steps:
		      - set:
		          - A: one
		          - B: two
		          - SCRATCH: leaked
	`, "parent")
	res.OK(t)
	res.Lines(t, "a=one b=two scratch=no")
}

func TestE2E_Results_IntoReplacesResults(t *testing.T) {
	// given a call with its own into:, when it completes, then only what
	// into: lists comes back, not the task's results (why: into: is the
	// call site choosing exactly what it takes, so it can avoid a clobber)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - A: mine
		      - call: child
		        into: [B]
		      - run: echo a={{.A}} b={{.B}}
		  child:
		    results: [A]
		    steps:
		      - set:
		          - A: theirs
		          - B: extra
	`, "parent")
	res.OK(t)
	res.Lines(t, "a=mine b=extra")
}

func TestE2E_Results_IntoMayRepeatResults(t *testing.T) {
	// given an into: that lists the task's results again, when the call
	// completes, then it works the same as a bare call (why: explicit
	// is allowed, just not required)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        into: [A]
		      - run: echo a={{.A}}
		  child:
		    results: [A]
		    steps:
		      - set:
		          - A: one
	`, "parent")
	res.OK(t)
	res.Lines(t, "a=one")
}

func TestE2E_Results_InheritedValueKeepsThePromise(t *testing.T) {
	// given DISK passed on the command line, when a bare call reaches a
	// prompt task whose get: is skipped because DISK is already set, then
	// the call succeeds with that value (why: results: promises DISK exists
	// afterwards, however it got there)
	res := Run(t, Case{
		Yml: `
			tasks:
			  flash:
			    steps:
			      - call: _pick-disk
			      - run: echo disk={{.DISK}}
			  _pick-disk:
			    results: [DISK]
			    steps:
			      - get: [DISK]
		`,
		Args: []string{"flash", "DISK=/dev/disk9"},
		TTY:  true,
	})
	res.OK(t)
	res.Lines(t, "disk=/dev/disk9")
	res.Prompted(t)
}

func TestE2E_Results_SkippedTaskWithInheritedValueKeepsThePromise(t *testing.T) {
	// given a caller that already holds DISK, when a bare call reaches a
	// task skipped by its own if:, then the call succeeds and DISK is
	// unchanged (why: the inherited value keeps the promise)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - DISK: /dev/disk2
		      - call: child
		      - run: echo disk={{.DISK}}
		  child:
		    if: "false"
		    results: [DISK]
		    steps:
		      - set:
		          - DISK: /dev/never
	`, "parent")
	res.OK(t)
	res.Lines(t, "disk=/dev/disk2")
}

func TestE2E_Results_SkippedTaskBreaksThePromise(t *testing.T) {
	// given a task skipped by its own if: with no DISK in scope, when called
	// bare, then the run fails naming the missing result (why: a skip doesn't
	// excuse the promise, or a caller would read a var that isn't there)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		  child:
		    if: "false"
		    results: [DISK]
		    steps:
		      - set:
		          - DISK: /dev/never
	`, "parent")
	res.Fails(t)
	res.Err(t, "child", "DISK", "not set")
}

func TestE2E_Results_UnsetResultBreaksThePromise(t *testing.T) {
	// given a task that runs but never sets a declared result, when called,
	// then the run fails naming it (why: results: is the task's contract,
	// so a typo or a missing step is caught at the task, not downstream)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		  child:
		    results: [DISK]
		    steps:
		      - set:
		          - DSIK: /dev/disk4
	`, "parent")
	res.Fails(t)
	res.Err(t, "child", "DISK", "not set")
}

func TestE2E_Results_PromiseCheckedEvenWithInto(t *testing.T) {
	// given a call with its own into: and | default, when the task breaks
	// its results: promise, then the run still fails (why: the promise
	// belongs to the task, whatever the call site does)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        into:
		          - D: .DISK | default "none"
		  child:
		    results: [DISK]
		    steps:
		      - set:
		          - OTHER: x
	`, "parent")
	res.Fails(t)
	res.Err(t, "child", "DISK", "not set")
}

func TestE2E_Results_OnceReplayPullsResults(t *testing.T) {
	// given a once: prompt task with results:, when two tasks call it bare,
	// then it prompts once and both get the answer (why: a replay pulls
	// results from the cached scope exactly as the first call did)
	res := Run(t, Case{
		Yml: `
			tasks:
			  all:
			    steps:
			      - call: flash
			      - call: verify
			  flash:
			    steps:
			      - call: _pick-disk
			      - run: echo flash={{.DISK}}
			  verify:
			    steps:
			      - call: _pick-disk
			      - run: echo verify={{.DISK}}
			  _pick-disk:
			    once: true
			    results: [DISK]
			    steps:
			      - get: [DISK]
		`,
		Args:    []string{"all"},
		Answers: map[string][]string{"DISK": {"/dev/disk4"}},
		TTY:     true,
	})
	res.OK(t)
	res.Out(t, "flash=/dev/disk4", "verify=/dev/disk4")
	res.Prompted(t, "DISK")
}

func TestE2E_Results_SecretResultStaysMasked(t *testing.T) {
	// given a secret result, when pulled by a bare call, then the caller
	// still masks it (why: a bare call goes through the same capture path as
	// into:, which adopts the child's secrets)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		      - run: ": {{.TOKEN}}"
		  child:
		    results: [TOKEN]
		    steps:
		      - set:
		          - TOKEN:
		              value: hunter2
		              secret: true
	`, "parent")
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Results_PrivateNameRejectedAtParseTime(t *testing.T) {
	// given results: listing a _-prefixed name, when parsed, then it's
	// rejected (why: a private variable can never leave its task)
	res := Yml(t, `
		tasks:
		  t:
		    results: [_RAW]
		    steps:
		      - set:
		          - _RAW: x
	`, "t")
	res.Fails(t)
	res.Err(t, "results:", "_RAW", "private")
}

func TestE2E_Results_DuplicateNameRejectedAtParseTime(t *testing.T) {
	// given results: listing a name twice, when parsed, then it's rejected
	// (why: a duplicate is always a typo, and caught cheapest at load)
	res := Yml(t, `
		tasks:
		  t:
		    results: [A, A]
		    steps:
		      - set:
		          - A: x
	`, "t")
	res.Fails(t)
	res.Err(t, "results:", `"A"`, "twice")
}

func TestE2E_Results_NonNameRejectedAtParseTime(t *testing.T) {
	// given results: holding something other than plain names, when parsed,
	// then it's rejected (why: reshaping belongs to into:, not results:)
	res := Yml(t, `
		tasks:
		  t:
		    results:
		      - DEV: .DISK.path
		    steps:
		      - set:
		          - DEV: x
	`, "t")
	res.Fails(t)
	res.Err(t, "results:", "variable names")
}

func TestE2E_Results_BareCallClashingWithConstRejected(t *testing.T) {
	// given a file whose const: declares DISK, when one of its tasks calls a
	// task with results: [DISK] bare, then loading fails (why: a bare call
	// sets its results in the caller, and nothing may set a const: name)
	res := Yml(t, `
		const:
		  - DISK: /dev/disk1
		tasks:
		  parent:
		    steps:
		      - call: child
		  child:
		    results: [DISK]
		    steps:
		      - run: echo hi
	`, "parent")
	res.Fails(t)
	res.Err(t, "parent", "DISK", "const:")
}

func TestE2E_Results_ModuleResultClashingWithCallerConstFails(t *testing.T) {
	// given a file whose const: declares K, when one of its tasks calls a
	// module task with results: [K] bare, then the call fails rather than
	// overwrite K (why: the load-time check can't see a module's
	// results, and nothing outside a file may override its const:)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				const:
				  - K: parent
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:getk
			`,
			"mod.yml": `
				tasks:
				  getk:
				    results: [K]
				    steps:
				      - set:
				          - K: module
			`,
		},
		Args: []string{"t"},
	})
	res.Fails(t)
	res.Err(t, `"K"`, "const:")
}

func TestE2E_Results_TemplatedTargetClashingWithConstFails(t *testing.T) {
	// given a templated call: target whose task has results: [K], when the
	// caller's file declares const: K, then the call fails at run time
	// (why: a templated target isn't known at load time)
	res := Yml(t, `
		const:
		  - K: fixed
		tasks:
		  t:
		    steps:
		      - set:
		          - TARGET: child
		      - call: "{{.TARGET}}"
		  child:
		    results: [K]
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, `"K"`, "const:")
}

func TestE2E_Results_EmptyIntoTakesNothing(t *testing.T) {
	// given a call with into: [], when the task has results, then none come
	// back (why: into: replaces results, so an empty into: is how a call
	// site opts out of every one)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - DISK: mine
		      - call: child
		        into: []
		      - run: echo disk={{.DISK}}
		  child:
		    results: [DISK]
		    steps:
		      - set:
		          - DISK: theirs
	`, "parent")
	res.OK(t)
	res.Lines(t, "disk=mine")
}

func TestE2E_Results_BareCallClashingWithConstFailsBeforeAnyStepRuns(t *testing.T) {
	// given a bare call to a same-file task whose results: name a const:,
	// when the task runs, then loading fails before its first step (why: a
	// literal same-file target's results are known at load, so the clash
	// is a file error, not something a run half-completes before finding)

	// Arrange
	yml := `
		const:
		  - DISK: /dev/disk1
		tasks:
		  parent:
		    steps:
		      - run: echo started
		      - call: child
		  child:
		    results: [DISK]
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "parent")

	// Assert
	res.Fails(t)
	res.Err(t, "task parent sets DISK, declared in const:")
	res.NotOut(t, "started")
}

func TestE2E_Results_EmptyIntoLoadsDespiteConstResult(t *testing.T) {
	// given a task whose results: name a const: of the file, when a call
	// takes into: [], then the file loads and the const: keeps its value
	// (why: into: [] takes nothing, so the call writes no const: name, and
	// the promise holds because the child inherits the const: from its caller)

	// Arrange
	yml := `
		const:
		  - X: fixed
		tasks:
		  pick:
		    results: [X]
		    steps:
		      - set:
		          - Y: chosen
		  default:
		    steps:
		      - call: pick
		        into: []
		      - run: echo $X
	`

	// Act
	res := Yml(t, yml, "default")

	// Assert
	res.OK(t)
	res.Lines(t, "fixed")
}
