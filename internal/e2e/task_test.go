package e2e

import "testing"

func TestE2E_TaskIf_ConditionFalseSkipsWholeTask(t *testing.T) {
	// given a task with if: that evaluates false, when run, then none of its
	// steps execute — not even a step before the one that would have failed
	// (why: task-level if: is a guard on the whole task, not just its first
	// step)
	// Arrange
	yml := `
		tasks:
		  t:
		    if: '[ "{{.ENABLED}}" = "true" ]'
		    steps:
		      - run: echo should-not-run
	`

	// Act
	res := Yml(t, yml, "t", "ENABLED=false")

	// Assert
	res.OK(t)
	res.Lines(t /* none */)
}

func TestE2E_TaskIf_ConditionTrueRunsTask(t *testing.T) {
	// given a task with if: that evaluates true, when run, then its steps
	// execute normally (why: the positive half of the task-level if: check)
	// Arrange
	yml := `
		tasks:
		  t:
		    if: '[ "{{.ENV}}" = "production" ]'
		    steps:
		      - run: echo deployed
	`

	// Act
	res := Yml(t, yml, "t", "ENV=production")

	// Assert
	res.OK(t)
	res.Lines(t, "deployed")
}

func TestE2E_TaskIf_BadConditionErrorsNamingTask(t *testing.T) {
	// given a task with a malformed if: template, when run, then it fails
	// naming the task rather than silently skipping (why: a broken condition
	// is a config bug, not a legitimate "skip")
	// Arrange
	yml := `
		tasks:
		  t:
		    if: '{{.MISSING_CLOSE_BRACE'
		    steps:
		      - run: echo hello
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, `task "t" if:`)
}

func TestE2E_TaskIf_EvaluatedInTaskDirNotCallerDir(t *testing.T) {
	// given a task with its own dir: and an if: that checks a file relative
	// to that dir, when run, then if: runs in the task's own dir, not
	// wherever the caller happened to be (why: regression guard — if: must
	// see the same directory the task's steps will run in)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    dir: sub
				    if: '[ -f marker ]'
				    steps:
				      - run: echo ran
			`,
			"sub/marker": "",
		},
		Args: []string{"t"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "ran")
}
