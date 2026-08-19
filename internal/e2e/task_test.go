package e2e

import "testing"

func TestE2E_TaskIf_ConditionFalseSkipsWholeTask(t *testing.T) {
	// given a task with if: that evaluates false, when run, then none of its
	// steps execute — not even a step before the one that would have failed
	// (why: task-level if: is a guard on the whole task, not just its first
	// step)
	res := Yml(t, `
		tasks:
		  t:
		    if: '[ "{{.ENABLED}}" = "true" ]'
		    steps:
		      - run: echo should-not-run
	`, "t", "ENABLED=false")
	res.OK(t)
	res.Lines(t /* none */)
}

func TestE2E_TaskIf_ConditionTrueRunsTask(t *testing.T) {
	// given a task with if: that evaluates true, when run, then its steps
	// execute normally
	res := Yml(t, `
		tasks:
		  t:
		    if: '[ "{{.ENV}}" = "production" ]'
		    steps:
		      - run: echo deployed
	`, "t", "ENV=production")
	res.OK(t)
	res.Lines(t, "deployed")
}

func TestE2E_TaskIf_BadConditionErrorsNamingTask(t *testing.T) {
	// given a task with a malformed if: template, when run, then it fails
	// naming the task rather than silently skipping (why: a broken condition
	// is a config bug, not a legitimate "skip")
	res := Yml(t, `
		tasks:
		  t:
		    if: '{{.MISSING_CLOSE_BRACE'
		    steps:
		      - run: echo hello
	`, "t")
	res.Fails(t)
	res.Err(t, `task "t" if:`)
}

func TestE2E_TaskIf_EvaluatedInTaskDirNotCallerDir(t *testing.T) {
	// given a task with its own dir: and an if: that checks a file relative
	// to that dir, when run, then if: runs in the task's own dir, not
	// wherever the caller happened to be (why: regression guard — if: must
	// see the same directory the task's steps will run in)
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "ran")
}
