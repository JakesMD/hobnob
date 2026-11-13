package e2e

import "testing"

func TestE2E_Set_TopToBottomResolution(t *testing.T) {
	// given a set: block, when a later entry references an earlier one, then
	// it resolves against the value just assigned above it
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - BASE_URL: https://api.example.com
		          - AUTH_URL: "{{.BASE_URL}}/v1/auth"
		      - run: echo {{.AUTH_URL}}
	`, "t")
	res.OK(t)
	res.Lines(t, "https://api.example.com/v1/auth")
}

func TestE2E_Set_ListLiteralBecomesArray(t *testing.T) {
	// given a YAML sequence under set:, when executed, then it becomes a real
	// JSON array in scope — provable by looping over it, which only works on
	// an Array, not on text that merely looks like one
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - PORTS: [8080, 9090]
		      - loop: .PORTS
		        steps:
		          - run: echo port={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "port=8080", "port=9090")
}

func TestE2E_Set_MapLiteralBecomesObjectKeepingJSONTypes(t *testing.T) {
	// given a YAML map under set: with mixed scalar types and a template
	// leaf, when executed, then it becomes a real JSON object whose non-
	// string scalars keep their native type (a bool stays a bool, a number
	// stays a number) and whose template leaf resolves against scope —
	// provable via an accessor, which errors on a string but not on real
	// structure
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - ENV: prod
		          - CONFIG:
		              host: prod.example.com
		              replicas: 3
		              enabled: true
		              tag: "{{.ENV}}"
		      - run: echo host={{.CONFIG.host}} replicas={{.CONFIG.replicas}} enabled={{.CONFIG.enabled}} tag={{.CONFIG.tag}} gt={{gt .CONFIG.replicas 2}}
	`, "t")
	res.OK(t)
	res.Lines(t, "host=prod.example.com replicas=3 enabled=true tag=prod gt=true")
}

func TestE2E_Set_QuoteInTemplateLeafDoesNotCorruptJSON(t *testing.T) {
	// given a map-literal template leaf whose value contains a double quote,
	// when executed, then the quote is escaped rather than corrupting the
	// object (why: regression — marshaling must happen once, after leaf
	// evaluation, not by string-concatenating JSON text before it)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - NAME: 'he said "hi"'
		          - OBJ:
		              name: "{{.NAME}}"
		      - run: |
		          echo '{{.OBJ.name}}'
	`, "t")
	res.OK(t)
	res.Lines(t, `he said "hi"`)
}

func TestE2E_Set_SecretMasksValue(t *testing.T) {
	// given a set: entry marked secret, when a run: command referencing it is
	// displayed, then the secret is masked in that display line (why:
	// maskSecrets redacts hobnob's own "run: <command>" chrome — it can't
	// redact whatever the child process chooses to print, so this uses `:`,
	// a shell no-op that consumes its argument without echoing it, to keep
	// the assertion scoped to what hobnob itself controls)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - API_KEY:
		              value: abc123
		              secret: true
		      - run: ": {{.API_KEY}}"
	`, "t")
	res.OK(t)
	res.Masked(t, "abc123")
}

func TestE2E_Set_NonSecretEntryNotMasked(t *testing.T) {
	// given a set: entry with no secret flag, when its value is echoed, then
	// it prints unmasked (why: masking must be opt-in, not applied blanket)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - HOST: localhost
		      - run: echo host={{.HOST}}
	`, "t")
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Set_IfFalseSkipsSilently(t *testing.T) {
	// given a set: step whose if: evaluates false, when executed, then the
	// var is never assigned and nothing is printed (why: the "skipped" chrome
	// line is scoped to run: steps only — a silently-skipped set: doesn't
	// need an explanation the way a missing command's output would)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - MARKER: ran
		        if: '[ "{{.ENABLED}}" = "true" ]'
		      - run: echo marker={{.MARKER | default "unset"}}
	`, "t", "ENABLED=false")
	res.OK(t)
	res.NotOut(t, "skipped", "ran")
	res.Lines(t, "marker=unset")
}

func TestE2E_Set_TemplateSyntaxInVarNameRejected(t *testing.T) {
	// given a set: key containing template syntax, when parsed, then it's
	// rejected rather than silently treated as a literal key (why: this
	// same rejection applies at every var-naming site — set:, into:, get:,
	// loop: matrix keys — set: is the cheapest one to pin here)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - "{{.BAD}}": oops
	`, "t")
	res.Fails(t)
	res.Err(t, "template syntax")
}

func TestE2E_Set_NestedMapKeyErrorNamesFullPath(t *testing.T) {
	// given an invalid key deep inside a set: map literal, when parsed, then
	// the error names the full dotted path, not just the outer key (why:
	// otherwise a bad key several levels deep is nearly impossible to find
	// in a large literal)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - OUTER:
		              inner:
		                "{{bad}}": oops
	`, "t")
	res.Fails(t)
	res.Err(t, "OUTER.inner.{{bad}}")
}

func TestE2E_Set_SpreadSplicesListsIntoAListLiteral(t *testing.T) {
	// given lists spread with ... around a plain element and a nested one,
	// when set: builds the literal, then each spread splices its items in
	// place while the element without ... still nests (why: [.A..., d]
	// builds one list from others without losing JSON nesting)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - A: [a, b]
		          - B: [c]
		          - NONE: []
		          - C: [.A..., d, .NONE..., .B..., .B]
		      - run: echo '{{ .C }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `["a","b","d","c",["c"]]`)
}

func TestE2E_Set_SpreadOutsideAListErrors(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		wantErr string
	}{
		{"given a single-value set: entry written .A..., when the file loads, then it errors (why: B: .A... has no list to splice into)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - B: .A...
		`, `set entry "B": ".A..." spreads a list`},
		{"given the expanded value: form written .A..., when the file loads, then it errors (why: value: holds a single value too)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - B:
			              value: .A...
			              secret: true
		`, `set entry "B": ".A..." spreads a list`},
		{"given a map literal field written .A..., when the file loads, then it errors (why: an object field holds a single value)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - B: { k: .A... }
		`, `set entry "B": B.k: ".A..." spreads a list`},
		{"given a bare spread entry in set:'s list of KEY: value maps, when the file loads, then it errors (why: a spread names no variable to set)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - .A...
		`, `set entry ".A...": a spread is not a KEY: value pair`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			res := Yml(t, tc.yml, "t")

			// Assert
			res.Fails(t)
			res.Err(t, tc.wantErr)
		})
	}
}

func TestE2E_Set_SpreadOfAStringErrorsNamingJSON(t *testing.T) {
	// given a list literal spreading a string variable, when set: runs, then
	// it errors naming | json (why: spreading a non-list is a wrong-kind
	// error, and text that should be a list is parsed explicitly)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - TEXT: '["a"]'
		          - B: [.TEXT..., b]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "cannot spread .TEXT", "| json")
}

func TestE2E_Set_SpreadOfParsedTextAndAccessorChain(t *testing.T) {
	// given spreads of a parenthesised json pipeline and a [*] accessor
	// chain, when set: runs, then both splice their resulting lists (why:
	// spread applies to the whole element reference, filters and accessors
	// included)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - TEXT: '["a","b"]'
		          - USERS: [{ name: c }, { name: d }]
		          - ALL:
		              - (.TEXT | json)...
		              - .USERS[*].name...
		      - run: echo '{{ .ALL }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `["a","b","c","d"]`)
}

func TestE2E_Set_SpreadOfAMissingPathErrors(t *testing.T) {
	// given a spread of a path that isn't there, when set: runs, then it
	// fails as path not found (why: spread follows the usual absence rules)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { a: [x] }
		          - B: [.CFG.nope..., y]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, `"nope" not found`)
}

func TestE2E_Set_SpreadOfANonListErrors(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		wantErr string
	}{
		{"given a spread of an unset variable, when set: runs, then it errors saying it is null or unset (why: a typo'd name should read as a typo, not a type puzzle)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - B: [x, .MISSING..., y]
		`, "cannot spread .MISSING: it is null or unset"},
		{"given a spread of a number, when set: runs, then it errors naming the kind (why: only a list can be spliced)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - L: [3]
			          - B:
			              - .L[0]...
			              - y
		`, "cannot spread .L[0]: it is a number"},
		{"given a spread of an object, when set: runs, then it errors naming the kind (why: only a list can be spliced)", `
			tasks:
			  t:
			    steps:
			      - set:
			          - O: { k: v }
			          - B: [.O..., y]
		`, "cannot spread .O: it is an object"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			res := Yml(t, tc.yml, "t")

			// Assert
			res.Fails(t)
			res.Err(t, tc.wantErr)
		})
	}
}

func TestE2E_Set_SpreadInsideNestedStructures(t *testing.T) {
	// given spreads inside a nested list and inside an object within a list,
	// when set: runs, then each splices into its own enclosing list (why: a
	// spread belongs to the list it is written in, at any depth)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - A: [a, b]
		          - NESTED: [[.A..., c], d]
		          - IN_OBJECT: [{ k: [.A..., z] }]
		      - run: echo '{{ .NESTED }} {{ .IN_OBJECT }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `[["a","b","c"],"d"] [{"k":["a","b","z"]}]`)
}

func TestE2E_Set_SpreadOfAMissingPathCaughtByDefaultAddsNothing(t *testing.T) {
	// given a spread of a missing path caught by default "[]" | json, when
	// set: runs, then it adds nothing (why: spread follows the usual absence
	// rules, so default catches it)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - CFG: { a: [x] }
		          - B:
		              - '.CFG.nope | default "[]" | json...'
		              - y
		      - run: echo '{{ .B }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `["y"]`)
}
