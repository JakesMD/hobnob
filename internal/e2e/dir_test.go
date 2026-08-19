package e2e

import (
	"path/filepath"
	"testing"
)

func TestE2E_Dir_TaskLevelSetsRunCwd(t *testing.T) {
	// given a task-level dir:, when a run: step has no dir: of its own, then
	// it executes in the task's dir
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    dir: sub
				    steps:
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"sub/.keep": "", // dir: must exist on disk before sh -c can chdir into it
		},
		Args: []string{"t"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "sub")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_AbsolutePathUsedVerbatim(t *testing.T) {
	// given a step's dir: is an absolute path, when it runs, then that exact
	// path is used rather than being resolved against the taskfile dir
	absDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - run: pwd
				        dir: "` + absDir + `"
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
		},
		Args: []string{"t"},
	})
	res.OK(t)
	want := absDir
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_StepLevelOverridesTaskLevel(t *testing.T) {
	// given both a task-level dir: and a step-level dir:, when that step
	// runs, then the step's own dir: wins for that one step only
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    dir: task-dir
				    steps:
				      - run: pwd
				        dir: step-dir
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"step-dir/.keep": "",
			"task-dir/.keep": "", // the 2nd run: step has no dir: of its own, so it inherits task-dir
		},
		Args: []string{"t"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "step-dir")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_RelativePathResolvesAgainstTaskfileDir(t *testing.T) {
	// given a call chain where an intermediate call: sets an inherited dir,
	// when a step several levels down declares a relative dir:, then it
	// resolves against the taskfile's own directory, not the inherited
	// caller dir (why: every relative dir: anchors to one fixed point, the
	// taskfile location, regardless of how deep the call chain is)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  grandparent:
				    steps:
				      - call: parent
				        dir: elsewhere
				  parent:
				    steps:
				      - run: pwd
				        dir: sub
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"sub/.keep":       "",
			"elsewhere/.keep": "", // the 2nd run: step has no dir: of its own, so it inherits "elsewhere"
		},
		Args: []string{"grandparent"},
	})
	res.OK(t)
	// "sub" must resolve against the taskfile dir (res.Dir), not against
	// "elsewhere" — the inherited dir grandparent's call: set for parent.
	want := filepath.Join(res.Dir, "sub")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_CallStepDirOverridesCalledTasksOwnDir(t *testing.T) {
	// given a call: step with its own dir: and the called task also
	// declaring a dir:, when the call runs, then the call site's dir: wins
	// (why: the caller's override is more specific than the callee's default)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  parent:
				    steps:
				      - call: child
				        dir: call-site-dir
				  child:
				    dir: task-own-dir
				    steps:
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"call-site-dir/.keep": "",
		},
		Args: []string{"parent"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "call-site-dir")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_CalledTaskInheritsCallerDirWhenNeitherOverrides(t *testing.T) {
	// given a call: step with no dir: and a called task with no dir: either,
	// when the call runs, then the callee inherits whatever dir: the caller
	// was already running in
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  parent:
				    dir: parent-dir
				    steps:
				      - call: child
				  child:
				    steps:
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"parent-dir/.keep": "",
		},
		Args: []string{"parent"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "parent-dir")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Dir_DisplayLineShowsRelativeDirWhenOverridden(t *testing.T) {
	// given a run: step with a dir: override under the taskfile dir, when
	// its command is echoed, then the "run:" chrome line notes the dir
	// relative to the invocation dir (why: the user should see at a glance
	// which steps ran somewhere other than the default)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - run: echo hi
				        dir: sub
			`,
			"sub/.keep": "",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Out(t, "(dir: ./sub)")
}
