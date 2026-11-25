package e2e

import (
	"net/http"
	"strings"
	"testing"

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
