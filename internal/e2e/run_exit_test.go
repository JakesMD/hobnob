package e2e

import (
	"strings"
	"testing"
)

func TestE2E_RunExit_CapturesZeroOnSuccess(t *testing.T) {
	// given a succeeding run: step, when into: captures exit, then CODE is 0
	// (why: success must be capturable too, not only failure)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: 'true'
		        into:
		          - CODE: exit
		      - run: echo code={{.CODE}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "code=0")
}

func TestE2E_RunExit_CapturedOnSoftFailureAndComparesNumerically(t *testing.T) {
	// given soft: true and a step exiting 3, when into: captures exit, then
	// CODE is 3 and a later step's numeric if: (ne .CODE 0) fires (why:
	// proves both capture-on-failure and that exit is a typed number, not
	// text)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: exit 3
		        soft: true
		        into:
		          - CODE: exit
		      - run: echo rollback
		        if: '{{ ne .CODE 0 }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "rollback")
}

func TestE2E_RunExit_StderrAlsoCapturedOnSoftFailure(t *testing.T) {
	// given soft: true and a failing step also capturing stderr, when it
	// fails, then the stderr text is captured too (why: regression; into:
	// used to be skipped entirely on any failing step, not just exit)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo oops 1>&2
		          exit 1
		        soft: true
		        into:
		          - CODE: exit
		          - LOG: stderr
		      - run: echo "code={{.CODE}} log={{.LOG | trim}}"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	if !strings.Contains(res.Stderr, "oops") {
		t.Fatalf("expected stderr to contain %q\nstderr:\n%s", "oops", res.Stderr)
	}
	res.Lines(t, "code=1 log=oops")
}

func TestE2E_RunExit_DoesNotDisarmFailureWithoutSoft(t *testing.T) {
	// given a failing run: step capturing exit but no soft:, when executed,
	// then the run still fails (why: exit: capture alone must not silently
	// continue the timeline)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: exit 1
		        into:
		          - CODE: exit
		      - run: echo "should not run"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.NotOut(t, "should not run")
}

func TestE2E_RunExit_NoCodeInventedWhenBinaryMissing(t *testing.T) {
	// given a run: step (argv form, no shell) naming a binary that doesn't
	// exist, when executed, then the run errors and no exit code is
	// fabricated (why: a Start() failure never reaches a process, so there is
	// no exit status to report)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: [this-binary-does-not-exist-anywhere]
		        soft: true
		        into:
		          - CODE: exit
		      - run: echo code={{.CODE | default "unset"}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "code=unset")
}

func TestE2E_RunExit_UnknownIntoSourceNamesAllThree(t *testing.T) {
	// given an into: source that isn't stdout/stderr/exit, when parsed at
	// runtime, then the error names all three valid sources (why: the error
	// should tell the author how to fix it)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: 'true'
		        into:
		          - X: exitcode
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "stdout, stderr or exit")
}
