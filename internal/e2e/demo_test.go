package e2e

import (
	"strings"
	"testing"
)

func TestE2E_Demo_FlagListsBuiltInTasks(t *testing.T) {
	// given --demo, when --list runs, then the built-in demo's tasks are
	// listed instead of anything discovered on disk
	res := Run(t, Case{Files: Files{}, Args: []string{"--demo", "--list"}, Discover: true})
	res.OK(t)
	res.Out(t, "tell-joke")
}

func TestE2E_Demo_FlagSaysItIsTheDemo(t *testing.T) {
	// given --demo, when it loads, then it says so and points at the guide
	// (why: --demo is often the first hobnob command anyone runs, and the
	// next thing they need is how to write their own)
	res := Run(t, Case{Files: Files{}, Args: []string{"--demo", "--list"}, Discover: true})
	res.OK(t)
	// Not res.Err: the notice accompanies a successful run, and Err asserts
	// the run failed. It goes to stderr so it can't contaminate a piped
	// task's output.
	for _, sub := range []string{"Running the built-in demo", "GUIDE.md"} {
		if !strings.Contains(res.Stderr, sub) {
			t.Errorf("expected stderr to contain %q\nstderr:\n%s", sub, res.Stderr)
		}
	}
}

func TestE2E_Demo_FlagBeatsARealTaskfile(t *testing.T) {
	// given --demo in a directory that does have a hobnob.yml, when run, then
	// the demo wins (why: an explicit source flag isn't a fallback — it says
	// which file to use, same as --file)
	res := Run(t, Case{Yml: `
		tasks:
		  build:
		    steps:
		      - run: echo building
	`, Args: []string{"--demo", "--list"}, Discover: true})
	res.OK(t)
	res.Out(t, "tell-joke")
	res.NotOut(t, "build")
}

func TestE2E_Demo_FlagRejectsCombinationWithFile(t *testing.T) {
	// given both --demo and --file, when run, then it errors rather than
	// silently picking one (why: they're alternative answers to the same
	// question, so a caller passing both means something hobnob can't know)
	res := Run(t, Case{Files: Files{}, Args: []string{"--demo", "--file", "x.yml", "--list"}, Discover: true})
	res.Fails(t)
	res.Err(t, "pass only one")
}

func TestE2E_Demo_NotReachableWithoutTheFlag(t *testing.T) {
	// given no hobnob.yml anywhere and no --demo, when a demo task name is
	// run, then discovery still fails (why: the demo is opt-in — someone who
	// doesn't know hobnob isn't helped by a task appearing from nowhere)
	res := Run(t, Case{Files: Files{}, Args: []string{"tell-joke"}, Discover: true})
	res.Fails(t)
	res.Err(t, "no hobnob.yml or hobnob.yaml found")
	assertNoDemoNotice(t, res)
}

func TestE2E_Demo_BareInvocationStillShowsUsage(t *testing.T) {
	// given no hobnob.yml and no arguments, when run, then usage prints as it
	// always did — the demo does not stand in for it
	res := Run(t, Case{Files: Files{}, Discover: true})
	res.OK(t)
	res.Out(t, "Usage:")
	res.NotOut(t, "tell-joke")
	assertNoDemoNotice(t, res)
}

// assertNoDemoNotice fails if the run announced the built-in demo anywhere.
// It checks Combined rather than Stdout because the notice is written to
// stderr — a NotOut on stdout alone would pass no matter what. The match is
// on the notice's full opening rather than "built-in demo", which also
// appears in --demo's own line in the usage text.
func assertNoDemoNotice(t *testing.T, res *Result) {
	t.Helper()
	if strings.Contains(res.Combined, "Running the built-in demo") {
		t.Errorf("expected no demo fallback, got:\n%s", res.Combined)
	}
}
