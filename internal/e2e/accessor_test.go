package e2e

import "testing"

// TestE2E_Accessor_* covers DESIGN-PATH.md's accessor syntax end to end —
// the replacement for the pluck filter. See internal/value/path_test.go and
// internal/eval/accessor_test.go for direct unit coverage of the evaluator
// and lexer; these tests exercise the same behavior through a real
// hobnob.yml run, matching this suite's end-to-end style.

func TestE2E_Accessor_MissingPathErrorsNamingThePath(t *testing.T) {
	// given an object missing a key, when accessed with no default, then the
	// run fails naming the exact path that was missing
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":1}'
		        into:
		          - RESP: stdout
		      - run: echo {{.RESP.nope}}
	`, "t")
	res.Fails(t)
	res.Err(t, "RESP.nope")
}

func TestE2E_Accessor_DefaultCatchesMissingPath(t *testing.T) {
	// given a missing path piped through | default, then the fallback is
	// used instead of failing the run
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":1}'
		        into:
		          - RESP: stdout
		      - run: echo {{.RESP.nope | default "fallback"}}
	`, "t")
	res.OK(t)
	res.Lines(t, `{"a":1}`, "fallback")
}

func TestE2E_Accessor_WrongKindNotCaughtByDefault(t *testing.T) {
	// given a plain (uncaptured) string masquerading as JSON, when accessed
	// with | default, then it still fails naming | json — default only
	// catches absence, never a wrong-kind access
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - STR: '{"a":1}'
		      - run: echo {{.STR.a | default "fallback"}}
	`, "t")
	res.Fails(t)
	res.Err(t, "json")
}

func TestE2E_Accessor_ArgvStrictness(t *testing.T) {
	// given a missing path spliced into a run: argv element, when the step
	// runs, then it fails before the command executes rather than passing an
	// empty argument (the motivating aws s3 rm example in DESIGN-PATH.md)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { bucket: prod }
		      - run: [echo, .CFG.typo]
	`, "t")
	res.Fails(t)
	res.Err(t, "CFG.typo")
}

func TestE2E_Accessor_WildcardMapsAndDropsNoMatchElements(t *testing.T) {
	// given a heterogeneous array, when mapped with [*].name, then elements
	// missing "name" are dropped rather than erroring or leaving a hole —
	// and the resulting Array splices into one argv element per item (argv
	// form, so no shell quoting is involved)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '[{"name":"a"},{"other":1},{"name":"c"}]'
		        into:
		          - ITEMS: stdout
		      - run: [echo, ".ITEMS[*].name"]
	`, "t")
	res.OK(t)
	res.Lines(t, `[{"name":"a"},{"other":1},{"name":"c"}]`, "a c")
}

func TestE2E_Accessor_SliceClampsOutOfRangeBound(t *testing.T) {
	// given a 3-element array, when sliced [0:99], then the range clamps to
	// the array's actual length rather than erroring
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - ITEMS: [a, b, c]
		      - run: [echo, ".ITEMS[0:99]"]
	`, "t")
	res.OK(t)
	res.Lines(t, "a b c")
}

func TestE2E_Accessor_NegativeIndex(t *testing.T) {
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - ITEMS: [a, b, c]
		      - run: echo {{.ITEMS[-1]}}
	`, "t")
	res.OK(t)
	res.Lines(t, "c")
}

func TestE2E_Accessor_WildcardOnObjectYieldsSortedValues(t *testing.T) {
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { b: 2, a: 1 }
		      - run: echo {{.CFG[*]}}
	`, "t")
	res.OK(t)
	res.Lines(t, "[1,2]")
}

func TestE2E_Accessor_DynamicKeyWithDotsAndSlashes(t *testing.T) {
	// given a key containing "." and "/" — the injection-prone case
	// DESIGN-PATH.md motivates the whole feature with — when addressed via a
	// dynamic key held in a variable, then the exact key is looked up rather
	// than being parsed as a nested path
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"app.kubernetes.io/name":"myapp"}'
		        into:
		          - JSON: stdout
		      - set:
		          - KEY: app.kubernetes.io/name
		      - run: echo {{.JSON[.KEY]}}
	`, "t")
	res.OK(t)
	res.Lines(t, `{"app.kubernetes.io/name":"myapp"}`, "myapp")
}

func TestE2E_Accessor_BraceFreeOptionsForm(t *testing.T) {
	// given options: written without {{ }}, when it resolves through an
	// accessor, then the single-element result satisfies the prompt just
	// like any other bare-ref options: list
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - PROFILES: [{ name: alice }, { name: bob }]
		      - get:
		          - WHO:
		              info: Select
		              options: .PROFILES[0].name
	`, "t", "--no-input", "WHO=alice")
	res.OK(t)
}

func TestE2E_Accessor_BraceFreeDirForm(t *testing.T) {
	// given dir: written without {{ }}, when it resolves through an
	// accessor, then the task runs in the resolved directory
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - set:
				          - CFG: { path: sub }
				      - run: pwd
				        dir: .CFG.path
			`,
			"sub/.keep": "",
		},
		Args: []string{"t"},
	})
	res.OK(t)
}

func TestE2E_Accessor_IntoSourceWithAccessorAndFilterChain(t *testing.T) {
	// given an into: source combining an accessor and a filter chain, when
	// captured, then the accessor is applied first, then the chain
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '[{"name":" ada "}]'
		        into:
		          - NAME: stdout[0].name | trim
		      - run: echo name={{.NAME}}
	`, "t")
	res.OK(t)
	res.Lines(t, `[{"name":" ada "}]`, "name=ada")
}

func TestE2E_Accessor_StringLiteralBracketsSurviveUnchanged(t *testing.T) {
	// given a string literal that looks like an accessor, when rendered,
	// then it stays literal text — the lexer must skip string literals
	// rather than parsing their contents
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ printf "%s" "a.b[0]" }}'
	`, "t")
	res.OK(t)
	res.Lines(t, "a.b[0]")
}
