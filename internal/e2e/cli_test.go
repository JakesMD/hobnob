package e2e

import "testing"

func TestE2E_VersionFlag(t *testing.T) {
	// given --version, when run, then prints the binary name and version
	// (why: bug reports and install checks rely on it)
	// Arrange
	c := Case{Yml: "tasks: {}", Args: []string{"--version"}, Discover: true}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "hobnob")
}

func TestE2E_HelpFlagShowsVersionAndUsage(t *testing.T) {
	// given --help, when run, then shows the version-headed usage block (why:
	// help is the first thing a new user reads)
	// Arrange
	yml := "tasks: {}"

	// Act
	res := Yml(t, yml, "--help")

	// Assert
	res.OK(t)
	res.Out(t, "hobnob", "Usage:")
}

func TestE2E_NoArgsDefaultTaskRuns(t *testing.T) {
	// given a taskfile with a default task, when run with no args, then the
	// default task executes (why: the common task shouldn't need its name
	// typed)
	// Arrange
	yml := `
		tasks:
		  default:
		    steps:
		      - run: echo hobnob-default-ran
	`

	// Act
	res := Yml(t, yml)

	// Assert
	res.OK(t)
	res.Lines(t, "hobnob-default-ran")
}

func TestE2E_NoArgsNoDefaultTaskShowsTaskList(t *testing.T) {
	// given a taskfile with no default task, when run with no args and no
	// TTY, then falls back to listing tasks (why: without a terminal, an
	// interactive picker would hang)
	// Arrange
	yml := `
		tasks:
		  build:
		    steps:
		      - run: echo building
	`

	// Act
	res := Yml(t, yml)

	// Assert
	res.OK(t)
	res.Out(t, "Usage:", "build")
}

func TestE2E_NoArgsNoTasksShowsUsageAndEmptyMessage(t *testing.T) {
	// given a taskfile with zero tasks, when run with no args even on a
	// simulated TTY, then shows the same usage+list output as --help (why:
	// regression guard — the interactive zero-task path used to print a bare
	// "No tasks available." and skip the usage block)
	// Arrange
	c := Case{Yml: "tasks:\n", TTY: true}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "Usage:", "No tasks found")
	res.NotOut(t, "No tasks available.")
}

func TestE2E_InternalTaskRejected(t *testing.T) {
	// given a "_"-prefixed task name, when run directly from the CLI, then
	// fails instead of executing it (why: internal tasks are call:-only)
	// Arrange
	yml := `
		tasks:
		  _helper:
		    steps:
		      - run: echo should-not-run
	`

	// Act
	res := Yml(t, yml, "_helper")

	// Assert
	res.Fails(t)
	res.Err(t, "internal")
	res.NotOut(t, "should-not-run")
}

func TestE2E_NamedTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given a named task that succeeds, when run, then prints ✓ with the task
	// name (why: success feedback must identify which task completed)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - run: echo ok
	`

	// Act
	res := Yml(t, yml, "deploy")

	// Assert
	res.OK(t)
	res.Out(t, "✓", "[deploy]", "done")
}

func TestE2E_NamedTaskFailurePrintsFailureMessage(t *testing.T) {
	// given a named task that fails, when run, then prints ✗ with the task
	// name (why: failure feedback must identify which task failed)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - run: exit 1
	`

	// Act
	res := Yml(t, yml, "deploy")

	// Assert
	res.Fails(t)
	res.Err(t, "✗", "[deploy]")
}

func TestE2E_DefaultTaskSuccessPrintsSuccessMessage(t *testing.T) {
	// given a default task that succeeds, when run with no args, then success
	// feedback applies to it same as any named task (why: reaching a task
	// without naming it must not change its feedback)
	// Arrange
	yml := `
		tasks:
		  default:
		    steps:
		      - run: echo ok
	`

	// Act
	res := Yml(t, yml)

	// Assert
	res.OK(t)
	res.Out(t, "✓", "[default]", "done")
}

func TestE2E_DefaultTaskFailurePrintsFailureMessage(t *testing.T) {
	// given a default task that fails, when run with no args, then failure
	// feedback applies to it same as any named task (why: reaching a task
	// without naming it must not change its feedback)
	// Arrange
	yml := `
		tasks:
		  default:
		    steps:
		      - run: exit 1
	`

	// Act
	res := Yml(t, yml)

	// Assert
	res.Fails(t)
	res.Err(t, "✗", "[default]")
}

func TestE2E_NoArgsDefaultTaskFailsFastWhenStdinNotTerminal(t *testing.T) {
	// given a default task with a required get: and no TTY, when run with no
	// args, then fails fast with the --no-input error instead of trying to
	// open a prompt (why: regression guard — the no-args path used to
	// compute noPrompts from CI alone, ignoring terminal detection)
	// Arrange
	yml := `
		tasks:
		  default:
		    steps:
		      - get: [FOO]
	`

	// Act
	res := Yml(t, yml)

	// Assert
	res.Fails(t)
	res.Err(t, "--no-input: FOO requires input")
}

func TestE2E_SelectFlagNoTTYFallsBackToList(t *testing.T) {
	// given --select with no TTY, when run, then falls back to a plain task
	// list (why: without a terminal, an interactive picker would hang)
	// Arrange
	yml := `
		tasks:
		  build:
		    info: Build it
		    steps:
		      - run: echo building
		  deploy:
		    steps:
		      - run: echo deploying
	`

	// Act
	res := Yml(t, yml, "--select")

	// Assert
	res.OK(t)
	res.Out(t, "build", "deploy")
	res.NotOut(t, "Usage:")
}

func TestE2E_SelectFlagWithNoInputFallsBackToList(t *testing.T) {
	// given --select --no-input even on a TTY, when run, then --no-input
	// still wins and falls back to the list (why: an explicit flag must
	// override an otherwise-interactive terminal)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  build:
			    steps:
			      - run: echo building
		`,
		Args: []string{"--select", "--no-input"},
		TTY:  true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "build")
}

func TestE2E_ListFlag(t *testing.T) {
	// given --list, when run, then prints every visible task (why: --list is
	// how people and shell completion discover tasks)
	// Arrange
	yml := `
		tasks:
		  build:
		    steps:
		      - run: echo building
		  _hidden:
		    steps:
		      - run: echo hidden
	`

	// Act
	res := Yml(t, yml, "--list")

	// Assert
	res.OK(t)
	res.Out(t, "build")
	res.NotOut(t, "_hidden")
}

func TestE2E_NoInputFlagFailsFastOnMissingVar(t *testing.T) {
	// given --no-input on a task requiring a var with no default, when run,
	// then fails immediately naming the var rather than prompting (why: CI
	// has nobody to answer, so it must fail fast and say what's missing)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - get: [ENV]
		      - run: echo {{.ENV}}
	`

	// Act
	res := Yml(t, yml, "deploy", "--no-input")

	// Assert
	res.Fails(t)
	res.Err(t, "--no-input: ENV requires input")
}

func TestE2E_FileFlagCustomFilename(t *testing.T) {
	// given --file naming a taskfile that isn't hobnob.yml, when run, then
	// hobnob uses that file instead of auto-discovery (why: --file is the
	// escape hatch from the hobnob.yml/hobnob.yaml naming convention)
	// Discover:true stops the harness auto-adding --file <dir>/hobnob.yml;
	// the relative path resolves against cwd, which Run already chdir'd into.
	// Arrange
	c := Case{
		Files:    Files{"ops/custom.yml": "tasks:\n  t:\n    steps:\n      - run: echo custom-file-used\n"},
		Args:     []string{"--file", "ops/custom.yml", "t"},
		Discover: true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "custom-file-used")
}

func TestE2E_DiscoveryWalksUpToParentDir(t *testing.T) {
	// given hobnob.yml only at the root and no --file, when run from a
	// subdirectory, then findTaskfile walks up and finds it (why: the
	// documented no-flag "run from anywhere under the project" workflow)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo found-via-discovery
		`,
		Args:     []string{"t"},
		Cwd:      "nested/deeper",
		Discover: true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "found-via-discovery")
}

func TestE2E_NoTaskfileFoundShowsUsage(t *testing.T) {
	// given no hobnob.yml anywhere up the tree and no --file, when run with
	// no args, then prints usage instead of erroring (why: a bare `hobnob` in
	// an unrelated directory should guide the user, not crash)
	// Arrange
	c := Case{Files: Files{}, Discover: true}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "Usage:")
}

func TestE2E_CompletionScript(t *testing.T) {
	// given `completion <shell>`, when run, then prints that shell's
	// completion script, or errors naming supported shells (why: an unknown
	// shell must fail loudly rather than print nothing to an rc file)
	// Arrange
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
			// Act
			res := Run(t, Case{Yml: "tasks: {}", Args: []string{"completion", test.shell}, Discover: true})

			// Assert
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
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - run: echo ok
	`

	// Act
	res := Yml(t, yml, "deploy", "not-a-valid-arg")

	// Assert
	res.Fails(t)
	res.Err(t, "not-a-valid-arg")
}
