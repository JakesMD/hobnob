package e2e

import "testing"

func TestE2E_LogErrors_StepThatCannotStartIsOneLineWithTheStepAsWritten(t *testing.T) {
	// given steps that can't start (a template error, a broken if:, a call:
	// to no task), when the run fails, then each ends with one ✗ line: the step
	// as written, then the error in hobnob's terms, no time, and the task's
	// end line follows (why: Go's template noise says nothing the author can
	// act on)
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
