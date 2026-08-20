package e2e

import "testing"

func TestE2E_Modules_InternalModuleHiddenButCallable(t *testing.T) {
	// given a module imported with a "_" prefix, when its tasks are used,
	// then they're hidden from --list but still callable via call: from a
	// parent task
	files := Files{
		"hobnob.yml": `
			modules:
			  - _farm: farm.yml
			tasks:
			  deploy:
			    steps:
			      - call: _farm:milk_cow
		`,
		"farm.yml": `
			tasks:
			  milk_cow:
			    steps:
			      - run: echo milked
		`,
	}
	list := Run(t, Case{Files: files, Args: []string{"--list"}})
	list.OK(t)
	list.NotOut(t, "_farm:milk_cow")

	run := Run(t, Case{Files: files, Args: []string{"deploy"}})
	run.OK(t)
	run.Lines(t, "milked")
}

func TestE2E_Modules_TaskWithUnderscorePrefixStaysPrivateToItsOwnModule(t *testing.T) {
	// given a public module whose own task has a "_" prefix, when a parent
	// task tries to call it, then the call fails — a module's own internal
	// tasks aren't registered in the parent at all, unlike a "_"-module's
	// tasks which are registered (just hidden)
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			tasks:
			  parent:
			    steps:
			      - call: yard:_weed
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - run: echo cleaned
			  _weed:
			    steps:
			      - run: echo weeded
		`,
	}
	res := Run(t, Case{Files: files, Args: []string{"parent"}})
	res.Fails(t)

	clean := Run(t, Case{Files: files, Args: []string{"yard:clean"}})
	clean.OK(t)
	clean.Lines(t, "cleaned")
}

func TestE2E_Modules_ShowFilterWhitelists(t *testing.T) {
	// given a module imported with show: [clean, fix], when die (not in the
	// list) is called, then it fails as unregistered — only the shown tasks
	// exist in the parent at all
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			    show: [clean, fix]
			tasks: {}
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - run: echo cleaned
			  fix:
			    steps:
			      - run: echo fixed
			  die:
			    steps:
			      - run: echo died
		`,
	}
	clean := Run(t, Case{Files: files, Args: []string{"yard:clean"}})
	clean.OK(t)

	die := Run(t, Case{Files: files, Args: []string{"yard:die"}})
	die.Fails(t)
	die.Err(t, `"yard:die" not found`)
}

func TestE2E_Modules_HideFilterBlacklists(t *testing.T) {
	// given a module imported with hide: [die], when die is called, then it
	// fails as unregistered, while everything else imports normally
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			    hide: [die]
			tasks: {}
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - run: echo cleaned
			  die:
			    steps:
			      - run: echo died
		`,
	}
	clean := Run(t, Case{Files: files, Args: []string{"yard:clean"}})
	clean.OK(t)

	die := Run(t, Case{Files: files, Args: []string{"yard:die"}})
	die.Fails(t)
}

func TestE2E_Modules_FlattenExposesBareNameAndHidesPrefixedFromList(t *testing.T) {
	// given a module imported with flatten: true, when --list runs, then the
	// bare name is shown instead of the prefixed one — but the prefixed
	// alias still works when called directly, it's just hidden from the menu
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			    flatten: true
			tasks: {}
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - run: echo cleaned
		`,
	}
	list := Run(t, Case{Files: files, Args: []string{"--list"}})
	list.OK(t)
	list.Out(t, "clean")
	list.NotOut(t, "yard:clean")

	bareRun := Run(t, Case{Files: files, Args: []string{"clean"}})
	bareRun.OK(t)
	bareRun.Lines(t, "cleaned")

	prefixedRun := Run(t, Case{Files: files, Args: []string{"yard:clean"}})
	prefixedRun.OK(t)
	prefixedRun.Lines(t, "cleaned")
}

func TestE2E_Modules_FlattenNeverOverridesNativeTask(t *testing.T) {
	// given the parent already has a task named the same as a flattened
	// module task, when run, then the parent's own task wins — and the
	// prefixed alias stays visible in --list since the flat alias lost
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			    flatten: true
			tasks:
			  clean:
			    steps:
			      - run: echo parent-clean
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - run: echo module-clean
		`,
	}
	bare := Run(t, Case{Files: files, Args: []string{"clean"}})
	bare.OK(t)
	bare.Lines(t, "parent-clean")

	list := Run(t, Case{Files: files, Args: []string{"--list"}})
	list.OK(t)
	list.Out(t, "yard:clean")
}

func TestE2E_Modules_TaskCannotCallParentOnlyTask(t *testing.T) {
	// given a module task that tries to call: a task that only exists in the
	// parent, when run, then it fails (why: a module's own Cfg is isolated
	// from the parent — this is what makes a module portable/reusable rather
	// than implicitly coupled to whatever imports it)
	files := Files{
		"hobnob.yml": `
			modules:
			  - yard: yard.yml
			tasks:
			  parent_only:
			    steps:
			      - run: echo should-not-be-reachable
		`,
		"yard.yml": `
			tasks:
			  clean:
			    steps:
			      - call: parent_only
		`,
	}
	res := Run(t, Case{Files: files, Args: []string{"yard:clean"}})
	res.Fails(t)
	res.NotOut(t, "should-not-be-reachable")
}

func TestE2E_Modules_TemplatePathUsesDefaultWhenVarUnset(t *testing.T) {
	// given a module path written as a template with | default, when the
	// referenced var isn't set, then the default path is used
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - farm: '{{.FARM_FILE | default "farm.yml"}}'
				tasks: {}
			`,
			"farm.yml": `
				tasks:
				  milk_cow:
				    steps:
				      - run: echo milked
			`,
		},
		Args: []string{"farm:milk_cow"},
	})
	res.OK(t)
	res.Lines(t, "milked")
}

func TestE2E_Modules_NestedFormatPathKeyBehavesLikeShorthand(t *testing.T) {
	// given the expanded modules: mapping form (path:/show:/flatten: keys),
	// when run, then it behaves identically to the shorthand form
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - yard:
				      path: yard.yml
				      show: [clean]
				      flatten: true
				tasks: {}
			`,
			"yard.yml": `
				tasks:
				  clean:
				    steps:
				      - run: echo cleaned
				  die:
				    steps:
				      - run: echo died
			`,
		},
		Args: []string{"clean"},
	})
	res.OK(t)
	res.Lines(t, "cleaned")

	dieRes := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - yard:
				      path: yard.yml
				      show: [clean]
				      flatten: true
				tasks: {}
			`,
			"yard.yml": `
				tasks:
				  clean:
				    steps:
				      - run: echo cleaned
				  die:
				    steps:
				      - run: echo died
			`,
		},
		Args: []string{"yard:die"},
	})
	dieRes.Fails(t)
}

func TestE2E_Modules_SubdirRelativePathResolvesAgainstTaskfileDir(t *testing.T) {
	// given a module path pointing into a subdirectory, when run, then it
	// resolves relative to the taskfile's own directory
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - sub: subdir/mod.yml
				tasks: {}
			`,
			"subdir/mod.yml": `
				tasks:
				  hello:
				    steps:
				      - run: echo hello-from-subdir
			`,
		},
		Args: []string{"sub:hello"},
	})
	res.OK(t)
	res.Lines(t, "hello-from-subdir")
}

func TestE2E_Modules_NestedImportNamespacesCorrectly(t *testing.T) {
	// given A imports B, and B imports C, when run, then B's own task and
	// C's task (via B's namespace) are both callable from A, but C is not
	// directly reachable from A — only through B
	files := Files{
		"hobnob.yml": `
			modules:
			  - b: b.yml
			tasks: {}
		`,
		"b.yml": `
			modules:
			  - c: c.yml
			tasks:
			  b_task:
			    steps:
			      - run: echo b_task
		`,
		"c.yml": `
			tasks:
			  c_task:
			    steps:
			      - run: echo c_task
		`,
	}
	bTask := Run(t, Case{Files: files, Args: []string{"b:b_task"}})
	bTask.OK(t)
	bTask.Lines(t, "b_task")

	nestedCTask := Run(t, Case{Files: files, Args: []string{"b:c:c_task"}})
	nestedCTask.OK(t)
	nestedCTask.Lines(t, "c_task")

	directCTask := Run(t, Case{Files: files, Args: []string{"c:c_task"}})
	directCTask.Fails(t)
}

func TestE2E_Modules_DiamondImportBothNamespacesWork(t *testing.T) {
	// given A imports both B and C, and both B and C import D, when run,
	// then D's task is reachable under both B's and C's namespace with no
	// error (why: the same module imported twice via different paths must
	// not conflict)
	files := Files{
		"hobnob.yml": `
			modules:
			  - b: b.yml
			  - c: c.yml
			tasks: {}
		`,
		"b.yml": `
			modules:
			  - d: d.yml
			tasks:
			  b_task:
			    steps:
			      - run: echo b_task
		`,
		"c.yml": `
			modules:
			  - d: d.yml
			tasks:
			  c_task:
			    steps:
			      - run: echo c_task
		`,
		"d.yml": `
			tasks:
			  d_task:
			    steps:
			      - run: echo d_task
		`,
	}
	bViaD := Run(t, Case{Files: files, Args: []string{"b:d:d_task"}})
	bViaD.OK(t)
	bViaD.Lines(t, "d_task")

	cViaD := Run(t, Case{Files: files, Args: []string{"c:d:d_task"}})
	cViaD.OK(t)
	cViaD.Lines(t, "d_task")
}

func TestE2E_Modules_SharedDirectAndTransitiveImportBothWork(t *testing.T) {
	// given A imports B (which imports C) and also imports C directly, when
	// run, then C is reachable both via B's namespace and directly
	files := Files{
		"hobnob.yml": `
			modules:
			  - b: b.yml
			  - c: c.yml
			tasks: {}
		`,
		"b.yml": `
			modules:
			  - c: c.yml
			tasks:
			  b_task:
			    steps:
			      - run: echo b_task
		`,
		"c.yml": `
			tasks:
			  c_task:
			    steps:
			      - run: echo c_task
		`,
	}
	viaB := Run(t, Case{Files: files, Args: []string{"b:c:c_task"}})
	viaB.OK(t)

	direct := Run(t, Case{Files: files, Args: []string{"c:c_task"}})
	direct.OK(t)
}

func TestE2E_Modules_CircularImportErrors(t *testing.T) {
	// given A imports B and B imports A, when run, then it fails rather than
	// hanging (why: cycle detection must stop the recursive load)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - b: b.yml
				tasks:
				  t:
				    steps:
				      - run: echo hi
			`,
			"b.yml": `
				modules:
				  - a: hobnob.yml
				tasks: {}
			`,
		},
		Args: []string{"t"},
	})
	res.Fails(t)
}

func TestE2E_Modules_OwnEnvFileStaysPrivateToModule(t *testing.T) {
	// given a module with its own env: block, when a parent task echoes that
	// var, then it's unset there — a module's env: never leaks to the parent
	// (why: ${VAR-UNSET} distinguishes "unset" from "empty", proving
	// this isn't just an empty string)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - run: echo "v=${MODULE_VAR-UNSET}"
			`,
			"mod.yml": `
				env:
				  - module.env
				tasks:
				  ping:
				    steps:
				      - run: echo ping
			`,
			"module.env": "MODULE_VAR=from_module\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "v=UNSET")
}
