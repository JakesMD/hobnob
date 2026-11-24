package e2e

import "testing"

func TestE2E_Accessor_MissingPathErrorsNamingThePath(t *testing.T) {
	// given an object missing a key, when accessed with no default, then the
	// run fails naming the exact path that was missing (why: a typo in a path
	// must point at itself, not surface as an empty value later)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":1}'
		        into:
		          - RESP: stdout
		      - run: echo {{.RESP.nope}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "RESP.nope")
}

func TestE2E_Accessor_DefaultCatchesMissingPath(t *testing.T) {
	// given a missing path, when piped through | default, then the fallback
	// is used instead of failing the run (why: default is the one sanctioned
	// way to make a path optional)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '{"a":1}'
		        into:
		          - RESP: stdout
		      - run: echo {{.RESP.nope | default "fallback"}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `{"a":1}`, "fallback")
}

func TestE2E_Accessor_WrongKindNotCaughtByDefault(t *testing.T) {
	// given a plain (uncaptured) string masquerading as JSON, when accessed
	// with | default, then it still fails naming | json (why: default only
	// catches absence, never a wrong-kind access, so a missing | json can't
	// hide behind a fallback)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - STR: '{"a":1}'
		      - run: echo {{.STR.a | default "fallback"}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "json")
}

func TestE2E_Accessor_ArgvStrictness(t *testing.T) {
	// given a missing path spliced into a run: argv element, when the step
	// runs, then it fails before the command executes rather than passing an
	// empty argument (why: an empty argv element can silently widen a
	// destructive command's target)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { bucket: prod }
		      - run: [echo, .CFG.typo]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "CFG.typo")
}

func TestE2E_Accessor_WildcardMapsAndDropsNoMatchElements(t *testing.T) {
	// given a heterogeneous array, when mapped with [*].name, then elements
	// missing "name" are dropped and the result splices into one argv element
	// per item (why: multiplicity is not absence, so one odd element must not
	// fail or hole the whole mapping)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '[{"name":"a"},{"other":1},{"name":"c"}]'
		        into:
		          - ITEMS: stdout
		      - run: [echo, ".ITEMS[*].name"]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `[{"name":"a"},{"other":1},{"name":"c"}]`, "a c")
}

func TestE2E_Accessor_SliceClampsOutOfRangeBound(t *testing.T) {
	// given a 3-element array, when sliced [0:99], then the range clamps to
	// the array's actual length rather than erroring (why: "the first N" must
	// work on lists shorter than N)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - ITEMS: [a, b, c]
		      - run: [echo, ".ITEMS[0:99]"]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "a b c")
}

func TestE2E_Accessor_NegativeIndex(t *testing.T) {
	// given a 3-element array, when indexed with [-1], then the last element
	// is returned (why: negative indexes count from the end, so the last item
	// needs no length arithmetic)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - ITEMS: [a, b, c]
		      - run: echo {{.ITEMS[-1]}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "c")
}

func TestE2E_Accessor_WildcardOnObjectYieldsSortedValues(t *testing.T) {
	// given an object, when accessed with [*], then its values come back
	// ordered by key (why: map iteration order is random, and output must be
	// deterministic)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { b: 2, a: 1 }
		      - run: echo {{.CFG[*]}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "[1,2]")
}

func TestE2E_Accessor_DynamicKeyWithDotsAndSlashes(t *testing.T) {
	// given a key containing "." and "/", when addressed via a dynamic key
	// held in a variable, then the exact key is looked up rather than being
	// parsed as a nested path (why: real-world keys such as Kubernetes labels
	// contain both, and must be reachable without injection)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '{"app.kubernetes.io/name":"myapp"}'
		        into:
		          - JSON: stdout
		      - set:
		          - KEY: app.kubernetes.io/name
		      - run: echo {{.JSON[.KEY]}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `{"app.kubernetes.io/name":"myapp"}`, "myapp")
}

func TestE2E_Accessor_BraceFreeOptionsForm(t *testing.T) {
	// given options: written without {{ }}, when it resolves through an
	// accessor, then the single-element result satisfies the prompt like any
	// other bare-ref options: list (why: brace-free fields must accept
	// accessors, not just bare names)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - PROFILES: [{ name: alice }, { name: bob }]
		      - get:
		          - WHO:
		              info: Select
		              options: .PROFILES[0].name
	`

	// Act
	res := Yml(t, yml, "t", "--no-input", "WHO=alice")

	// Assert
	res.OK(t)
}

func TestE2E_Accessor_BraceFreePrivateVarForm(t *testing.T) {
	// given options: written without {{ }} naming a _-prefixed var, when it
	// resolves, then it reads the var rather than staying a literal string
	// (why: private variables are a task's scratch values, and a prompt's
	// options list is the commonest one)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - _TYPES: [dad, programming]
		      - get:
		          - TYPE:
		              options: ._TYPES
		      - run: echo type={{.TYPE}}
	`

	// Act
	res := Yml(t, yml, "t", "--no-input", "TYPE=dad")

	// Assert
	res.OK(t)
	res.Lines(t, "type=dad")
}

func TestE2E_Accessor_BraceFreeDirForm(t *testing.T) {
	// given dir: written without {{ }}, when it resolves through an accessor,
	// then the task runs in the resolved directory (why: brace-free fields
	// must accept accessors, not just bare names)
	// Arrange
	c := Case{
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
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
}

func TestE2E_Accessor_IntoSourceWithAccessorAndFilterChain(t *testing.T) {
	// given an into: source combining an accessor and a filter chain, when
	// captured, then the accessor is applied first, then the chain (why:
	// filters operate on the selected value, not the whole captured document)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: printf '[{"name":" ada "}]'
		        into:
		          - NAME: stdout[0].name | trim
		      - run: echo name={{.NAME}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `[{"name":" ada "}]`, "name=ada")
}

func TestE2E_Accessor_StringLiteralBracketsSurviveUnchanged(t *testing.T) {
	// given a string literal that looks like an accessor, when rendered, then
	// it stays literal text (why: the accessor rewriter must skip string
	// literals rather than parsing their contents)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: 'echo {{ printf "%s" "a.b[0]" }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "a.b[0]")
}
