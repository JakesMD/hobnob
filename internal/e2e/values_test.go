package e2e

import "testing"

// Typed round-trips through the pipeline — value.Capture's sniff-once
// discipline (run: into: and CLI args) and the bare-.VAR-keeps-type rule from
// docs/guide/structured-data.md. Not a second filter-registry test — see
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

func TestE2E_Values_CLIArgThatIsJSONBecomesStructure(t *testing.T) {
	// given a CLI KEY=VALUE arg that decodes cleanly as a JSON array, when
	// looped over and indexed, then it behaves as a real Array (why: CLI
	// args go through value.Capture, the same rule as run: into:)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop: .TAGS
		        steps:
		          - run: echo item={{.ITEM}}
		      - run: echo first={{.TAGS[0]}}
	`, "t", `TAGS=["a","b"]`)
	res.OK(t)
	res.Lines(t, "item=a", "item=b", "first=a")
}

func TestE2E_Values_CLIArgThatIsJSONObjectBecomesAMap(t *testing.T) {
	// given a CLI arg that decodes cleanly as a JSON object, when reached
	// into, then it behaves as a real map (why: lists and objects follow
	// the same rule)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo easy={{.LEVELS.easy}} keys={{.LEVELS | keys | join ","}}
	`, "t", `LEVELS={"easy":1,"hard":3}`)
	res.OK(t)
	res.Lines(t, "easy=1 keys=easy,hard")
}

func TestE2E_Values_CLIArgThatIsJSONRendersAsTyped(t *testing.T) {
	// given a CLI arg that's JSON with spacing and key order of its own,
	// when rendered into a command, then the text comes out exactly as typed
	// (why: a captured value keeps its raw text, so passing JSON through to
	// another tool is untouched)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo 'body={{.BODY}}'
	`, "t", `BODY={"z": 1, "a": [ 2 ]}`)
	res.OK(t)
	res.Lines(t, `body={"z": 1, "a": [ 2 ]}`)
}

func TestE2E_Values_CLIArgThatIsNotJSONStaysText(t *testing.T) {
	// given CLI args that start with [ or { but aren't valid JSON, or are
	// JSON scalars, when used, then they stay plain text (why: only a
	// cleanly decoding list or object becomes structure)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: |
		          echo 'msg={{.MSG}} pat={{.PAT}} ver={{.VER}} len={{len .MSG}}'
	`, "t", "MSG=[WIP] fix", "PAT=[0-9]", "VER=1.0")
	res.OK(t)
	res.Lines(t, "msg=[WIP] fix pat=[0-9] ver=1.0 len=9")
}

func TestE2E_Values_EnvVarStaysTextEvenWhenJSONShaped(t *testing.T) {
	// given an OS environment variable whose value looks like a JSON array,
	// when indexed, then it stays plain text and the error names | json
	// (why: the shell environment holds names nobody set for hobnob, so
	// only CLI args are sniffed)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo {{.TAGS[0]}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"TAGS": `["a","b"]`},
	})
	res.Fails(t)
	res.Err(t, "json")
}

func TestE2E_Values_JSONFilterParsesPlainStringExplicitly(t *testing.T) {
	// given an env var that's JSON-shaped text, when explicitly piped
	// through | json, then it becomes real structure usable by an accessor
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo first={{(.TAGS | json)[0]}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"TAGS": `["a","b"]`},
	})
	res.OK(t)
	res.Lines(t, "first=a")
}

func TestE2E_Values_BareVarReferenceKeepsTypeThroughSet(t *testing.T) {
	// given set: A: .USERS (a bare var reference, no surrounding text), when
	// USERS is a real Array, then A keeps that Array type — but set: B:
	// "{{.USERS}}" (surrounding-text-free but explicitly wrapped) still
	// renders to its JSON text form, same as any templated field with
	// surrounding text would (why: the guide's "a field that's exactly one
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
