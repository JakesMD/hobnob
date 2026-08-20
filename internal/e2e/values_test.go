package e2e

import "testing"

// Typed round-trips through the pipeline — value.Capture's sniff-once
// discipline and the bare-.VAR-keeps-type rule from GUIDE.md's "Typed
// values" section. Not a second filter-registry test — see
// internal/value/filter_test.go for that.

func TestE2E_Values_CapturedValidJSONArrayBecomesLoopableStructure(t *testing.T) {
	// given run: into: capturing stdout that decodes cleanly as a JSON
	// array, when the result is looped over, then it behaves as a real
	// Array (why: value.Capture sniffs structure at the capture boundary,
	// not on every later use)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '["a","b","c"]'
		        into:
		          - ITEMS: stdout
		      - loop: .ITEMS
		        steps:
		          - run: echo item={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, `["a","b","c"]`, "item=a", "item=b", "item=c")
}

func TestE2E_Values_CapturedTextThatMerelyStartsWithBraceStaysString(t *testing.T) {
	// given run: into: capturing stdout that starts with { but doesn't
	// decode as valid JSON, when the result is used, then it stays a plain
	// string rather than being partially parsed or erroring (why:
	// value.Capture only structures text that decodes cleanly — a
	// truncated/malformed JSON-ish blob is not silently reinterpreted)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{not valid json'
		        into:
		          - RESP: stdout
		      - run: echo resp={{.RESP}}
	`, "t")
	res.OK(t)
	res.Lines(t, "{not valid json", "resp={not valid json")
}

func TestE2E_Values_CLIArgStaysTextEvenWhenJSONShaped(t *testing.T) {
	// given a CLI KEY=VALUE arg whose value looks like a JSON array, when
	// used directly, then it stays plain text — indexing it errors naming
	// | json rather than silently treating it as structure (why: only set:/
	// with: literals, into: capture, and the explicit | json filter
	// ever introduce structure — CLI args never do)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo {{.TAGS[0]}}
	`, "t", `TAGS=["a","b"]`)
	res.Fails(t)
	res.Err(t, "json")
}

func TestE2E_Values_JSONFilterParsesPlainStringExplicitly(t *testing.T) {
	// given a CLI arg that's JSON-shaped text, when explicitly piped through
	// | json, then it becomes real structure usable by an accessor/loop
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo first={{(.TAGS | json)[0]}}
	`, "t", `TAGS=["a","b"]`)
	res.OK(t)
	res.Lines(t, "first=a")
}

func TestE2E_Values_BareVarReferenceKeepsTypeThroughSet(t *testing.T) {
	// given set: A: .USERS (a bare var reference, no surrounding text), when
	// USERS is a real Array, then A keeps that Array type — but set: B:
	// "{{.USERS}}" (surrounding-text-free but explicitly wrapped) still
	// renders to its JSON text form, same as any templated field with
	// surrounding text would (why: GUIDE.md's "a field that's exactly one
	// variable reference keeps that variable's type; any surrounding text
	// renders it to a plain string" rule)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - USERS: [ada, grace]
		          - A: .USERS
		          - B: "{{.USERS}}"
		      - loop: .A
		        steps:
		          - run: echo item={{.ITEM}}
		      - run: |
		          echo 'b={{.B}}'
	`, "t")
	res.OK(t)
	res.Lines(t, "item=ada", "item=grace", `b=["ada","grace"]`)
}
