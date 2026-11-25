package e2e

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"hobnob/internal/web"
)

const waitForGo = `while [ ! -e go ]; do :; done`

func TestE2E_Web_TaskWithoutPromptsEndsOnDone(t *testing.T) {
	// given a task with no prompts, when it runs with --web, then the page is
	// opened on 127.0.0.1 and ends on Done with the task and its duration,
	// the URL is printed and the exit code is 0 (why: browser mode is the
	// same run behind a page)
	// Arrange
	yml := `
		tasks:
		  deploy:
		    info: Deploy the app
		    steps:
		      - run: echo hello
	`
	var done web.Snapshot
	var address string
	c := Case{Yml: yml, Args: []string{"--web", "deploy"}, Browser: func(b *Browser) {
		address = b.Address()
		done = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "http://127.0.0.1:", "deploy │ hello")
	if res.URL != "http://"+address+"/?token="+strings.SplitN(res.URL, "token=", 2)[1] {
		t.Errorf("URL %q is not on %s", res.URL, address)
	}
	if done.Task.Name != "deploy" || done.Task.Info != "Deploy the app" || done.Result.Outcome != web.OutcomeDone || done.Result.DurationMS < 0 {
		t.Errorf("done snapshot = %+v", done)
	}
}

func TestE2E_Web_PageOpenedAfterTheRunEndedStillGetsTheOutcome(t *testing.T) {
	// given a run that ended before any page connected, when a page opens
	// late, then it gets Done and the run only returns after that (why: a
	// slow browser must not miss how the run went)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: touch ran
	`
	var done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		b.WaitLate("ran")
		done = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if done.Result == nil || done.Result.Outcome != web.OutcomeDone {
		t.Errorf("late page saw %+v", done)
	}
}

func TestE2E_Web_FileFlagRunsTheNamedTaskfile(t *testing.T) {
	// given --file beside --web, when run, then that taskfile's task runs
	// (why: --web combines with --file)
	// Arrange
	c := Case{
		Files:    Files{"other.yml": "tasks:\n  t:\n    steps:\n      - run: echo from-other\n"},
		Args:     []string{"--file", "other.yml", "--web", "t"},
		Discover: true,
		Browser:  func(b *Browser) { b.Open().Screen(web.ScreenDone) },
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ from-other")
}

func TestE2E_Web_ProgressShowsTheLabelTheLastLineAndTheRail(t *testing.T) {
	// given a task that calls another, when it runs, then the page shows the
	// innermost task's info (its name without one), the last output line and
	// a rail with a setup entry per task entered, ticked as each finishes
	// (why: the person watches setup progress, never the commands)
	// Arrange
	yml := `
		tasks:
		  top:
		    info: Set up the laptop
		    steps:
		      - call: inner
		      - run: echo after
		  inner:
		    steps:
		      - run: echo "working on it"; ` + waitForGo + `
	`
	var running, done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "top"}, Browser: func(b *Browser) {
		tab := b.Open()
		running = tab.Until(func(s web.Snapshot) bool {
			return s.Progress != nil && s.Progress.LastLine == "working on it"
		})
		b.Release("go")
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	wantRunning := []web.RailEntry{{Label: "Set up the laptop", State: web.StateRunning}, {Label: "inner", State: web.StateRunning}}
	if running.Progress.Label != "inner" || !equalRail(running.Rail.Setup, wantRunning) {
		t.Errorf("running snapshot = %+v", running)
	}
	wantDone := []web.RailEntry{{Label: "Set up the laptop", State: web.StateDone}, {Label: "inner", State: web.StateDone}}
	if !equalRail(done.Rail.Setup, wantDone) {
		t.Errorf("done rail = %+v", done.Rail)
	}
}

func equalRail(got, want []web.RailEntry) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestE2E_Web_HeldStepShowsNoLastLineAndASecretIsMasked(t *testing.T) {
	// given a quiet: step and a step printing a secret, when they run, then
	// the first shows an empty last line and the log shows the secret masked
	// (why: a held step's output stays out of sight, and a secret never
	// reaches the page)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - TOKEN:
		              value: hunter2
		              secret: true
		      - run: echo hidden; i=0; while [ $i -lt 50000 ]; do i=$((i+1)); done; touch printed; ` + waitForGo + `
		        quiet: true
		      - run: echo "token is {{.TOKEN}}"
	`
	var held, after web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		b.Wait("printed")
		held = b.Open().Until(func(s web.Snapshot) bool { return s.Progress != nil })
		b.Release("go")
		after = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if held.Progress.LastLine != "" {
		t.Errorf("held step showed %q", held.Progress.LastLine)
	}
	if !strings.Contains(after.Result.Log, "token is ****") || strings.Contains(after.Result.Log, "hunter2") {
		t.Errorf("log = %q", after.Result.Log)
	}
}

func TestE2E_Web_QuietChangesOnlyTheTerminal(t *testing.T) {
	// given a run with --quiet, when it ends, then the terminal holds only the
	// command line, while the page still saw the output line (why: --quiet is
	// for the terminal, the page is the person)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo chatter
	`
	var done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "--quiet", "t"}, Browser: func(b *Browser) {
		done = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.NotOut(t, "t │ chatter")
	if !strings.Contains(done.Result.Log, "t │ chatter") {
		t.Errorf("log = %q", done.Result.Log)
	}
}

func TestE2E_Web_FailedNamesTheStepAndExitsLikeTheTerminalRun(t *testing.T) {
	// given a task calling a task whose step fails, when it runs with --web,
	// then the page ends on Failed naming the inner task of the step and its
	// last line, the rail marks it failed and the exit code is 1 (why: the
	// person must see what broke, and scripts must see the same exit code)
	// Arrange
	yml := `
		tasks:
		  t:
		    info: Do everything
		    steps:
		      - call: inner
		  inner:
		    info: Install tools
		    steps:
		      - run: echo "disk is full"; exit 3
	`
	var failed web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		failed = b.Open().Screen(web.ScreenFailed)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	if failed.Result.Outcome != web.OutcomeFailed || failed.Result.Label != "Install tools" || failed.Result.LastLine != "disk is full" {
		t.Errorf("failed result = %+v", failed.Result)
	}
	if len(failed.Rail.Setup) != 2 || failed.Rail.Setup[1].State != web.StateFailed {
		t.Errorf("rail = %+v", failed.Rail)
	}
}

func TestE2E_Web_RequestsNeedTheToken(t *testing.T) {
	// given an open page, when a request has no token or a wrong one, then it
	// is refused, and with the right one it is served (why: other local
	// programs and web pages must not read the run or drive it)
	// Arrange
	var statuses []int
	c := Case{Yml: "tasks:\n  t:\n    steps:\n      - run: " + waitForGo + "\n", Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		for _, token := range []string{"", "wrong", b.Token()} {
			status, _ := b.Get("/", token)
			statuses = append(statuses, status)
			if token == b.Token() {
				continue
			}
			if status, _ = b.Get("/events", token); status != http.StatusForbidden {
				b.Fail("events answered %d without the right token", status)
			}
		}
		tab := b.Open()
		b.Release("go")
		tab.Screen(web.ScreenDone)
	}}

	// Act
	Run(t, c)

	// Assert
	if want := []int{403, 403, 200}; !equalInts(statuses, want) {
		t.Errorf("statuses = %v, want %v", statuses, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestE2E_Web_FlagsThatCannotCombine(t *testing.T) {
	// given --web beside --no-input, --list, --select or --help, when run,
	// then it is refused and no page opens (why: the page has nothing to do
	// there)
	// Arrange
	for name, args := range map[string][]string{
		"no-input": {"--web", "t", "--no-input"},
		"list":     {"--web", "--list"},
		"select":   {"--web", "--select"},
		"help":     {"--web", "--help"},
	} {
		t.Run(name, func(t *testing.T) {
			// Act
			res := Yml(t, "tasks:\n  t:\n    steps:\n      - run: echo hi\n", args...)

			// Assert
			res.Fails(t)
			res.Err(t, "--web")
			if res.URL != "" {
				t.Errorf("opened %s", res.URL)
			}
		})
	}
}

func TestE2E_Web_UnknownTaskFailsLikeTheTerminalRunWithoutOpeningAPage(t *testing.T) {
	// given a task name the taskfile lacks, when run with --web, then it
	// fails as the terminal run does and no page opens (why: a page showing a
	// failed run of nothing is noise)
	// Arrange
	yml := "tasks:\n  t:\n    steps:\n      - run: echo hi\n"

	// Act
	res := Yml(t, yml, "--web", "nosuch")

	// Assert
	res.Fails(t)
	res.Err(t, `no task "nosuch"`)
	if res.URL != "" {
		t.Errorf("opened %s", res.URL)
	}
}

func TestE2E_Web_FailedNamesTheFailureThatEndedTheRunNotAForgivenOne(t *testing.T) {
	// given a soft call: that fails and a later step that fails hard, when it
	// runs, then Failed names the task of the later step and its last line
	// (why: a forgiven failure did not end the run)
	// Arrange
	yml := `
		tasks:
		  t:
		    info: Outer
		    steps:
		      - call: inner
		        soft: true
		      - run: echo "the real one"; exit 1
		  inner:
		    info: Inner
		    steps:
		      - run: echo "forgiven"; exit 2
	`
	var failed web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		failed = b.Open().Screen(web.ScreenFailed)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	if failed.Result.Label != "Outer" || failed.Result.LastLine != "the real one" {
		t.Errorf("failed result = %+v", failed.Result)
	}
}

func TestE2E_Web_OutputWithoutATrailingNewlineIsTheLastLineAndInTheLog(t *testing.T) {
	// given a step whose final output has no trailing newline, when it fails,
	// then that text is the last line on the page and in the log (why: the
	// last thing a tool said is the clue, newline or not)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf "no newline at end"; exit 4
	`
	var failed web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		failed = b.Open().Screen(web.ScreenFailed)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	if failed.Result.LastLine != "no newline at end" || !strings.Contains(failed.Result.Log, "t │ no newline at end") {
		t.Errorf("failed result = %+v", failed.Result)
	}
}

func TestE2E_Web_LastLineHasNoColourCodes(t *testing.T) {
	// given a step printing coloured output, when it runs, then the page's
	// last line and the log hold the text without escape sequences (why: the
	// page shows text, not terminal codes)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '\033[31mred alert\033[0m\n'; ` + waitForGo + `
	`
	var running, done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		running = b.Open().Until(func(s web.Snapshot) bool {
			return s.Progress != nil && s.Progress.LastLine != ""
		})
		b.Release("go")
		done = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if running.Progress.LastLine != "red alert" || strings.Contains(done.Result.Log, "\x1b") {
		t.Errorf("last line = %q, log = %q", running.Progress.LastLine, done.Result.Log)
	}
}

func TestE2E_Web_ProgressSnapshotsCarryElapsedTime(t *testing.T) {
	// given a running step, when a page connects some time into the run, then
	// its snapshot carries the time elapsed since the run began (why: the
	// page's clock starts from it, also after a reconnect)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo started; ` + waitForGo + `
	`
	var later web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		b.Open().Until(func(s web.Snapshot) bool {
			return s.Progress != nil && s.Progress.LastLine == "started"
		})
		time.Sleep(150 * time.Millisecond)
		later = b.Open().Until(func(s web.Snapshot) bool { return s.Progress != nil })
		b.Release("go")
		b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if later.Progress.ElapsedMS < 150 {
		t.Errorf("elapsed = %dms", later.Progress.ElapsedMS)
	}
}

func TestE2E_Web_RequestsNeedTheLoopbackHostAndAMatchingOrigin(t *testing.T) {
	// given an open page and the right token, when a request has another
	// Host or another Origin, then it is refused, and with the page's own
	// Host and Origin, or no Origin, it is served (why: a web page on
	// another site must not reach the run, by rebinding or cross-site)
	// Arrange
	statuses := map[string]int{}
	c := Case{Yml: "tasks:\n  t:\n    steps:\n      - run: " + waitForGo + "\n", Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		own := "http://" + b.Address()
		statuses["no origin"] = b.GetWith("/", nil)
		statuses["own origin"] = b.GetWith("/", map[string]string{"Origin": own})
		statuses["wrong host"] = b.GetWith("/", map[string]string{"Host": "example.com"})
		statuses["wrong port"] = b.GetWith("/", map[string]string{"Host": "127.0.0.1:1"})
		statuses["wrong origin"] = b.GetWith("/", map[string]string{"Origin": "http://example.com"})
		statuses["null origin"] = b.GetWith("/", map[string]string{"Origin": "null"})
		tab := b.Open()
		b.Release("go")
		tab.Screen(web.ScreenDone)
	}}

	// Act
	Run(t, c)

	// Assert
	want := map[string]int{"no origin": 200, "own origin": 200, "wrong host": 403, "wrong port": 403, "wrong origin": 403, "null origin": 403}
	for name, status := range want {
		if statuses[name] != status {
			t.Errorf("%s: status %d, want %d", name, statuses[name], status)
		}
	}
}

const pickerYml = `
	tasks:
	  default:
	    info: The default
	    steps:
	      - run: echo ran-default
	  setup:
	    info: Set up the laptop
	    steps:
	      - run: echo ran-setup
	  _private:
	    steps:
	      - run: echo no
	  broken:
	    steps:
	      - run: exit 3
`

func TestE2E_Web_WithoutATaskThePickerListsTheVisibleTasksEvenWithADefault(t *testing.T) {
	// given a taskfile with a default task and a _-prefixed task, when run with --web and no task, then the picker lists the visible
	// tasks with their info and name, and the default does not run, and the
	// exit code is 0 as nothing ran (why: one shortcut offers several wizards)
	// Arrange
	var picker web.Snapshot
	c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(b *Browser) {
		picker = b.Open().Screen(web.ScreenTasks)
		b.Interrupt()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.NotOut(t, "ran-default")
	want := []web.Task{{Name: "broken"}, {Name: "default", Info: "The default"}, {Name: "setup", Info: "Set up the laptop"}}
	if !equalTasks(picker.Tasks, want) {
		t.Errorf("picker tasks = %+v, want %+v", picker.Tasks, want)
	}
}

func equalTasks(got, want []web.Task) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestE2E_Web_ChoosingATaskRunsItAndHomeLeadsToAFreshSecondRun(t *testing.T) {
	// given the picker, when a task is chosen, then it runs and ends on Done
	// with Home, and Home returns to the picker, where a second task starts
	// with a fresh rail and log (why: one page runs several tasks in a row)
	// Arrange
	var first, second web.Snapshot
	var tab *Tab
	c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(b *Browser) {
		tab = b.Open()
		tab.Screen(web.ScreenTasks)
		b.Post("choose", map[string]string{"task": "setup"})
		first = tab.Screen(web.ScreenDone)
		b.Post("home", nil)
		tab.Screen(web.ScreenTasks)
		b.Post("choose", map[string]string{"task": "default"})
		second = tab.Screen(web.ScreenDone)
		b.Interrupt()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "setup │ ran-setup", "default │ ran-default")
	if !first.Result.Home || first.Task.Name != "setup" || !strings.Contains(first.Result.Log, "ran-setup") {
		t.Errorf("first run = %+v", first)
	}
	wantRail := []web.RailEntry{{Label: "The default", State: web.StateDone}}
	if second.Task.Name != "default" || !equalRail(second.Rail.Setup, wantRail) || strings.Contains(second.Result.Log, "ran-setup") {
		t.Errorf("second run = %+v", second)
	}
}

func TestE2E_Web_TheExitCodeIsTheLastRunsOutcome(t *testing.T) {
	// given a failed run followed by a successful one, and the reverse, when
	// the terminal is interrupted on the last Done or Failed page, then the
	// exit code is the last run's (why: it reports like hobnob <task> does)
	// Arrange
	for name, order := range map[string][]string{
		"ok last":     {"broken", "setup"},
		"failed last": {"setup", "broken"},
	} {
		t.Run(name, func(t *testing.T) {
			c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(b *Browser) {
				tab := b.Open()
				for i, task := range order {
					tab.Screen(web.ScreenTasks)
					b.Post("choose", map[string]string{"task": task})
					tab.Until(func(s web.Snapshot) bool { return s.Result != nil })
					if i == 0 {
						b.Post("home", nil)
					}
				}
				b.Interrupt()
			}}

			// Act
			res := Run(t, c)

			// Assert
			if failed := order[len(order)-1] == "broken"; failed != (res.ExitCode != 0) {
				t.Errorf("exit code %d after %v", res.ExitCode, order)
			}
		})
	}
}

func TestE2E_Web_HomeIsOnlyOfferedWhenHobnobHadNoTask(t *testing.T) {
	// given a task named on the command line, when it ends, then Done offers
	// no Home (why: a task-launched wizard ends at Done)
	// Arrange
	var done web.Snapshot
	c := Case{Yml: pickerYml, Args: []string{"--web", "setup"}, Browser: func(b *Browser) {
		done = b.Open().Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if done.Result.Home {
		t.Error("Done offered Home")
	}
}

func TestE2E_Web_PickerRefusesATaskItDidNotOffer(t *testing.T) {
	// given the picker, when a _-prefixed task is chosen, then the request is
	// refused and the picker still works (why: hidden tasks are not runnable
	// from the page)
	// Arrange
	var status int
	var after web.Snapshot
	c := Case{Yml: pickerYml, Args: []string{"--web"}, Browser: func(b *Browser) {
		tab := b.Open()
		tab.Screen(web.ScreenTasks)
		status = b.Post("choose", map[string]string{"task": "_private"})
		b.Post("choose", map[string]string{"task": "setup"})
		after = tab.Screen(web.ScreenDone)
		b.Interrupt()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if status != http.StatusBadRequest || after.Task.Name != "setup" {
		t.Errorf("status %d, then %+v", status, after)
	}
}

func TestE2E_Web_DemoWithoutATaskOpensThePicker(t *testing.T) {
	// given --demo beside --web and no task, when run, then the picker lists
	// the demo's tasks (why: --demo --web works)
	// Arrange
	var picker web.Snapshot
	c := Case{Args: []string{"--demo", "--web"}, Discover: true, Browser: func(b *Browser) {
		picker = b.Open().Screen(web.ScreenTasks)
		b.Interrupt()
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if len(picker.Tasks) == 0 || picker.Tasks[0].Name != "ask-trivia" {
		t.Errorf("picker tasks = %+v", picker.Tasks)
	}
}
