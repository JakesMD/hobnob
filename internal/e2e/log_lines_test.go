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
