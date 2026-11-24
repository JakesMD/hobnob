package e2e

import (
	"reflect"
	"testing"
)

func TestE2E_Match_MenuHidesNonMatchingOptions(t *testing.T) {
	// given a menu with match:, when prompted, then only matching options are
	// offered (why: match: means the answer must match, so the rest are noise)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAG:
			              options: [v1.0, v2.0, v1.5, main]
			              match: "v1.*"
			      - run: echo tag={{.TAG}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"TAG": {"v1.5"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "tag=v1.5")
	if got, want := res.Prompts[0].Items, []string{"v1.0", "v1.5"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offered items = %v, want %v", got, want)
	}
}

func TestE2E_Match_TemplatedPatternAndBangPattern(t *testing.T) {
	// given a templated pattern and a ! pattern in a list, when prompted, then
	// the pattern is rendered and the ! pattern removes its matches (why:
	// patterns are templates, and ! always wins)
	// Arrange
	c := Case{
		Yml: `
			vars:
			  - EXT: yml
			tasks:
			  t:
			    steps:
			      - get:
			          - F:
			              options: [a.yml, b.json, ci.yml, C.YML]
			              match: ["*.{{.EXT}}", "!ci.*"]
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"F": {"a.yml"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if got, want := res.Prompts[0].Items, []string{"a.yml", "C.YML"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offered items = %v, want %v", got, want)
	}
}

func TestE2E_Match_BareReferenceToList(t *testing.T) {
	// given match: .PATTERNS naming a list var, when prompted, then the list's
	// entries are the patterns (why: parsed exactly like options:)
	// Arrange
	c := Case{
		Yml: `
			vars:
			  - PATTERNS: ["*.go", "*.md"]
			tasks:
			  t:
			    steps:
			      - get:
			          - F:
			              options: [a.go, b.md, c.txt]
			              match: .PATTERNS
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"F": {"b.md"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	if got, want := res.Prompts[0].Items, []string{"a.go", "b.md"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offered items = %v, want %v", got, want)
	}
}

func TestE2E_Match_MenuFilteredToNothingFails(t *testing.T) {
	// given a menu where no option matches, when run, then the step fails
	// naming the variable and the patterns (why: an empty menu can't be
	// answered)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAG:
			              options: [a, b]
			              match: ["v*", "x*"]
		`,
		Args: []string{"t"},
		TTY:  true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.Fails(t)
	res.Err(t, "get TAG: no options match v*, x*")
	res.Prompted(t /* none */)
}

func TestE2E_Match_MenuFilteredToNothingOptionalIsEmpty(t *testing.T) {
	// given the same empty menu with optional: true, when run, then the
	// variable is set empty without asking (why: optional means no answer is
	// fine)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAG:
			              options: [a, b]
			              match: "v*"
			              optional: true
			      - run: echo "tag=[{{.TAG}}]"
		`,
		Args: []string{"t"},
		TTY:  true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "tag=[]")
	res.Prompted(t /* none */)
}

func TestE2E_Match_TextAnswerAskedAgain(t *testing.T) {
	// given a text get: with match:, when the first answer doesn't match,
	// then it is asked again (why: same loop as check:)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - BRANCH:
			              match: "feature/*"
			      - run: echo branch={{.BRANCH}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"BRANCH": {"feature/a/b", "main", "feature/login"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "branch=feature/login")
}

func TestE2E_Match_MultiItemsEachMatch(t *testing.T) {
	// given a multi: menu with match: and a check: over the whole list, when
	// prompted, then only matching items are offered and check: still sees
	// the full answer (why: match: filters items, check: never filters)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAGS:
			              options: [v1, v2, x1]
			              match: "v*"
			              multi: true
			              check: '[ "{{.TAGS | join ","}}" = "v1,v2" ]'
			      - run: echo tags={{.TAGS | join ","}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"TAGS": {"v1", "v1,v2"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "tags=v1,v2")
	if got, want := res.Prompts[0].Items, []string{"v1", "v2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("offered items = %v, want %v", got, want)
	}
}

func TestE2E_Match_MultiPresetItemFails(t *testing.T) {
	// given a multi: value already set with one non-matching item, when run,
	// then the step fails (why: each item must match on its own)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              match: "v*"
	`

	// Act
	res := Yml(t, yml, "t", `TAGS=["v1","x2"]`)

	// Assert
	res.Fails(t)
	res.Err(t, `get TAGS: "x2" doesn't match v*`)
}

func TestE2E_Match_PresetValueMustMatch(t *testing.T) {
	// given a value already set on the command line that doesn't match, when
	// run with prompts on or off, then the step fails (why: a value nobody
	// typed at a prompt still has to satisfy match:)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAG:
		              match: "v*"
	`

	// Act
	off := Yml(t, yml, "t", "TAG=main", "--no-input")
	on := Run(t, Case{Yml: yml, Args: []string{"t", "TAG=main"}, TTY: true})
	ok := Yml(t, yml, "t", "TAG=v1")

	// Assert
	off.Fails(t)
	off.Err(t, `get TAG: "main" doesn't match v*`)
	on.Fails(t)
	on.Err(t, `get TAG: "main" doesn't match v*`)
	on.Prompted(t /* none */)
	ok.OK(t)
}

func TestE2E_Match_DefaultWithPromptsOffMustMatch(t *testing.T) {
	// given a default: that doesn't match, when run --no-input, then the step
	// fails (why: a default is an answer like any other)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAG:
		              default: main
		              match: "v*"
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.Fails(t)
	res.Err(t, `get TAG: "main" doesn't match v*`)
}
