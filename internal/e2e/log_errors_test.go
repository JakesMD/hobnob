package e2e

import "testing"

func TestE2E_LogErrors_StepThatCannotStartIsOneLineWithTheStepAsWritten(t *testing.T) {
	// given steps that can't start (a template error, a broken if:, a call:
	// to no task), when the run fails, then each ends with one ✗ line: the step
	// as written, then the error in hobnob's terms, no time, and the task's
	// end line follows (why: Go's template noise says nothing the author can
	// act on)
	// Arrange
	tests := []struct {
		name string
		yml  string
		want string
	}{
		{"template error", `
			tasks:
			  deploy:
			    steps:
			      - run: echo {{ upper .X }}
		`, `^deploy ✗ echo \{\{ upper \.X \}\}: upper: expected a string, got nil; pipe through \| string to convert\ndeploy ✗ failed after ` + secs + `\n$`},
		{"broken if", `
			tasks:
			  deploy:
			    steps:
			      - run: echo hi
			        if: "{{ nope }}"
		`, `^deploy ✗ echo hi: if \{\{ nope \}\}: function "nope" not defined\ndeploy ✗ failed after ` + secs + `\n$`},
		{"call to no task", `
			tasks:
			  deploy:
			    steps:
			      - call: nope
		`, `^deploy ✗ nope: no task "nope"\ndeploy ✗ failed after ` + secs + `\n$`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			res := Yml(t, test.yml, "deploy")

			// Assert
			res.Fails(t)
			res.streamMatches(t, "stderr", res.Stderr, test.want)
		})
	}
}

func TestE2E_LogErrors_CommandThatCannotStartEndsLikeAnyFailedStep(t *testing.T) {
	// given a command that can't start, or an into: that can't take its
	// output, when the run fails, then the step ends with ✗ and its time like
	// any failed step, with a reason in hobnob's terms (why: a Go exec error
	// like "fork/exec /bin/sh: no such file or directory" blames the wrong
	// thing)
	// Arrange
	tests := []struct {
		name string
		yml  string
		want string
	}{
		{"missing command", `
			tasks:
			  deploy:
			    steps:
			      - run: [nosuchcmd, a]
		`, `^deploy \$ nosuchcmd a\ndeploy ✗ ` + secs + ` nosuchcmd a: no such command\ndeploy ✗ failed after ` + secs + `\n$`},
		{"missing dir", `
			tasks:
			  deploy:
			    steps:
			      - run: echo hi
			        dir: ./nowhere
		`, `deploy ✗ ` + secs + ` echo hi: no directory \./nowhere\ndeploy ✗ failed after ` + secs + `\n$`},
		{"into cannot take the output", `
			tasks:
			  deploy:
			    steps:
			      - run: echo notjson
			        into:
			          - X: stdout | json
		`, `deploy │ notjson\ndeploy ✗ ` + secs + ` echo notjson: into X: [^\n]*\ndeploy ✗ failed after ` + secs + `\n$`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			res := Yml(t, test.yml, "deploy")

			// Assert
			res.Fails(t)
			res.streamMatches(t, "stderr", res.Stdout+res.Stderr, test.want)
		})
	}
}

func TestE2E_LogErrors_BeforeARunHobnobTakesTheTasksPlace(t *testing.T) {
	// given errors before any task runs, when hobnob reports them, then each
	// is a "hobnob ✗" line on stderr (why: the same log style as a run, and
	// no "error:" prefix to tell apart from a task's own output)
	// Arrange
	tests := []struct {
		name string
		c    Case
		want string
	}{
		{"unparsable taskfile", Case{Yml: "tasks: [\n", Args: []string{"x"}}, `^hobnob ✗ hobnob\.yml line 1: did not find expected node content\n$`},
		{"no such task", Case{Yml: "tasks: {}\n", Args: []string{"nope"}}, `^hobnob ✗ no task "nope"\n$`},
		{"no taskfile", Case{Files: Files{}, Args: []string{"x"}, Discover: true}, `^hobnob ✗ no hobnob\.yml or hobnob\.yaml in \S+ or any folder above it\n$`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Act
			res := Run(t, test.c)

			// Assert
			res.Fails(t)
			res.streamMatches(t, "stderr", res.Stderr, test.want)
		})
	}
}

func TestE2E_LogErrors_MissingEnvFileWarnsAsHobnobAndTheRunCarriesOn(t *testing.T) {
	// given an env: file that doesn't exist, when the task runs, then a
	// "hobnob !" line names the file as written and the task still runs (why:
	// an optional env file mustn't block every task)
	// Arrange
	yml := `
		env:
		  - missing.env
		tasks:
		  t:
		    steps:
		      - run: echo ran
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.streamMatches(t, "stderr", res.Stderr, `^hobnob ! no env file missing\.env, skipping it\n$`)
}

func TestE2E_LogErrors_DemoNoticeIsAHobnobNote(t *testing.T) {
	// given --demo, when a demo task runs, then the notice is "hobnob » …" on
	// stderr (why: it follows the log's style and stays off a piped stdout)
	// Act
	res := Run(t, Case{Files: Files{}, Args: []string{"--demo", "--list"}, Discover: true})

	// Assert
	res.OK(t)
	res.streamMatches(t, "stderr", res.Stderr, `^hobnob » Running the built-in demo\. Write your own: \S+\n$`)
}
