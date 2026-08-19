package e2e

import "testing"

func TestE2E_Loop_ListForm(t *testing.T) {
	// given loop: over a literal list, when executed, then it iterates
	// binding each item to ITEM (why: list form uses ITEM as its default
	// iterator name)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop: [alpha, beta, gamma]
		        steps:
		          - run: echo item={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "item=alpha", "item=beta", "item=gamma")
}

func TestE2E_Loop_BareVarTarget(t *testing.T) {
	// given loop: .VAR (bare, no {{ }}) where VAR is a real typed Array, when
	// executed, then it iterates the array's elements directly — no
	// re-parsing from text, since the array was already typed from a set:
	// literal
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - FILES: [x, y, z]
		      - loop: .FILES
		        steps:
		          - run: echo item={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "item=x", "item=y", "item=z")
}

func TestE2E_Loop_EmptyListRunsZeroIterations(t *testing.T) {
	// given loop: over an empty list, when executed, then it runs zero
	// iterations without erroring
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo before
		      - loop: []
		        steps:
		          - run: echo item={{.ITEM}}
		      - run: echo after
	`, "t")
	res.OK(t)
	res.Lines(t, "before", "after")
}

func TestE2E_Loop_ObjectFormBindsKeyAndValueInSortedOrder(t *testing.T) {
	// given loop: over a real typed Object, when executed, then it iterates
	// sorted keys binding KEY/VALUE — the object counterpart to list
	// iteration's ITEM
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - REGIONS: { us: us-east-1, eu: eu-west-1 }
		      - loop: .REGIONS
		        steps:
		          - run: echo {{.KEY}}={{.VALUE}}
	`, "t")
	res.OK(t)
	res.Lines(t, "eu=eu-west-1", "us=us-east-1")
}

func TestE2E_Loop_StringThatMerelyLooksLikeJSONStaysAString(t *testing.T) {
	// given loop: over a plain string that starts with { but isn't valid
	// JSON, when executed, then it runs once with ITEM bound to the whole
	// string, rather than being re-sniffed as an object or erroring (why:
	// structure is sniffed exactly once, at capture — loop: must not
	// re-attempt it on a var that merely looks JSON-shaped)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - BAD: "{not valid json"
		      - loop: .BAD
		        steps:
		          - run: echo item={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "item={not valid json")
}

func TestE2E_Loop_ItemNotLeakedAfterLoop(t *testing.T) {
	// given ITEM not in scope before a list loop, when the loop completes,
	// then ITEM no longer exists afterward (why: the iterator must not leak
	// into the surrounding scope)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop: [a, b, c]
		        steps:
		          - run: echo item={{.ITEM}}
		      - run: echo after={{.ITEM | default "gone"}}
	`, "t")
	res.OK(t)
	res.Lines(t, "item=a", "item=b", "item=c", "after=gone")
}

func TestE2E_Loop_PreexistingIterNameRestoredAfterLoop(t *testing.T) {
	// given ITEM already set before a list loop, when the loop completes,
	// then ITEM is restored to its prior value (why: loop: must not
	// permanently clobber a caller's variable that happens to share the
	// iterator's name)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - ITEM: original
		      - loop: [x]
		        steps:
		          - run: echo during={{.ITEM}}
		      - run: echo after={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "during=x", "after=original")
}

func TestE2E_Loop_MapFormKeyValueNotLeakedAfterLoop(t *testing.T) {
	// given KEY/VALUE not in scope before a map-form loop, when it
	// completes, then both are gone afterward, mirroring ITEM's cleanup
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - MAP: { a: "1" }
		      - loop: .MAP
		        steps:
		          - run: echo key={{.KEY}} value={{.VALUE}}
		      - run: echo after key={{.KEY | default "gone"}} value={{.VALUE | default "gone"}}
	`, "t")
	res.OK(t)
	res.Lines(t, "key=a value=1", "after key=gone value=gone")
}

func TestE2E_Loop_MatrixSingleVarForm(t *testing.T) {
	// given loop: matrix form with a single named var, when executed, then
	// it iterates that var's list without needing a second key
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          PLATFORM: [ubuntu, macos, windows]
		        steps:
		          - run: echo platform={{.PLATFORM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "platform=ubuntu", "platform=macos", "platform=windows")
}

func TestE2E_Loop_MatrixCartesianProductInKeyOrder(t *testing.T) {
	// given loop: matrix form with two vars, when executed, then it runs
	// the full cartesian product with the first declared key as the
	// outermost loop (why: nesting order follows key-declaration order, not
	// alphabetical or map-random order)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          PLATFORM: [ubuntu, macos]
		          DB: [postgres, sqlite]
		        steps:
		          - run: echo {{.PLATFORM}}/{{.DB}}
	`, "t")
	res.OK(t)
	res.Lines(t,
		"ubuntu/postgres", "ubuntu/sqlite",
		"macos/postgres", "macos/sqlite",
	)
}

func TestE2E_Loop_MatrixReversedKeyOrderChangesNesting(t *testing.T) {
	// given the same two matrix vars declared in the opposite order, when
	// executed, then the nesting reverses too (why: pins that ordering
	// follows declaration, not the var names)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          DB: [postgres, sqlite]
		          PLATFORM: [ubuntu, macos]
		        steps:
		          - run: echo {{.DB}}/{{.PLATFORM}}
	`, "t")
	res.OK(t)
	res.Lines(t,
		"postgres/ubuntu", "postgres/macos",
		"sqlite/ubuntu", "sqlite/macos",
	)
}

func TestE2E_Loop_MatrixThreeVarsAllCombinations(t *testing.T) {
	// given a three-var matrix, when executed, then every combination runs
	// in correct nesting order
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          OS: [linux, macos]
		          ARCH: [amd64, arm64]
		          DB: [pg]
		        steps:
		          - run: echo {{.OS}}/{{.ARCH}}/{{.DB}}
	`, "t")
	res.OK(t)
	res.Lines(t,
		"linux/amd64/pg", "linux/arm64/pg",
		"macos/amd64/pg", "macos/arm64/pg",
	)
}

func TestE2E_Loop_MatrixDynamicListFromVar(t *testing.T) {
	// given a matrix var whose list is a {{.VAR}} template resolving to a
	// real typed Array, when executed, then it resolves without re-parsing
	// from text
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - SERVERS: [web1, web2]
		      - loop:
		          NODE: "{{.SERVERS}}"
		        steps:
		          - run: echo node={{.NODE}}
	`, "t")
	res.OK(t)
	res.Lines(t, "node=web1", "node=web2")
}

func TestE2E_Loop_MatrixIteratorVarsNotLeakedAfterLoop(t *testing.T) {
	// given matrix iterator vars not in scope before the loop, when it
	// completes, then none of them exist afterward
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          OS: [linux, mac]
		          ARCH: [amd64]
		        steps:
		          - run: echo {{.OS}}/{{.ARCH}}
		      - run: echo after os={{.OS | default "gone"}} arch={{.ARCH | default "gone"}}
	`, "t")
	res.OK(t)
	res.Lines(t, "linux/amd64", "mac/amd64", "after os=gone arch=gone")
}
