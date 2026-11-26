package e2e

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"hobnob/internal/browser"
)

func TestE2E_Browser_MenuShowsItsOptionsAndDefaultAndTakesOneChoice(t *testing.T) {
	// given a single-choice menu with a default:, when the page answers with
	// another option, then the snapshot carries the options and default, the
	// run uses the choice and the rail lists it (why: the page is the person
	// choosing)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - COLOR:
		              info: Favourite colour?
		              options: [red, green, blue]
		              default: green
		      - run: echo colour={{.COLOR}}
	`
	var asked, done browser.Snapshot
	c := Case{Yml: yml, Args: []string{"--browser", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(browser.ScreenPrompt)
		if status := b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "blue"}); status != http.StatusNoContent {
			b.Fail("answer status = %d", status)
		}
		done = tab.Screen(browser.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ colour=blue")
	if p := asked.Prompt; p.Kind != browser.KindMenu || p.Multi || !slices.Equal(p.Options, []string{"red", "green", "blue"}) || p.Default != "green" || !p.HasDefault {
		t.Errorf("prompt = %+v", p)
	}
	if want := []browser.RailEntry{{Label: "Favourite colour?", Value: "blue"}}; !equalRail(done.Rail.Answers, want) {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
}

func TestE2E_Browser_MultiMenuHasItsDefaultTickedAndTakesSeveralChoices(t *testing.T) {
	// given a multi: menu with a default: list, when the page answers
	// with two options, then the snapshot lists the ticked defaults and the
	// run gets both choices in the options' order (why: several choices make
	// a list)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - APPS:
		              options: [Slack, Zoom, Notion]
		              multi: true
		              default: '["Zoom"]'
		      - run: echo apps={{.APPS | join ","}}
	`
	var asked, done browser.Snapshot
	c := Case{Yml: yml, Args: []string{"--browser", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked = tab.Screen(browser.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{"Notion", "Slack"}})
		done = tab.Screen(browser.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ apps=Slack,Notion")
	if p := asked.Prompt; !p.Multi || !slices.Equal(p.Defaults, []string{"Zoom"}) {
		t.Errorf("prompt = %+v", p)
	}
	if len(done.Rail.Answers) != 1 || done.Rail.Answers[0].Value != "Slack, Notion" {
		t.Errorf("answers = %+v", done.Rail.Answers)
	}
}

func TestE2E_Browser_MenuChoiceThatFailsItsCheckIsRefusedWithTheReason(t *testing.T) {
	// given a menu with a check:, when the page picks an option that fails it
	// and then one that passes, then the next snapshot is the same prompt with
	// the reason and the second choice goes through (why: the terminal's
	// menu refuses in place too)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SIZE:
		              options: [small, large]
		              check: '[ "{{.SIZE}}" = large ]'
		      - run: echo size={{.SIZE}}
	`
	var refused browser.Snapshot
	c := Case{Yml: yml, Args: []string{"--browser", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(browser.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "small"})
		refused = tab.Until(func(s browser.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" })
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": "large"})
		tab.Screen(browser.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Out(t, "t │ size=large")
	if !strings.Contains(refused.Prompt.Refusal, "= large") || len(refused.Rail.Answers) != 0 {
		t.Errorf("refused = %+v", refused.Prompt)
	}
}

func TestE2E_Browser_MenuRefusesAnOptionItDoesNotOfferAndAnEmptyMultiChoice(t *testing.T) {
	// given a required multi: menu, when the page answers with an option that
	// isn't offered and then with nothing, then each is refused and the
	// prompt stays open (why: only offered options can be answered)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - APPS:
		              options: [Slack, Zoom]
		              multi: true
		      - run: echo ok
	`
	var unknown, empty browser.Snapshot
	c := Case{Yml: yml, Args: []string{"--browser", "t"}, Browser: func(b *Browser) {
		tab := b.Open()
		asked := tab.Screen(browser.ScreenPrompt)
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{"Teams"}})
		unknown = tab.Until(func(s browser.Snapshot) bool { return s.Prompt != nil && s.Prompt.Refusal != "" })
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{}})
		empty = tab.Until(func(s browser.Snapshot) bool {
			return s.Prompt != nil && strings.Contains(s.Prompt.Refusal, "at least one")
		})
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": []string{"Zoom"}})
		tab.Screen(browser.ScreenDone)
	}}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if !strings.Contains(unknown.Prompt.Refusal, "Teams") || empty.Screen != browser.ScreenPrompt {
		t.Errorf("unknown = %+v, empty = %+v", unknown.Prompt, empty.Prompt)
	}
}

func TestE2E_Browser_OptionalMenuCanBeSkippedAndContinueOnItsDefaultTakesIt(t *testing.T) {
	// given an optional menu with a default:, when the page skips it, then
	// the variable is empty, and when it answers null, then the default is
	// the answer (why: Continue on an untouched menu takes the default)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - COLOR:
		              options: [red, green]
		              default: green
		              optional: true
		      - run: echo colour=[{{.COLOR}}]
	`
	run := func(act func(b *Browser, asked browser.Snapshot)) *Result {
		return Run(t, Case{Yml: yml, Args: []string{"--browser", "t"}, Browser: func(b *Browser) {
			tab := b.Open()
			act(b, tab.Screen(browser.ScreenPrompt))
			tab.Screen(browser.ScreenDone)
		}})
	}

	// Act
	skipped := run(func(b *Browser, asked browser.Snapshot) { b.Post("skip", map[string]any{"id": asked.Prompt.ID}) })
	taken := run(func(b *Browser, asked browser.Snapshot) {
		b.Post("answer", map[string]any{"id": asked.Prompt.ID, "value": nil})
	})

	// Assert
	skipped.OK(t)
	skipped.Out(t, "t │ colour=[]")
	taken.OK(t)
	taken.Out(t, "t │ colour=[green]")
}
