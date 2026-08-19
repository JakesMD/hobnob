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
	// provable via pluck, which errors on a string but not on real structure
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
		      - run: echo host={{.CONFIG | pluck "host"}} replicas={{.CONFIG | pluck "replicas"}} enabled={{.CONFIG | pluck "enabled"}} tag={{.CONFIG | pluck "tag"}}
	`, "t")
	res.OK(t)
	res.Lines(t, "host=prod.example.com replicas=3 enabled=true tag=prod")
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
		          echo '{{.OBJ | pluck "name"}}'
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
