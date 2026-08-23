package e2e

// Covers the CLI surface: flag dispatch, no-args behavior, task-name
// rejection, success/failure chrome, --select fallback, --file, taskfile
// discovery, and completion. Replaces cmd/hobnob's old subprocess-based
// main_test.go — the 13 scenarios there all have an equivalent here, driven
// in-process instead of through os/exec.

import "testing"

func TestE2E_VersionFlag(t *testing.T) {
	// given --version, when run, then prints the binary name and version
	res := Run(t, Case{Yml: "tasks: {}", Args: []string{"--version"}, Discover: true})
	res.OK(t)
	res.Out(t, "hobnob")
}

func TestE2E_HelpFlagShowsVersionAndUsage(t *testing.T) {
	// given --help, when run, then shows the version-headed usage block
	res := Yml(t, "tasks: {}", "--help")
	res.OK(t)
	res.Out(t, "hobnob", "Usage:")
}

func TestE2E_NoArgsDefaultTaskRuns(t *testing.T) {
	// given a taskfile with a default task, when run with no args, then the
	// default task executes without needing its name typed
	res := Yml(t, `
		tasks:
		  default:
		    steps:
		      - run: echo hobnob-default-ran
	`)
	res.OK(t)
	res.Lines(t, "hobnob-default-ran")
}

func TestE2E_NoArgsNoDefaultTaskShowsTaskList(t *testing.T) {
	// given a taskfile with no default task, when run with no args and no TTY,
	// then falls back to listing tasks instead of hanging on a picker
	res := Yml(t, `
		tasks:
		  build:
		    steps:
		      - run: echo building
	`)
	res.OK(t)
	res.Out(t, "Usage:", "build")
}

func TestE2E_NoArgsNoTasksShowsUsageAndEmptyMessage(t *testing.T) {
	// given a taskfile with zero tasks, when run with no args even on a
	// simulated TTY, then shows the same usage+list output as --help (why:
	// regression guard — the interactive zero-task path used to print a bare
	// "No tasks available." and skip the usage block)
	res := Run(t, Case{Yml: "tasks:\n", TTY: true})
	res.OK(t)
	res.Out(t, "Usage:", "No tasks found")
	res.NotOut(t, "No tasks available.")
}

func TestE2E_InternalTaskRejected(t *testing.T) {
	// given a "_"-prefixed task name, when run directly from the CLI, then
	// fails instead of executing it (why: internal tasks are call:-only)
	res := Yml(t, `
		tasks:
		  _helper:
		    steps:
		      - run: echo should-not-run
	`, "_helper")
	res.Fails(t)
	res.Err(t, "internal")
	res.NotOut(t, "should-not-run")
}

func TestE2E_NamedTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given a named task that succeeds, when run, then prints ✓ with the task
	// name so success feedback identifies which task completed
	res := Yml(t, `
		tasks:
		  deploy:
		    steps:
		      - run: echo ok
	`, "deploy")
	res.OK(t)
	res.Out(t, "✓", "[deploy]", "done")
}

func TestE2E_NamedTaskFailurePrintsFailureMessage(t *testing.T) {
	// given a named task that fails, when run, then prints ✗ with the task
	// name so failure feedback identifies which task failed
	res := Yml(t, `
		tasks:
		  deploy:
		    steps:
		      - run: exit 1
	`, "deploy")
	res.Fails(t)
	res.Err(t, "✗", "[deploy]")
}

func TestE2E_DefaultTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given a default task that succeeds, when run with no args, then success
	// feedback applies to it same as any named task
	res := Yml(t, `
		tasks:
		  default:
		    steps:
		      - run: echo ok
	`)
	res.OK(t)
	res.Out(t, "✓", "[default]", "done")
}

func TestE2E_DefaultTaskFailurePrintsFailureMessage(t *testing.T) {
	// given a default task that fails, when run with no args, then failure
	// feedback applies to it same as any named task
	res := Yml(t, `
		tasks:
		  default:
		    steps:
		      - run: exit 1
	`)
	res.Fails(t)
	res.Err(t, "✗", "[default]")
}

func TestE2E_NoArgsDefaultTaskFailsFastWhenStdinNotTerminal(t *testing.T) {
	// given a default task with a required get: and no TTY, when run with no
	// args, then fails fast with the --no-input error instead of trying to
	// open a prompt (why: regression guard — the no-args path used to
	// compute noPrompts from CI alone, ignoring terminal detection)
	res := Yml(t, `
		tasks:
		  default:
		    steps:
		      - get: [FOO]
	`)
	res.Fails(t)
	res.Err(t, "--no-input: FOO requires input")
}

func TestE2E_SelectFlagNoTTYFallsBackToList(t *testing.T) {
	// given --select with no TTY, when run, then falls back to a plain task
	// list instead of hanging on an interactive picker
	res := Yml(t, `
		tasks:
		  build:
		    info: Build it
		    steps:
		      - run: echo building
		  deploy:
		    steps:
		      - run: echo deploying
	`, "--select")
	res.OK(t)
	res.Out(t, "build", "deploy")
	res.NotOut(t, "Usage:")
}

func TestE2E_SelectFlagWithNoInputFallsBackToList(t *testing.T) {
	// given --select --no-input even on a TTY, when run, then --no-input
	// still wins and falls back to the list (why: an explicit flag must
	// override an otherwise-interactive terminal)
	res := Run(t, Case{
		Yml: `
			tasks:
			  build:
			    steps:
			      - run: echo building
		`,
		Args: []string{"--select", "--no-input"},
		TTY:  true,
	})
	res.OK(t)
	res.Out(t, "build")
}

func TestE2E_ListFlag(t *testing.T) {
	// given --list, when run, then prints every visible task
	res := Yml(t, `
		tasks:
		  build:
		    steps:
		      - run: echo building
		  _hidden:
		    steps:
		      - run: echo hidden
	`, "--list")
	res.OK(t)
	res.Out(t, "build")
	res.NotOut(t, "_hidden")
}

func TestE2E_NoInputFlagFailsFastOnMissingVar(t *testing.T) {
	// given --no-input on a task requiring a var with no default, when run,
	// then fails immediately naming the var rather than prompting
	res := Yml(t, `
		tasks:
		  deploy:
		    steps:
		      - get: [ENV]
		      - run: echo {{.ENV}}
	`, "deploy", "--no-input")
	res.Fails(t)
	res.Err(t, "--no-input: ENV requires input")
}

func TestE2E_FileFlagCustomFilename(t *testing.T) {
	// given --file naming a taskfile that isn't hobnob.yml, when run, then
	// hobnob uses that file instead of auto-discovery (why: --file is the
	// escape hatch from the hobnob.yml/hobnob.yaml naming convention)
	// Discover:true stops the harness auto-adding --file <dir>/hobnob.yml;
	// the relative path resolves against cwd, which Run already chdir'd into.
	res := Run(t, Case{
		Files:    Files{"ops/custom.yml": "tasks:\n  t:\n    steps:\n      - run: echo custom-file-used\n"},
		Args:     []string{"--file", "ops/custom.yml", "t"},
		Discover: true,
	})
	res.OK(t)
	res.Lines(t, "custom-file-used")
}

func TestE2E_DiscoveryWalksUpToParentDir(t *testing.T) {
	// given hobnob.yml only at the root and no --file, when run from a
	// subdirectory, then findTaskfile walks up and finds it (why: the
	// documented no-flag "run from anywhere under the project" workflow)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo found-via-discovery
		`,
		Args:     []string{"t"},
		Cwd:      "nested/deeper",
		Discover: true,
	})
	res.OK(t)
	res.Lines(t, "found-via-discovery")
}

func TestE2E_NoTaskfileFoundShowsUsage(t *testing.T) {
	// given no hobnob.yml anywhere up the tree and no --file, when run with
	// no args, then prints usage instead of erroring (why: a bare `hobnob` in
	// an unrelated directory should guide the user, not crash)
	res := Run(t, Case{Files: Files{}, Discover: true})
	res.OK(t)
	res.Out(t, "Usage:")
}

func TestE2E_CompletionScript(t *testing.T) {
	tests := []struct {
		shell   string
		wantErr bool
	}{
		{shell: "bash"},
		{shell: "zsh"},
		{shell: "fish"},
		{shell: "powershell", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.shell, func(t *testing.T) {
			// given `completion <shell>`, when run, then prints that shell's
			// completion script, or errors naming supported shells
			res := Run(t, Case{Yml: "tasks: {}", Args: []string{"completion", test.shell}, Discover: true})
			if test.wantErr {
				res.Fails(t)
				res.Err(t, "unknown shell")
				return
			}
			res.OK(t)
			if res.Stdout == "" {
				t.Errorf("expected non-empty completion script for %s, got empty stdout", test.shell)
			}
		})
	}
}

func TestE2E_InvalidCLIArgErrors(t *testing.T) {
	// given a task arg that's neither KEY=VALUE nor --no-input, when run,
	// then fails naming the bad argument (why: silently ignoring a typo'd
	// flag would be worse than failing loudly)
	res := Yml(t, `
		tasks:
		  deploy:
		    steps:
		      - run: echo ok
	`, "deploy", "not-a-valid-arg")
	res.Fails(t)
	res.Err(t, "not-a-valid-arg")
}
