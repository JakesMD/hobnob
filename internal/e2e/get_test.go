package e2e

import "testing"

// Most of get:'s grammar (default:, check:, options:, multi:, secret:,
// optional:, bare shorthand) is proven fastest and most deterministically
// through --no-input mode: it exercises the same parsing and default/check
// resolution a real prompt would, without needing a driven terminal. The
// smaller set of tests that need TTY:true + Answers cover what --no-input
// mode can't: the prompt actually firing, its ordering, and a check: that
// refuses an answer.

func TestE2E_Get_NoDefaultNoInputFailsFast(t *testing.T) {
	// given a get: with no default and no preset value, when run --no-input,
	// then fails naming the var rather than hanging on a prompt (why: CI has
	// nobody to answer, so it must fail fast and say what's missing)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - NAME:
		              info: Enter your name
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.Fails(t)
	res.Err(t, "--no-input: NAME requires input")
}

func TestE2E_Get_DefaultUsedUnderNoInput(t *testing.T) {
	// given a get: with a literal default:, when run --no-input, then the
	// default is used without prompting (why: --no-input means take the
	// defaults, not fail on every prompt)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - COLOR:
		              info: Enter color
		              default: blue
		      - run: echo color={{.COLOR}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "color=blue")
}

func TestE2E_Get_BareVarDefaultResolvesLikeTemplate(t *testing.T) {
	// given default: .VAR (bare, no {{ }}), when run --no-input, then it
	// resolves exactly as default: "{{.VAR}}" would (why: the bare form is
	// shorthand, not a different meaning)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - FALLBACK: fallback-color
		      - get:
		          - COLOR:
		              info: Enter color
		              default: .FALLBACK
		      - run: echo color={{.COLOR}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "color=fallback-color")
}

func TestE2E_Get_BareVarPipeDefaultResolvesLikeTemplate(t *testing.T) {
	// given default: .VAR | filter (bare, piped), when run --no-input, then
	// it resolves exactly as default: "{{.VAR | filter}}" would (why: the
	// bare form is shorthand, not a different meaning)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - RAW: "  padded  "
		      - get:
		          - ITEM:
		              info: Pick item
		              default: .RAW | trim
		      - run: echo item={{.ITEM}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "item=padded")
}

func TestE2E_Get_CheckValidatesPresetValue(t *testing.T) {
	// given a get: with check: and a preset value, when run --no-input, then
	// the check runs against the preset and fails or passes accordingly
	// (why: a value supplied up front must meet the same rule as a typed
	// one)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - SIZE:
		              check: "[ {{.SIZE}} -le 100 ]"
	`

	// Act
	passing := Yml(t, yml, "t", "--no-input", "SIZE=50")
	failing := Yml(t, yml, "t", "--no-input", "SIZE=200")

	// Assert
	passing.OK(t)
	failing.Fails(t)
	failing.Err(t, "validation failed")
}

func TestE2E_Get_SelectFromListValidatesPresetAgainstOptions(t *testing.T) {
	// given a get: with options: as a literal list, when run --no-input with
	// a preset value not in that list, then it fails naming the bad value
	// (why: a preset bypasses the menu, so it must not smuggle in a value the
	// menu would never offer)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - MOTOR:
		              info: Select motor
		              options: [X-Axis, Y-Axis, Z-Axis]
	`

	// Act
	res := Yml(t, yml, "t", "--no-input", "MOTOR=Q-Axis")

	// Assert
	res.Fails(t)
	res.Err(t, `MOTOR value "Q-Axis" not in options`)
}

func TestE2E_Get_SelectFromBareVarReference(t *testing.T) {
	// given options: .VAR (bare, dynamic option list), when run --no-input,
	// then the referenced var supplies the option list exactly as options:
	// "{{.VAR}}" would (why: the bare form is shorthand, not a different
	// meaning)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - RELEASE_LIST: [v1, v2, v3]
		      - get:
		          - RELEASES:
		              info: Select release
		              options: .RELEASE_LIST
	`

	// Act
	res := Yml(t, yml, "t", "--no-input", "RELEASES=v2")

	// Assert
	res.OK(t)
}

func TestE2E_Get_MultiSelectValidatesEachElement(t *testing.T) {
	// given multi: true with options:, when run --no-input with a preset
	// JSON array, then every element is validated against the option list
	// (why: a preset bypasses the picker, so it must not smuggle in values
	// the picker would never offer)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              info: Pick tags
		              multi: true
		              options: [alpha, beta, gamma]
		      - run: |
		          echo 'tags={{.TAGS}}'
	`

	// Act
	ok := Yml(t, yml, "t", "--no-input", `TAGS=["alpha","gamma"]`)
	bad := Yml(t, yml, "t", "--no-input", `TAGS=["nope"]`)

	// Assert
	ok.OK(t)
	ok.Lines(t, `tags=["alpha","gamma"]`)
	bad.Fails(t)
	bad.Err(t, `TAGS value "nope" not in options`)
}

func TestE2E_Get_MultiPresetFromCLIBecomesAList(t *testing.T) {
	// given multi: true and a JSON array passed on the command line, when a
	// loop: reads the var without | json, then it runs once per element (why:
	// a multi: answer is a list however it arrives, typed or passed in)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              options: [alpha, beta, gamma]
		      - loop: .TAGS
		        steps:
		          - run: echo tag={{.ITEM}}
	`

	// Act
	res := Yml(t, yml, "t", `TAGS=["alpha","gamma"]`)

	// Assert
	res.OK(t)
	res.Lines(t, "tag=alpha", "tag=gamma")
}

func TestE2E_Get_MultiPresetBecomesAListWithPromptsOn(t *testing.T) {
	// given multi: true and a terminal, when the value is passed on the
	// command line, then the var is still a list (why: the options check
	// only runs with prompts off, the parse must not depend on it)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAGS:
			              multi: true
			              options: [x, y]
			      - loop: .TAGS
			        steps:
			          - run: echo tag={{.ITEM}}
		`,
		Args: []string{"t", `TAGS=["x","y"]`},
		TTY:  true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "tag=x", "tag=y")
}

func TestE2E_Get_MultiPresetFromShellJSONTextBecomesAList(t *testing.T) {
	// given multi: true and a JSON list in the shell's environment, which
	// arrives as text, when a loop: reads the var, then it runs once per
	// element (why: multi: declares a list, so its text is parsed without
	// | json, as it was before CLI args became data)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - TAGS:
			              multi: true
			              options: [alpha, beta]
			      - loop: .TAGS
			        steps:
			          - run: echo tag={{.ITEM}}
		`,
		Args: []string{"t", "--no-input"},
		Env:  map[string]string{"TAGS": `["alpha","beta"]`},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "tag=alpha", "tag=beta")
}

func TestE2E_Get_MultiDefaultAsJSONTextBecomesAList(t *testing.T) {
	// given multi: true with a default: written as JSON text, when run
	// --no-input, then the default is used as a list (why: a quoted default
	// worked before multi: answers were always lists, and must keep working)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              options: [alpha, beta]
		              default: '["beta"]'
		      - loop: .TAGS
		        steps:
		          - run: echo tag={{.ITEM}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "tag=beta")
}

func TestE2E_Get_MultiPresetNotAJSONListFails(t *testing.T) {
	// given multi: true, when the value passed in isn't a JSON list, then
	// the step fails showing the expected form, with prompts on or off (why:
	// text isn't quietly turned into a list of one)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              options: [alpha, beta]
	`
	for _, args := range [][]string{
		{"t", "TAGS=alpha,beta"},
		{"t", "--no-input", "TAGS=alpha,beta"},
	} {
		// Act
		res := Run(t, Case{Yml: yml, Args: args, TTY: true})

		// Assert
		res.Fails(t)
		res.Err(t, `TAGS must be a JSON list, like TAGS='["a","b"]'`)
	}
}

func TestE2E_Get_MultiPresetEmptyTextIsEmptyList(t *testing.T) {
	// given multi: true optional: true, when passed an empty value, then the
	// var is an empty list, the same as no answer at all (why: a later
	// loop: over it must simply run zero times)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              optional: true
		              options: [a, b]
		      - run: echo 'tags={{.TAGS}}'
	`

	// Act
	res := Yml(t, yml, "t", "TAGS=")

	// Assert
	res.OK(t)
	res.Lines(t, "tags=[]")
}

func TestE2E_Get_TwoVarsResolveInOrder(t *testing.T) {
	// given two get: entries in one step, when run --no-input, then each
	// resolves independently in declaration order (why: a later entry's
	// default may build on an earlier one's value)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - A:
		              default: aval
		          - B:
		              default: bval
		      - run: echo a={{.A}} b={{.B}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "a=aval b=bval")
}

func TestE2E_Get_BareShorthandNoModifiers(t *testing.T) {
	// given get: [VARNAME] (bare shorthand, no info/default/etc.), when
	// satisfied via a CLI arg, then it resolves like any full-syntax entry
	// (why: the shorthand is just a full entry with no modifiers)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get: [COMMAND]
		      - run: echo cmd={{.COMMAND}}
	`

	// Act
	res := Yml(t, yml, "t", "COMMAND=build")

	// Assert
	res.OK(t)
	res.Lines(t, "cmd=build")
}

func TestE2E_Get_BareShorthandMixedWithFullSyntax(t *testing.T) {
	// given a bare shorthand entry alongside a full-syntax entry in the same
	// get:, when run --no-input, then both resolve correctly (why: the two
	// forms may be mixed freely in one list)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - COMMAND
		          - CHECK:
		              default: pubspec.yaml
		      - run: echo cmd={{.COMMAND}} check={{.CHECK}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input", "COMMAND=build")

	// Assert
	res.OK(t)
	res.Lines(t, "cmd=build check=pubspec.yaml")
}

func TestE2E_Get_OptionalSkipsCheckWhenEmpty(t *testing.T) {
	// given optional: true with a check: and no value provided, when run
	// --no-input, then it succeeds with an empty value rather than running
	// the check against emptiness (why: a check validates a value that was
	// given; skipping an optional one isn't a bad value)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - NOTE:
		              info: Any notes?
		              check: "[ -n {{.NOTE}} ]"
		              optional: true
		      - run: echo note={{.NOTE | default "none"}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "note=none")
}

func TestE2E_Get_OptionalMultiEmptyIsJSONArrayNotNull(t *testing.T) {
	// given multi: true optional: true with no value, when run --no-input,
	// then the var is an empty JSON array (why: a later loop: over it must
	// just do nothing, where null or an empty string would break that
	// consumer)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              info: Pick tags
		              multi: true
		              optional: true
		              options: [a, b]
		      - run: echo before
		      - loop: .TAGS
		        steps:
		          - run: echo tag={{.ITEM}}
		      - run: echo after
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "before", "after")
}

func TestE2E_Get_SecretMasksPromptedValue(t *testing.T) {
	// given secret: true, when its value is echoed by a later step, then the
	// value is masked in that display (why: masking applies even when the
	// value came from a default, not just from typed input)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - PASSWORD:
		              info: Enter password
		              default: hunter2
		              secret: true
		      - run: ": {{.PASSWORD}}"
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Get_InteractiveTextPromptSetsVar(t *testing.T) {
	// given a text get: with a real (faked) terminal prompt, when answered,
	// then the typed value flows into scope (why: proves the interactive
	// path, which --no-input tests never reach)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - NAME:
			              info: Enter your name
			      - run: echo hi-{{.NAME}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"NAME": {"ada"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "hi-ada")
	res.Prompted(t, "NAME")
}

func TestE2E_Get_InteractiveSelectPromptSetsVar(t *testing.T) {
	// given a select get: (options: given) with a real (faked) terminal, when
	// an option is picked, then the picked value flows into scope (why:
	// proves the interactive menu path, which --no-input tests never reach)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - ENV:
			              info: Pick env
			              options: [staging, production]
			      - run: echo env={{.ENV}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"ENV": {"production"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "env=production")
}

func TestE2E_Get_InteractiveSelectRepromptsUntilCheckPasses(t *testing.T) {
	// given a select get: with check:, when the first answer fails the check,
	// then it re-prompts and accepts the next answer that passes (why: the
	// re-prompt loop must not stop at the first failed attempt)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - CHOICE:
			              options: [bad, good]
			              check: '[ "{{.CHOICE}}" = "good" ]'
			      - run: echo choice={{.CHOICE}}
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"CHOICE": {"bad", "good"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "choice=good")
}

func TestE2E_Get_TwoPromptsAskInOrder(t *testing.T) {
	// given two get: entries needing real prompts, when run, then they're
	// asked in declaration order (why: people answer prompts in the order the
	// file reads)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - FIRST:
			              info: First
			          - SECOND:
			              info: Second
		`,
		Args:    []string{"t"},
		Answers: map[string][]string{"FIRST": {"1"}, "SECOND": {"2"}},
		TTY:     true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Prompted(t, "FIRST", "SECOND")
}

func TestE2E_Get_CLIArgSatisfiesPromptWithoutAsking(t *testing.T) {
	// given a get: for a var already supplied on the CLI, when run even on a
	// simulated TTY, then no prompt fires at all (why: a value passed up
	// front must never be asked for again)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - get:
			          - NAME:
			              info: Enter your name
			      - run: echo hi-{{.NAME}}
		`,
		Args: []string{"t", "NAME=cli-value"},
		TTY:  true,
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "hi-cli-value")
	res.Prompted(t /* none */)
}

func TestE2E_Get_OptionalWithDefaultUsesDefaultWithPromptsOff(t *testing.T) {
	// given optional: true entries with a default: (text and menu) and one
	// without, when run --no-input, then the defaults are used and only the
	// entry with none is empty (why: optional means skippable, not that a
	// default the author wrote should be ignored)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - COUNT: { optional: true, default: 3 }
		          - COLOR: { optional: true, default: green, options: [red, green] }
		          - NOTE: { optional: true }
		      - run: echo count={{.COUNT}} color={{.COLOR}} note={{.NOTE | default "none"}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input")

	// Assert
	res.OK(t)
	res.Lines(t, "count=3 color=green note=none")
}
