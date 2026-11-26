package e2e

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"hobnob/internal/web"
)

func TestE2E_Web_TextPromptShowsInfoAndDefaultAndTakesTheAnswer(t *testing.T) {
	// given a text get: with info: and default:, when the page answers it,
	// then the snapshot carries both, the run uses the answer and the rail
	// lists it under the info (why: the page is the person answering)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - NAME:
		              info: What is your name?
		              default: Ada
		      - run: echo hello={{.NAME}}
	`
	var asked, done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(web.ScreenPrompt)
		if status := b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "Grace"}); status != http.StatusNoContent {
			b.Fail("answer status = %d", status)
		}
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ hello=Grace")
	if p := asked.Prompt; p.Kind != web.KindText || p.Info != "What is your name?" || p.Default != "Ada" || p.Secret || p.Optional {
		t.Errorf("prompt = %+v", p)
	}
	if want := []web.RailEntry{{Label: "What is your name?", Value: "Grace"}}; !equalRail(done.Rail.Answers, want) {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
}

func TestE2E_Web_SecretPromptIsHiddenAndMaskedOnTheRailAndInTheLog(t *testing.T) {
	// given a secret: get: with a default, when the page answers it, then the
	// snapshot marks it secret without the default, and the rail and the log
	// show the answer masked (why: a password must not leak into the page)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TOKEN:
		              secret: true
		              default: hunter2default
		      - run: echo using {{.TOKEN}}
	`
	var asked, done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "s3cretvalue"})
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if p := asked.Prompt; !p.Secret || p.Default != "" || !p.HasDefault {
		t.Errorf("prompt = %+v", p)
	}
	if len(done.Rail.Answers) != 1 || done.Rail.Answers[0].Label != "TOKEN" || strings.Contains(done.Rail.Answers[0].Value, "s3cret") || done.Rail.Answers[0].Value == "" {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
	if strings.Contains(done.Result.Log, "s3cretvalue") {
		t.Errorf("log leaks the secret: %s", done.Result.Log)
	}
}

func TestE2E_Web_RefusedAnswerKeepsThePromptWithTheReason(t *testing.T) {
	// given a get: with match: and a shell check:, when the page sends an
	// answer that fails each in turn, then the next snapshot is the same
	// prompt with the reason, and a good answer continues (why: the same
	// validation as the terminal, shown under the field)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - CODE:
		              info: Code
		              match: "ab*"
		              check: '[ "{{.CODE}}" != "abx" ]'
		      - run: echo code={{.CODE}}
	`
	var byMatch, byCheck web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "zzz"})
		byMatch = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" })
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "abx"})
		byCheck = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && strings.Contains(s.Prompt.Refusal, "!=") })
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "abc"})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ code=abc")
	if byMatch.Screen != web.ScreenPrompt || !strings.Contains(byMatch.Prompt.Refusal, "must match ab*") {
		t.Errorf("match refusal = %+v", byMatch.Prompt)
	}
	if byCheck.Screen != web.ScreenPrompt || len(byCheck.Rail.Answers) != 0 {
		t.Errorf("check refusal = %+v", byCheck)
	}
}

func TestE2E_Web_SkipContinuesAnOptionalPromptWithoutAnAnswer(t *testing.T) {
	// given an optional: get:, when the page skips it, then the run goes on
	// with the variable empty and the rail has no answer for it; skipping a
	// required prompt is refused (why: only optional prompts may be skipped)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - NICK:
		              optional: true
		          - NAME: {}
		      - run: echo nick=[{{.NICK}}] name={{.NAME}}
	`
	var optional, required, done web.Snapshot
	var refused int
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		optional = tab.Screen(web.ScreenPrompt)
		b.Post("skip", map[string]any{"id": optional.Prompt.ID})
		required = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.VarName == "NAME" })
		refused = b.Post("skip", map[string]any{"id": required.Prompt.ID})
		b.Post("answer", map[string]any{"id": required.Prompt.ID, "value": "Ada"})
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ nick=[] name=Ada")
	if !optional.Prompt.Optional || refused != http.StatusBadRequest {
		t.Errorf("optional = %+v, skip of a required prompt got %d", optional.Prompt, refused)
	}
	if len(done.Rail.Answers) != 1 || done.Rail.Answers[0].Value != "Ada" {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
}

func TestE2E_Web_AnswerToAPromptThatIsNotOpenIsRefused(t *testing.T) {
	// given a prompt that was answered, when the page answers its old id
	// again, then the POST is refused and the next prompt stays open (why: a
	// double click must not answer the following prompt)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A: {}
		          - B: {}
	`
	var stale int
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		first := tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": first.Prompt.ID, "value": "1"})
		second := tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.VarName == "B" })
		stale = b.Post("answer", map[string]any{"id": first.Prompt.ID, "value": "oops"})
		b.Post("answer", map[string]any{"id": second.Prompt.ID, "value": "2"})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if stale != http.StatusBadRequest {
		t.Errorf("stale answer status = %d", stale)
	}
}

func TestE2E_Web_PresetSkipsItsPrompt(t *testing.T) {
	// given a KEY=VALUE preset for a get:, when the run starts with --web,
	// then no prompt is shown and the rail has no answer for it (why: presets
	// skip their prompts as in the terminal)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - NAME: {}
		      - run: echo name={{.NAME}}
	`
	var done web.Snapshot
	prompted := false
	c := Case{Yml: yml, Args: []string{"--web", "t", "NAME=preset"}, Browser: func(b *Browser) {
		tab := b.Open()
		done = tab.Until(func(s web.Snapshot) bool {
			prompted = prompted || s.Prompt != nil
			return s.Screen == web.ScreenDone
		})
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ name=preset")
	if prompted || len(done.Rail.Answers) != 0 {
		t.Errorf("prompted = %v, answers = %+v", prompted, done.Rail.Answers)
	}
}

const slowCheck = `check: 'touch started; while [ ! -e go ]; do :; done; echo run >> checks; true'`

func TestE2E_Web_SecondAnswerForOnePromptIsRefusedWithoutRunningItsCheck(t *testing.T) {
	// given a prompt whose check: is slow, when the page sends two answers
	// for it at once, then the second is refused as stale without running
	// the check and the run keeps the first answer (why: a double click must
	// not judge a closed prompt)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A:
		              ` + slowCheck + `
		      - run: echo got={{.A}} checks=$(wc -l < checks | tr -d ' ')
	`
	var first, second int
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		var posts sync.WaitGroup
		posts.Add(1)
		go func() {
			defer posts.Done()
			first = b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "one"})
		}()
		b.Wait("started")
		posts.Add(1)
		go func() {
			defer posts.Done()
			second = b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "two"})
		}()
		time.Sleep(100 * time.Millisecond)
		b.Release("go")
		posts.Wait()
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ got=one checks=1")
	if first != http.StatusNoContent || second != http.StatusBadRequest {
		t.Errorf("statuses = %d, %d", first, second)
	}
}

func TestE2E_Web_SkipRacingAnAnswerSettlesTheOptionalPromptOnce(t *testing.T) {
	// given an optional prompt whose check: is slow, when the page answers
	// and then skips it while the check runs, then the answer settles it and
	// the skip is refused (why: a prompt settles exactly once)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A:
		              optional: true
		              ` + slowCheck + `
		      - run: echo got={{.A}}
	`
	var answered, skipped int
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		var posts sync.WaitGroup
		posts.Add(1)
		go func() {
			defer posts.Done()
			answered = b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "one"})
		}()
		b.Wait("started")
		posts.Add(1)
		go func() {
			defer posts.Done()
			skipped = b.Post("skip", map[string]any{"id": asked.Prompt.ID})
		}()
		time.Sleep(100 * time.Millisecond)
		b.Release("go")
		posts.Wait()
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ got=one")
	if answered != http.StatusNoContent || skipped != http.StatusBadRequest {
		t.Errorf("statuses = %d, %d", answered, skipped)
	}
}

func TestE2E_Web_ContinueOnAnUntouchedSecretDefaultUsesItAndSkipGivesNone(t *testing.T) {
	// given an optional secret: get: with a default:, when the page answers
	// with the field left as the masked default, then the default is the
	// answer and never appears in a snapshot, and skipping gives no value
	// (why: the terminal's Enter takes the default too)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TOKEN:
		              secret: true
		              optional: true
		              default: abcdefault
		      - run: if [ -n "{{.TOKEN}}" ]; then echo token=set; else echo token=none; fi
	`
	var used, skippedRun web.Snapshot
	var usedRes, skippedRes *Result
	run := func(act func(b *Browser, asked web.Snapshot)) (*Result, web.Snapshot) {
		var done web.Snapshot
		res := Run(t, Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
			tab := b.Open()
			act(b, tab.Screen(web.ScreenPrompt))
			done = tab.Screen(web.ScreenDone)
		}})
		return res, done
	}

	// Act
	usedRes, used = run(func(b *Browser, asked web.Snapshot) {
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": nil})
	})
	skippedRes, skippedRun = run(func(b *Browser, asked web.Snapshot) {
		b.Post("skip", map[string]any{"id": asked.Prompt.ID})
	})

	// Assert
	usedRes.OK(t)
	usedRes.Out(t, "token=set")
	skippedRes.OK(t)
	skippedRes.Out(t, "token=none")
	if len(used.Rail.Answers) != 1 || len(skippedRun.Rail.Answers) != 0 || strings.Contains(used.Result.Log, "abcdefault") {
		t.Errorf("used = %+v, skipped = %+v", used.Rail, skippedRun.Rail)
	}
}

func TestE2E_Web_EmptyAnswerToARequiredPromptIsRefused(t *testing.T) {
	// given a required get: without a default:, when the page sends an empty
	// answer, then the next snapshot is the same prompt saying an answer is
	// required, and a later answer goes through (why: the person must answer
	// a required prompt)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A: {}
		      - run: echo got={{.A}}
	`
	var refused web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(web.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": ""})
		refused = tab.Until(func(s web.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" })
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "ok"})
		tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ got=ok")
	if refused.Prompt.Refusal != "an answer is required" {
		t.Errorf("refusal = %q", refused.Prompt.Refusal)
	}
}

func TestE2E_Web_SayShowsTheMessageAndContinueGoesOn(t *testing.T) {
	// given a say: naming a variable, when the page shows it and the person
	// continues, then the run goes on and the rail lists no answer for it
	// (why: a message asks nothing, so there is nothing to record)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - say: "Hello {{.WHO}}"
		      - run: echo after
	`
	var asked, done web.Snapshot
	c := Case{Yml: yml, Args: []string{"--web", "t", "WHO=Ada"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(web.ScreenPrompt)
		if status := b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": nil}); status != http.StatusNoContent {
			b.Fail("answer status = %d", status)
		}
		done = tab.Screen(web.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t » Hello Ada")
	res.Out(t, "t │ after")
	if p := asked.Prompt; p.Kind != web.KindSay || p.Info != "Hello Ada" {
		t.Errorf("prompt = %+v", p)
	}
	if len(done.Rail.Answers) != 0 {
		t.Errorf("answers = %+v, want none", done.Rail.Answers)
	}
}
