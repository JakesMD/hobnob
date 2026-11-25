package e2e

import "testing"

const quietRun = `
	tasks:
	  deploy:
	    steps:
	      - set:
	          - ENV: prod
	      - run: echo building $ENV
	        quiet: compiling
	      - call: setup
	      - run: echo visible
	      - if: "false"
	        run: echo skipped
	  setup:
	    steps:
	      - run: echo installing
`

func TestE2E_Log_QuietRunKeepsOnlyCommandLinesAndTaskEnds(t *testing.T) {
	// given a run with a call, a quiet: note and command output, when it
	// runs with the flag, its short form or the env var, then stdout holds
	// only each $ line without notes and each task's end (why: agents and
	// logs want the story without the noise)
	// Arrange
	for name, c := range map[string]Case{
		"long flag":  {Yml: quietRun, Args: []string{"deploy", "--quiet"}},
		"short flag": {Yml: quietRun, Args: []string{"deploy", "-q"}},
		"env var":    {Yml: quietRun, Args: []string{"deploy"}, Env: map[string]string{"HOBNOB_QUIET": "1"}},
	} {
		t.Run(name, func(t *testing.T) {
			// Act
			res := Run(t, c)

			// Assert
			res.OK(t)
			res.streamMatches(t, "stdout", res.Stdout, `^deploy \$ echo building \$ENV\nsetup \$ echo installing\nsetup ✓ done after `+secs+`\ndeploy \$ echo visible\ndeploy ✓ done after `+secs+`\n$`)
		})
	}
}

func TestE2E_Log_QuietIsNeverOnByItself(t *testing.T) {
	// given the same run with no flag and with HOBNOB_QUIET set to something
	// other than 1, when it runs, then the normal log shows (why: quiet is
	// opt-in only: not when piped, not for agents)
	// Arrange
	for name, c := range map[string]Case{
		"no switch": {Yml: quietRun, Args: []string{"deploy"}},
		"env zero":  {Yml: quietRun, Args: []string{"deploy"}, Env: map[string]string{"HOBNOB_QUIET": "0"}},
	} {
		t.Run(name, func(t *testing.T) {
			// Act
			res := Run(t, c)

			// Assert
			res.OK(t)
			res.Out(t, "deploy │ visible", "output hidden", "deploy → setup")
		})
	}
}

func TestE2E_Log_QuietRunPrintsAFailedStepsHeldOutputBeforeItsEnd(t *testing.T) {
	// given a failing step after a successful one, when it runs quietly, then
	// only the failing step's output shows, before its ✗ line, and the task
	// ends failed (why: a hidden step must never fail silently)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo fine
		      - run: echo boom; exit 3
	`

	// Act
	res := Yml(t, yml, "t", "-q")

	// Assert
	res.Fails(t)
	res.streamMatches(t, "stdout", res.Stdout, `^t \$ echo fine\nt \$ echo boom; exit 3\nt │ boom\n$`)
	res.streamMatches(t, "stderr", res.Stderr, `^t ✗ `+secs+` echo boom; exit 3: exit status 3\nt ✗ failed after `+secs+`\n$`)
}

func TestE2E_Log_QuietRunPrintsASoftFailedStepsHeldOutputAndCountsIt(t *testing.T) {
	// given a soft: step that fails, when it runs quietly, then its output
	// shows before its ! line and the task end counts it (why: soft failures
	// must stay visible)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo warn; exit 1
		        soft: true
		      - run: echo after
	`

	// Act
	res := Yml(t, yml, "t", "-q")

	// Assert
	res.OK(t)
	res.streamMatches(t, "stdout", res.Stdout, `^t \$ echo warn; exit 1\nt │ warn\nt ! `+secs+` echo warn; exit 1: exit status 1\nt \$ echo after\nt ✓ done after `+secs+` • 1 soft failure\n$`)
}

func TestE2E_Log_QuietRunKeepsTheCtrlCLines(t *testing.T) {
	// given a run interrupted at a prompt, when it runs quietly, then the
	// stopping line and the stopped ends print as in a normal run, but the
	// call line does not (why: ctrl+c lines are as in a normal run)
	// Arrange
	yml := `
		tasks:
		  top:
		    steps:
		      - call: leaf
		  leaf:
		    steps:
		      - get:
		          - NAME:
		              info: Your name
	`

	// Act
	res := Run(t, Case{Yml: yml, Args: []string{"top", "-q"}, TTY: true, Interrupt: "NAME"})

	// Assert
	res.Fails(t)
	res.streamMatches(t, "stdout", res.Stdout, `^leaf ! stopping, ctrl\+c again to force\n$`)
	res.streamMatches(t, "stderr", res.Stderr, `(?m)^leaf ✗ stopped after `+secs+`\ntop ✗ stopped after `+secs+`\n$`)
}
