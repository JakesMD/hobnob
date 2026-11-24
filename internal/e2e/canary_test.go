package e2e

import "testing"

func TestCanary_Run(t *testing.T) {
	// given a run: step, when executed, then its stdout is captured with the
	// task prefix stripped by .Lines (why: proves capture + .Lines)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo hello-from-run
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "hello-from-run")
}

func TestCanary_Set(t *testing.T) {
	// given a set: step referencing an earlier entry, when executed, then a
	// later run: step sees the resolved value (why: proves scope threading
	// through the whole app.Run call, not just stdout plumbing)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - BASE: hello
		          - DERIVED: "{{.BASE}}-world"
		      - run: echo {{.DERIVED}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "hello-world")
}

func TestCanary_GetWithFakePrompt(t *testing.T) {
	// given a get: step and no preset var, when executed with a fake prompt
	// answer, then the prompt is recorded and its answer flows into scope
	// (why: proves the runner.SetPrompts seam end-to-end)
	// Arrange
	c := Case{
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
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "hi-ada")
	res.Prompted(t, "NAME")
}
