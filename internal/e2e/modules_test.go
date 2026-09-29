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

func TestE2E_Modules_OwnEnvFileVisibleToItsOwnTasks(t *testing.T) {
	// given a module with its own env: block, when one of the module's own
	// tasks echoes that var, then it sees the value (why: regression guard —
	// the module-local scope env: files are loaded into was previously only
	// ever used to evaluate the module's own load-time templates (path,
	// show:/hide:/flatten:) and never reached the runtime scope a module task
	// actually executes with, so this rendered empty despite
	// OwnEnvFileStaysPrivateToModule already covering the non-leakage half)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:ping
			`,
			"mod.yml": `
				env:
				  - module.env
				tasks:
				  ping:
				    steps:
				      - run: echo "v=${MODULE_VAR-UNSET}"
			`,
			"module.env": "MODULE_VAR=from_module\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "v=from_module")
}

func TestE2E_Modules_OwnEnvFileDoesNotOverrideExplicitWithVar(t *testing.T) {
	// given a module's own env: default and a call: site passing the same var
	// explicitly via with:, when the module task runs, then the caller's
	// with: value wins (why: a module's env: block is a default for its
	// subtree, not an override — matching how a root file's own env: block is
	// itself just the lowest layer scope.Load applies)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:ping
				        with:
				          - MODULE_VAR: from_caller
			`,
			"mod.yml": `
				env:
				  - module.env
				tasks:
				  ping:
				    steps:
				      - run: echo "v={{.MODULE_VAR}}"
			`,
			"module.env": "MODULE_VAR=from_module\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "v=from_caller")
}

func TestE2E_Modules_OwnConstAlwaysOverridesParentValue(t *testing.T) {
	// given a module's own const: entry and a parent const: entry of the
	// same name, when a module task runs, then the module's own value wins
	// in its own subtree (why: a module's const: is a hard fact about that
	// module, not a default — ordinary lexical shadowing, the nearest
	// declaration wins, unlike a module's env:/vars:, which only ever fills
	// a gap)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				const:
				  - LABEL: from-parent
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				const:
				  - LABEL: from-module
				tasks:
				  show:
				    steps:
				      - run: echo label={{.LABEL}}
			`,
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "label=from-module")
}

func TestE2E_Modules_OwnConstDoesNotLeakToParent(t *testing.T) {
	// given a module's own const: entry, when the PARENT's own task (not a
	// module task) echoes that name, then it's unset there — a module's
	// const: never leaks up, matching every other file-scoped rule
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
				      - run: echo "v=${LABEL-UNSET}"
			`,
			"mod.yml": `
				const:
				  - LABEL: from-module
				tasks:
				  show:
				    steps:
				      - run: echo label={{.LABEL}}
			`,
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "label=from-module", "v=UNSET")
}

func TestE2E_Modules_OwnConstOverridesEvenACLIArg(t *testing.T) {
	// given a module's own const: entry and a CLI arg of the same name,
	// when a module task runs, then the module's const: still wins (why:
	// const: outranks CLI args at the root, and a module's own const:
	// shadows its subtree just as completely — the nearest declaration
	// always wins, regardless of what's above it)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				const:
				  - LABEL: from-module
				tasks:
				  show:
				    steps:
				      - run: echo label={{.LABEL}}
			`,
		},
		Args: []string{"t", "LABEL=from-cli"},
	})
	res.OK(t)
	res.Lines(t, "label=from-module")
}

func TestE2E_Modules_OwnVarsOnlyFillsGapLikeEnvDoes(t *testing.T) {
	// given a module's own vars: entry and a CLI arg of the same name, when
	// a module task runs, then the CLI arg wins (why: unlike const:, a
	// module's vars: is a default for its subtree, not an override — the
	// module can't tell whether the inherited value came from something
	// higher-priority than a default, so it never risks clobbering it)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				vars:
				  - HOST: mod-default
				tasks:
				  show:
				    steps:
				      - run: echo host={{.HOST}}
			`,
		},
		Args: []string{"t", "HOST=from-cli"},
	})
	res.OK(t)
	res.Lines(t, "host=from-cli")
}

func TestE2E_Modules_OwnVarsUsedWhenNothingElseSetsIt(t *testing.T) {
	// given a module's own vars: entry and nothing else supplying that
	// name, when a module task runs, then the module's default is used —
	// the positive half of OwnVarsOnlyFillsGapLikeEnvDoes
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				vars:
				  - HOST: mod-default
				tasks:
				  show:
				    steps:
				      - run: echo host={{.HOST}}
			`,
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "host=mod-default")
}

func TestE2E_Modules_OwnVarsReadsOwnEnvFile(t *testing.T) {
	// given a module's own env: file and its own vars: entry built from it,
	// when a module task runs, then vars: sees the env: file's value — a
	// module's own env:/vars: follow the same upward-read model as the root
	// chain (docs/adr/0001)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				env:
				  - module.env
				vars:
				  - URL: "https://{{.API_HOST}}/v1"
				tasks:
				  show:
				    steps:
				      - run: echo {{.URL}}
			`,
			"module.env": "API_HOST=staging.example.com\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "https://staging.example.com/v1")
}

func TestE2E_Modules_OwnVarsReadsOwnConst(t *testing.T) {
	// given a module's own const: entry and its own vars: entry built from
	// it, when a module task runs, then vars: sees the const: value
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				const:
				  - REGION: eu
				vars:
				  - LABEL: "app-{{.REGION}}"
				tasks:
				  show:
				    steps:
				      - run: echo {{.LABEL}}
			`,
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "app-eu")
}

func TestE2E_Modules_OwnEnvPathReferencingOwnVarsNameFailsAtLoad(t *testing.T) {
	// given a module's own env: path template referencing its own vars: name,
	// when the file loads, then it errors naming the rule — same check as the
	// root chain, scoped to the module's own file
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - mod: mod.yml
				tasks:
				  t:
				    steps:
				      - call: mod:show
			`,
			"mod.yml": `
				vars:
				  - STAGE: dev

				env:
				  - "{{.STAGE}}.env"

				tasks:
				  show:
				    steps:
				      - run: echo hi
			`,
		},
		Args: []string{"t"},
	})
	res.Fails(t)
	res.Err(t, `env: "{{.STAGE}}.env" references .STAGE, declared in vars:`)
}

func TestE2E_Modules_OwnVarsBuildFromTheValueThatWins(t *testing.T) {
	// given a module whose env: file sets STAGE=dev and whose vars: builds
	// LABEL from STAGE, when run with a STAGE=prod CLI arg, then LABEL is
	// built from prod (why: a module's chain reads its importer's resolved
	// scope, so its env: file loses to the CLI arg before vars: reads it)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - call: m:show
			`,
			"mod.yml": `
				env:
				  - .env
				vars:
				  - LABEL: "built-from-{{.STAGE}}"
				tasks:
				  show:
				    steps:
				      - run: echo "STAGE={{.STAGE}} LABEL={{.LABEL}}"
			`,
			".env": "STAGE=dev\n",
		},
		Args: []string{"a", "STAGE=prod"},
	})
	res.OK(t)
	res.Lines(t, "STAGE=prod LABEL=built-from-prod")
}

func TestE2E_Modules_NestedModuleTaskSeesParentModuleFileScope(t *testing.T) {
	// given a parent module whose env: file sets REGION and a sub-module
	// whose vars: builds from it, when the root calls the sub-module's task
	// directly, then both REGION and the built value reach it (why: a
	// module's file scope applies to its whole subtree, so the value the
	// sub-module read at load is the value its task sees at run time)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - p: ./parent.yml
				tasks:
				  a:
				    steps:
				      - call: p:c:show
			`,
			"parent.yml": `
				env:
				  - parent.env
				modules:
				  - c: ./child.yml
				tasks: {}
			`,
			"parent.env": "REGION=eu\n",
			"child.yml": `
				env:
				  - child.env
				vars:
				  - HOST: "{{.REGION}}.example.com"
				tasks:
				  show:
				    steps:
				      - run: echo "REGION={{.REGION}} HOST={{.HOST}}"
			`,
			"child.env": "REGION=us\n",
		},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "REGION=eu HOST=eu.example.com")
}

func TestE2E_Modules_SetStepOnOSEnvNameBeatsModuleVarsDefault(t *testing.T) {
	// given EDITOR only in the OS env and a module whose vars: defaults it,
	// when a root task's set: step overwrites EDITOR and then calls the
	// module's task, then the set: value reaches it (why: a timeline write
	// claims the name, so the module's vars: may only fill a gap, not
	// clobber what a step just wrote)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - set:
				          - EDITOR: from-set-step
				      - call: m:show
			`,
			"mod.yml": `
				vars:
				  - EDITOR: from-module-vars
				tasks:
				  show:
				    steps:
				      - run: echo "EDITOR={{.EDITOR}}"
			`,
		},
		Env:  map[string]string{"EDITOR": "vim"},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "EDITOR=from-set-step")
}

func TestE2E_Modules_CallWithOnOSEnvNameBeatsModuleVarsDefault(t *testing.T) {
	// given EDITOR only in the OS env and a module whose vars: defaults it,
	// when a root task calls the module's task passing EDITOR via with:,
	// then the with: value reaches it (why: with: is a timeline write too,
	// so it claims the name just as set: does)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - call: m:show
				        with:
				          - EDITOR: from-with
			`,
			"mod.yml": `
				vars:
				  - EDITOR: from-module-vars
				tasks:
				  show:
				    steps:
				      - run: echo "EDITOR={{.EDITOR}}"
			`,
		},
		Env:  map[string]string{"EDITOR": "vim"},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "EDITOR=from-with")
}

func TestE2E_Modules_CallIntoOnOSEnvNameBeatsModuleVarsDefault(t *testing.T) {
	// given EDITOR only in the OS env and a module whose vars: defaults it,
	// when a root task pulls EDITOR back from a call: via into: and then
	// calls the module's task, then the into: value reaches it (why: into:
	// is a timeline write too, so it claims the name just as set: does)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - call: _pick
				        into:
				          - EDITOR: .PICKED
				      - call: m:show
				  _pick:
				    steps:
				      - set:
				          - PICKED: from-into
			`,
			"mod.yml": `
				vars:
				  - EDITOR: from-module-vars
				tasks:
				  show:
				    steps:
				      - run: echo "EDITOR={{.EDITOR}}"
			`,
		},
		Env:  map[string]string{"EDITOR": "vim"},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "EDITOR=from-into")
}

func TestE2E_Modules_LoopVarOnOSEnvNameBeatsModuleVarsDefault(t *testing.T) {
	// given ITEM only in the OS env and a module whose vars: defaults it,
	// when a root task loops and calls the module's task from the loop body,
	// then each iteration's ITEM reaches it (why: a loop: binding is a
	// timeline write for the iteration, so it claims the name just as set:
	// does)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - loop: [one, two]
				        steps:
				          - call: m:show
			`,
			"mod.yml": `
				vars:
				  - ITEM: from-module-vars
				tasks:
				  show:
				    steps:
				      - run: echo "ITEM={{.ITEM}}"
			`,
		},
		Env:  map[string]string{"ITEM": "from-env"},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "ITEM=one", "ITEM=two")
}

func TestE2E_Modules_GetAnsweredByOSEnvBeatsModuleVarsDefault(t *testing.T) {
	// given EDITOR only in the OS env and a module whose vars: defaults it,
	// when a root task's get: accepts the OS env value as its answer and
	// then calls the module's task, then the accepted answer reaches it
	// (why: a get: answered from scope claims the name, so a later default
	// can't quietly swap out what the task already took as its answer)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				modules:
				  - m: ./mod.yml
				tasks:
				  a:
				    steps:
				      - get: [EDITOR]
				      - call: m:show
			`,
			"mod.yml": `
				vars:
				  - EDITOR: from-module-vars
				tasks:
				  show:
				    steps:
				      - run: echo "EDITOR={{.EDITOR}}"
			`,
		},
		Env:  map[string]string{"EDITOR": "vim"},
		Args: []string{"a"},
	})
	res.OK(t)
	res.Lines(t, "EDITOR=vim")
}
