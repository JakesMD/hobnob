package e2e

// Canary tests for the harness itself (Phase 1): one run: step, one set:
// step, one get: step with a fake prompt. If these three pass, output
// capture, env isolation, cwd, and the prompt seam are all proven working
// before the rest of the suite builds on top of them.

import "testing"

func TestCanary_Run(t *testing.T) {
	// given a run: step, when executed, then its stdout is captured with the
	// task prefix stripped by .Lines (why: proves capture + .Lines)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo hello-from-run
	`, "t")

	res.OK(t)
	res.Lines(t, "hello-from-run")
}

func TestCanary_Set(t *testing.T) {
	// given a set: step referencing an earlier entry, when executed, then a
	// later run: step sees the resolved value (why: proves scope threading
	// through the whole app.Run call, not just stdout plumbing)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - BASE: hello
		          - DERIVED: "{{.BASE}}-world"
		      - run: echo {{.DERIVED}}
	`, "t")

	res.OK(t)
	res.Lines(t, "hello-world")
}

func TestCanary_GetWithFakePrompt(t *testing.T) {
	// given a get: step and no preset var, when executed with a fake prompt
	// answer, then the prompt is recorded and its answer flows into scope
	// (why: proves the runner.SetPrompts seam end-to-end)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - NAME:
			              info: Enter your name
			      - run: echo hi-{{.NAME}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"NAME": {"ada"}},
		TTY:     true,
	})

	res.OK(t)
	res.Lines(t, "hi-ada")
	res.Prompted(t, "NAME")
}
