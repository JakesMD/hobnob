package e2e

import "testing"

func TestE2E_Log_RunAndOutputLinesCarryTheTaskAndAMark(t *testing.T) {
	// given a run: step, when executed, then its command is a "task $" line
	// and its output a "task │" line (why: every line says which task wrote
	// it and what kind of line it is)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Order(t, "t $ echo hi\n", "t │ hi\n")
}

func TestE2E_Log_MultiLineCommandContinuesUnderTheFirstLine(t *testing.T) {
	// given a two-line command, when executed, then the second line is
	// printed under the first, indented to where the command starts (why:
	// continuation lines must not repeat the prefix)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo one
		          echo two
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "t $ echo one\n    echo two\n")
}

func TestE2E_Log_QuietStepNotesHiddenOutput(t *testing.T) {
	// given a quiet: run: step, when executed, then its line ends with a
	// "# …" note and its output is not printed (why: a hidden step must not
	// read as if nothing happened)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo secret-output
		        quiet: installing
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "t $ echo secret-output # installing… (output hidden)\n")
	res.NotOut(t, "t │")
}

func TestE2E_Log_CallPrintsTheCallLine(t *testing.T) {
	// given a call: step, when executed, then a "task → called" line comes
	// before the called task's own lines (why: the called task's lines need
	// to be traceable to the step that started them)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - call: child
		  child:
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Order(t, "t → child\n", "child $ echo hi\n")
}

func TestE2E_Log_CallToAnAlreadyRunOnceTaskIsDoneEarlier(t *testing.T) {
	// given a once: task called twice, when the second call is reached, then
	// it prints the call and "done earlier" and runs nothing (why: a
	// memoized call must still show up in the log)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - call: setup
		      - call: setup
		  setup:
		    once: true
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Order(t, "t → setup\n", "setup $ echo hi\n", "t → setup\n", "setup ✓ done earlier\n")
}

func TestE2E_Log_CallToASkippedTaskPrintsTheCallThenTheSkip(t *testing.T) {
	// given a call: to a task whose if: is false, when executed, then the
	// call line is followed by the task's own "skipped" line (why: the skip
	// belongs to the called task)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - call: lint
		  lint:
		    if: "false"
		    steps:
		      - run: echo lint
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Order(t, "t → lint\n", "lint ⊘ skipped\n")
	res.NotOut(t, "lint $")
}

func TestE2E_Log_SetLoopAndIntoPrintNothing(t *testing.T) {
	// given set:, loop: and into: steps, when executed, then they print no
	// line of their own (why: their values show in the commands that use
	// them)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - A: x
		      - run: echo hi
		        into:
		          - B: stdout
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "t $ echo hi\n")
	res.NotOut(t, "A", "B")
}

func TestE2E_Log_VariableLinesNameTheValueAndWhereItCameFrom(t *testing.T) {
	// given a get: entry answered by typing, by a command-line argument, by
	// the environment and by a secret, when executed, then each prints
	// "task » NAME=value", with a grey note for a value nobody typed and
	// **** for a secret (why: every get: value used is in the log)
	tests := []struct {
		name string
		yml  string
		c    Case
		want string
	}{
		{
			name: "typed",
			yml:  "- get: [ENV]",
			c:    Case{Args: []string{"t"}, Answers: map[string][]string{"ENV": {"prod"}}, TTY: true},
			want: "t » ENV=prod\n",
		},
		{
			name: "command line",
			yml:  "- get: [ENV]",
			c:    Case{Args: []string{"t", "ENV=prod"}},
			want: "t » ENV=prod # from the command line\n",
		},
		{
			name: "environment",
			yml:  "- get: [ENV]",
			c:    Case{Args: []string{"t"}, Env: map[string]string{"ENV": "prod"}},
			want: "t » ENV=prod # from the environment\n",
		},
		{
			name: "typed secret",
			yml:  "- get:\n          - PW:\n              secret: true",
			c:    Case{Args: []string{"t"}, Answers: map[string][]string{"PW": {"hunter2"}}, TTY: true},
			want: "t » PW=****\n",
		},
		{
			name: "secret from the command line",
			yml:  "- get:\n          - PW:\n              secret: true",
			c:    Case{Args: []string{"t", "PW=hunter2"}},
			want: "t » PW=**** # from the command line\n",
		},
		{
			name: "list",
			yml:  "- get:\n          - ENV:\n              options: [a, b]\n              multi: true",
			c:    Case{Args: []string{"t"}, Answers: map[string][]string{"ENV": {"a,b"}}, TTY: true},
			want: "t » ENV=a, b\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			test.c.Yml = "tasks:\n  t:\n    steps:\n      " + test.yml + "\n"

			// Act
			res := Run(t, test.c)

			// Assert
			res.OK(t)
			res.Out(t, test.want)
			res.NotOut(t, "get:", "hunter2")
		})
	}
}

func TestE2E_Log_SkippedOptionalPromptPrintsAMarkedName(t *testing.T) {
	// given an optional text, menu and picker prompt each skipped, when
	// executed, then each prints "task ⊘ NAME" and no value (why: a skip
	// reads the same whatever the prompt type)
	tests := []struct {
		name  string
		entry string
	}{
		{"text", "REPLICAS:\n              optional: true"},
		{"menu", "REPLICAS:\n              optional: true\n              options: [1, 2]"},
		{"picker", "REPLICAS:\n              optional: true\n              type: file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			c := Case{
				Yml:     "tasks:\n  t:\n    steps:\n      - get:\n          - " + test.entry + "\n",
				Args:    []string{"t"},
				Answers: map[string][]string{"REPLICAS": {""}},
				TTY:     true,
			}

			// Act
			res := Run(t, c)

			// Assert
			res.OK(t)
			res.Out(t, "t ⊘ REPLICAS\n")
			res.NotOut(t, "REPLICAS=")
		})
	}
}

func TestE2E_Log_SkippedOptionalWithPromptsOffPrintsAMarkedName(t *testing.T) {
	// given an optional get: with no value and --no-input, when executed,
	// then it prints "task ⊘ NAME" (why: a skip is logged even when nobody
	// was asked)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - REPLICAS:
		              optional: true
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Out(t, "t ⊘ REPLICAS\n")
}

func TestE2E_Log_EmptyValueForARequiredVariablePrintsAVariableLine(t *testing.T) {
	// given a required get: given an empty value on the command line, or
	// already set to "" by a set: step, when executed, then it prints a "»"
	// line, not a skip (why: only an optional prompt left unanswered is a skip)
	tests := []struct {
		name string
		yml  string
		args []string
		want string
	}{
		{
			name: "command line",
			yml:  "- get: [ENV]",
			args: []string{"t", "ENV=", "--no-input"},
			want: "t » ENV= # from the command line\n",
		},
		{
			name: "set to empty",
			yml:  "- set:\n          - ENV: \"\"\n      - get: [ENV]",
			args: []string{"t", "--no-input"},
			want: "t » ENV=\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			c := Case{Yml: "tasks:\n  t:\n    steps:\n      " + test.yml + "\n", Args: test.args}

			// Act
			res := Run(t, c)

			// Assert
			res.OK(t)
			res.Out(t, test.want)
			res.NotOut(t, "⊘")
		})
	}
}

func TestE2E_Log_SkippedCallNamesTheCalledTask(t *testing.T) {
	// given a call: step whose if: is false, when executed, then it prints
	// "task ⊘ called" (why: a skipped step names what it skips)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - call: lint
		        if: 'false'
		  lint:
		    steps:
		      - run: echo linting
	`

	// Act
	res := Yml(t, yml, "deploy")

	// Assert
	res.OK(t)
	res.Out(t, "deploy ⊘ lint\n")
	res.NotOut(t, "linting")
}

func TestE2E_Log_DefaultTakenWithPromptsOffIsNoted(t *testing.T) {
	// given a get: with a default and --no-input, when executed, then its
	// variable line says "# default" (why: nobody typed it)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    steps:
		      - get:
		          - REPLICAS:
		              default: 3
	`

	// Act
	res := Yml(t, yml, "deploy", "--no-input")

	// Assert
	res.OK(t)
	res.Out(t, "deploy » REPLICAS=3 # default\n")
}
