package e2e

import (
	"strings"
	"testing"
)

func TestE2E_Run_QuietSuppressesOutputOnSuccess(t *testing.T) {
	// given a quiet: true run: step that succeeds, when executed, then its
	// command output never reaches stdout — only hobnob's own "output
	// hidden" chrome line does. Uses shell arithmetic so the produced output
	// ("42") is distinct from the displayed command text itself — which
	// quiet: never hides — so the assertion can't pass by accident.
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo $((21+21))
		        quiet: true
	`, "t")
	res.OK(t)
	res.NotOut(t, "42")
	res.Out(t, "output hidden")
}

func TestE2E_Run_QuietWithMessageShowsMessageInsteadOfOutput(t *testing.T) {
	// given quiet: with a message, when the step runs, then the rendered
	// message replaces the suppressed output in the timeline
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - PKG: app
		      - run: echo $((21+21))
		        quiet: Installing {{.PKG}}
	`, "t")
	res.OK(t)
	res.NotOut(t, "42")
	res.Out(t, "Installing app", "output hidden")
}

func TestE2E_Run_QuietReplaysOutputOnFailure(t *testing.T) {
	// given a quiet: true run: step whose command fails, when executed, then
	// its buffered stdout/stderr is replayed in full rather than staying
	// hidden (why: a hidden step must never fail silently)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo out-line
		          echo err-line 1>&2
		          exit 1
		        quiet: true
	`, "t")
	res.Fails(t)
	res.Out(t, "out-line")
	res.Err(t, "exit status 1")
	if !strings.Contains(res.Combined, "err-line") {
		t.Errorf("expected err-line to be replayed, combined output:\n%s", res.Combined)
	}
}

func TestE2E_Run_QuietIntoStillCaptures(t *testing.T) {
	// given a quiet: true run: step with into:, when executed, then into:
	// still captures the command's output even though it's never printed
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo captured-value
		        quiet: true
		        into:
		          - RESULT: stdout | trim
		      - run: echo got={{.RESULT}}
	`, "t")
	res.OK(t)
	res.Lines(t, "got=captured-value")
}

func TestE2E_Run_QuietMessageMasksSecrets(t *testing.T) {
	// given a secret referenced in a quiet: message, when the step runs,
	// then the message is masked same as any other displayed text
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - TOKEN:
		              value: hunter2
		              secret: true
		      - run: echo hi
		        quiet: "Using {{.TOKEN}}"
	`, "t")
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Run_QuietOnCallRejectedAtParseTime(t *testing.T) {
	// given quiet: on a call: step, when parsed, then it's rejected since
	// quiet: only makes sense for a run: step's own output
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: child
		        quiet: true
		  child:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "quiet:", "run:")
}

func TestE2E_Run_QuietMappingRejectedAtParseTime(t *testing.T) {
	// given quiet: given a mapping instead of true/false/a string, when
	// parsed, then it's rejected rather than silently ignored
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo hi
		        quiet: { foo: bar }
	`, "t")
	res.Fails(t)
	res.Err(t, "quiet:")
}
