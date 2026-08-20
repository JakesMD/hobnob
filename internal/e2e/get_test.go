package e2e

import "testing"

// Most of get:'s grammar (default:, check:, from:, multi:, secret:,
// optional:, bare shorthand) is proven fastest and most deterministically
// through --no-input mode: it exercises the same parsing and default/check
// resolution a real prompt would, without needing a driven terminal. The
// smaller set of tests that need TTY:true + Answers cover what --no-input
// mode can't: the prompt actually firing, its ordering, and the check:
// re-prompt loop.

func TestE2E_Get_NoDefaultNoInputFailsFast(t *testing.T) {
	// given a get: with no default and no preset value, when run --no-input,
	// then fails naming the var rather than hanging on a prompt
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - NAME:
		              info: Enter your name
	`, "t", "--no-input")
	res.Fails(t)
	res.Err(t, "--no-input: NAME requires input")
}

func TestE2E_Get_DefaultUsedUnderNoInput(t *testing.T) {
	// given a get: with a literal default:, when run --no-input, then the
	// default is used without prompting
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - COLOR:
		              info: Enter color
		              default: blue
		      - run: echo color={{.COLOR}}
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "color=blue")
}

func TestE2E_Get_BareVarDefaultResolvesLikeTemplate(t *testing.T) {
	// given default: .VAR (bare, no {{ }}), when run --no-input, then it
	// resolves exactly as default: "{{.VAR}}" would
	res := Yml(t, `
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
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "color=fallback-color")
}

func TestE2E_Get_BareVarPipeDefaultResolvesLikeTemplate(t *testing.T) {
	// given default: .VAR | filter (bare, piped), when run --no-input, then
	// it resolves exactly as default: "{{.VAR | filter}}" would
	res := Yml(t, `
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
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "item=padded")
}

func TestE2E_Get_CheckValidatesPresetValue(t *testing.T) {
	// given a get: with check: and a preset value, when run --no-input, then
	// the check runs against the preset and fails or passes accordingly
	passing := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get: [SIZE]
	`, "t", "--no-input", "SIZE=50")
	passing.OK(t)

	failing := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - SIZE:
		              check: "[ {{.SIZE}} -le 100 ]"
	`, "t", "--no-input", "SIZE=200")
	failing.Fails(t)
	failing.Err(t, "validation failed")
}

func TestE2E_Get_SelectFromListValidatesPresetAgainstOptions(t *testing.T) {
	// given a get: with from: a literal list, when run --no-input with a
	// preset value not in that list, then it fails naming the bad value
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - MOTOR:
		              info: Select motor
		              options: [X-Axis, Y-Axis, Z-Axis]
	`, "t", "--no-input", "MOTOR=Q-Axis")
	res.Fails(t)
	res.Err(t, `MOTOR value "Q-Axis" not in options`)
}

func TestE2E_Get_SelectFromBareVarReference(t *testing.T) {
	// given from: .VAR (bare, dynamic option list), when run --no-input,
	// then the referenced var supplies the option list, wrapped in template
	// syntax exactly as from: "{{.VAR}}" would be
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - RELEASE_LIST: [v1, v2, v3]
		      - get:
		          - RELEASES:
		              info: Select release
		              options: .RELEASE_LIST
	`, "t", "--no-input", "RELEASES=v2")
	res.OK(t)
}

func TestE2E_Get_MultiSelectValidatesEachElement(t *testing.T) {
	// given multi: true with from:, when run --no-input with a preset JSON
	// array, then every element is validated against the option list
	ok := Yml(t, `
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
	`, "t", "--no-input", `TAGS=["alpha","gamma"]`)
	ok.OK(t)
	ok.Lines(t, `tags=["alpha","gamma"]`)

	bad := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - TAGS:
		              multi: true
		              options: [alpha, beta]
	`, "t", "--no-input", `TAGS=["nope"]`)
	bad.Fails(t)
	bad.Err(t, `TAGS value "nope" not in options`)
}

func TestE2E_Get_TwoVarsResolveInOrder(t *testing.T) {
	// given two get: entries in one step, when run --no-input, then each
	// resolves independently in declaration order
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - A:
		              default: aval
		          - B:
		              default: bval
		      - run: echo a={{.A}} b={{.B}}
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "a=aval b=bval")
}

func TestE2E_Get_BareShorthandNoModifiers(t *testing.T) {
	// given get: [VARNAME] (bare shorthand, no info/default/etc.), when
	// satisfied via a CLI arg, then it resolves like any full-syntax entry
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get: [COMMAND]
		      - run: echo cmd={{.COMMAND}}
	`, "t", "COMMAND=build")
	res.OK(t)
	res.Lines(t, "cmd=build")
}

func TestE2E_Get_BareShorthandMixedWithFullSyntax(t *testing.T) {
	// given a bare shorthand entry alongside a full-syntax entry in the same
	// get:, when run --no-input, then both resolve correctly
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - COMMAND
		          - CHECK:
		              default: pubspec.yaml
		      - run: echo cmd={{.COMMAND}} check={{.CHECK}}
	`, "t", "--no-input", "COMMAND=build")
	res.OK(t)
	res.Lines(t, "cmd=build check=pubspec.yaml")
}

func TestE2E_Get_OptionalSkipsCheckWhenEmpty(t *testing.T) {
	// given optional: true with a check: and no value provided, when run
	// --no-input, then it succeeds with an empty value rather than running
	// the check against emptiness
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - NOTE:
		              info: Any notes?
		              check: "[ -n {{.NOTE}} ]"
		              optional: true
		      - run: echo note={{.NOTE | default "none"}}
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "note=none")
}

func TestE2E_Get_OptionalMultiEmptyIsJSONArrayNotNull(t *testing.T) {
	// given multi: true optional: true with no value, when run --no-input,
	// then the var is an empty JSON array so a later loop: over it just does
	// nothing, rather than null or empty string breaking that consumer
	res := Yml(t, `
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
	`, "t", "--no-input")
	res.OK(t)
	res.Lines(t, "before", "after")
}

func TestE2E_Get_SecretMasksPromptedValue(t *testing.T) {
	// given secret: true, when its value is echoed by a later step, then the
	// value is masked in that display (why: masking applies even when the
	// value came from a default, not just from typed input)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - PASSWORD:
		              info: Enter password
		              default: hunter2
		              secret: true
		      - run: ": {{.PASSWORD}}"
	`, "t", "--no-input")
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Get_InteractiveTextPromptSetsVar(t *testing.T) {
	// given a text get: with a real (faked) terminal prompt, when answered,
	// then the typed value flows into scope
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "hi-ada")
	res.Prompted(t, "NAME")
}

func TestE2E_Get_InteractiveSelectPromptSetsVar(t *testing.T) {
	// given a select get: (from: given) with a real (faked) terminal, when
	// an option is picked, then the picked value flows into scope
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "env=production")
}

func TestE2E_Get_InteractiveSelectRepromptsUntilCheckPasses(t *testing.T) {
	// given a select get: with check:, when the first answer fails the check,
	// then it re-prompts and accepts the next answer that passes (why: the
	// re-prompt loop must not stop at the first failed attempt)
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "choice=good")
}

func TestE2E_Get_TwoPromptsAskInOrder(t *testing.T) {
	// given two get: entries needing real prompts, when run, then they're
	// asked in declaration order, not some other order
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Prompted(t, "FIRST", "SECOND")
}

func TestE2E_Get_CLIArgSatisfiesPromptWithoutAsking(t *testing.T) {
	// given a get: for a var already supplied on the CLI, when run even on a
	// simulated TTY, then no prompt fires at all — the CLI arg satisfies it
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "hi-cli-value")
	res.Prompted(t /* none */)
}

