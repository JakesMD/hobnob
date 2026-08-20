package e2e

import "testing"

func TestE2E_RunInto_StdoutCapture(t *testing.T) {
	// given into: stdout, when a command prints to stdout, then the raw
	// output is captured into the named var
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'hello'
		        into:
		          - OUT: stdout
		      - run: echo captured={{.OUT}}
	`, "t")
	res.OK(t)
	res.Lines(t, "hello", "captured=hello")
}

func TestE2E_RunInto_StderrCapture(t *testing.T) {
	// given into: stderr, when a command prints to stderr, then the raw
	// stderr is captured separately from stdout
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'err msg' >&2
		        into:
		          - ERR: stderr
		      - run: echo captured={{.ERR}}
	`, "t")
	res.OK(t)
	res.Lines(t, "captured=err msg")
}

func TestE2E_RunInto_FilterChain(t *testing.T) {
	// given into: sources piped through filters, when captured, then each
	// filter applies left to right — trim, lines (JSON array), upper, lower,
	// and a two-stage chain — exercising the same filter registry {{ }}
	// templates use, not a separate hand-rolled pipe grammar
	tests := []struct {
		name     string
		command  string
		filter   string
		rawLines []string // what the command itself prints, before capture
		wantEcho string   // what the captured+filtered var renders as
	}{
		{name: "trim", command: `printf 'hello\n'`, filter: "stdout | trim", rawLines: []string{"hello"}, wantEcho: "out=hello"},
		{name: "upper", command: `printf 'hello'`, filter: "stdout | upper", rawLines: []string{"hello"}, wantEcho: "out=HELLO"},
		{name: "lower", command: `printf 'HELLO'`, filter: "stdout | lower", rawLines: []string{"HELLO"}, wantEcho: "out=hello"},
		{name: "trim-then-upper-chains-left-to-right", command: `printf '  hello  \n'`, filter: "stdout | trim | upper", rawLines: []string{"  hello  "}, wantEcho: "out=HELLO"},
		{name: "accessor-on-the-into-source", command: `printf '{"name":"hobnob"}'`, filter: `stdout.name`, rawLines: []string{`{"name":"hobnob"}`}, wantEcho: "out=hobnob"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			res := Run(t, Case{
				Files: Files{"hobnob.yml": "tasks:\n  t:\n    steps:\n      - run: " + test.command + "\n        into:\n          - OUT: " + test.filter + "\n      - run: echo out={{.OUT}}\n"},
				Args:  []string{"t"},
			})
			res.OK(t)
			res.Lines(t, append(append([]string{}, test.rawLines...), test.wantEcho)...)
		})
	}
}

func TestE2E_RunInto_LinesProducesRealArray(t *testing.T) {
	// given into: stdout | lines on multi-line output, when the result is
	// looped over, then it behaves as a real array, not text that merely
	// looks like a JSON array (why: loop: only iterates a typed Array)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'alpha\nbeta\ngamma'
		        into:
		          - ITEMS: stdout | lines
		      - loop: .ITEMS
		        steps:
		          - run: echo item={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "alpha", "beta", "gamma", "item=alpha", "item=beta", "item=gamma")
}

func TestE2E_RunInto_TwoEntriesCaptureIndependently(t *testing.T) {
	// given two into: entries on one step, when the command writes to both
	// streams, then each is captured into its own var without cross-talk
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'out' && printf 'err' >&2
		        into:
		          - OUT: stdout
		          - ERR: stderr
		      - run: echo out={{.OUT}} err={{.ERR}}
	`, "t")
	res.OK(t)
	res.Lines(t, "out", "out=out err=err")
}

func TestE2E_RunInto_UnknownFilterErrors(t *testing.T) {
	// given into: piped through a filter that doesn't exist, when executed,
	// then fails naming the bad filter rather than silently no-op'ing
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'hello'
		        into:
		          - OUT: stdout | nope
	`, "t")
	res.Fails(t)
	res.Err(t, "run into")
}

func TestE2E_RunInto_UnknownSourceErrors(t *testing.T) {
	// given into: from a source that isn't stdout or stderr, when executed,
	// then fails naming the bad source
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf 'hello'
		        into:
		          - OUT: stdin
	`, "t")
	res.Fails(t)
	res.Err(t, "run into")
}

func TestE2E_RunInto_NestedObjectAssemblesTypedFields(t *testing.T) {
	// given a nested-object into: entry whose leaves access one JSON
	// response, when executed, then it assembles a single JSON var with
	// numbers kept as real numbers, not stringified (why: the tree is built
	// typed and marshaled once, not string-concatenated)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: printf '{"id":42,"profile":{"name":"Ada"}}'
		        into:
		          - CUSTOM:
		              id: stdout.id
		              name: stdout.profile.name
		      - run: echo id={{.CUSTOM.id}} name={{.CUSTOM.name}}
	`, "t")
	res.OK(t)
	res.Lines(t, `{"id":42,"profile":{"name":"Ada"}}`, "id=42 name=Ada")
}

func TestE2E_RunInto_NestedObjectQuoteEscaped(t *testing.T) {
	// given a nested into: leaf whose accessed value contains a double
	// quote, when assembled, then the quote is escaped rather than
	// corrupting the resulting JSON (why: same injection-bug regression as
	// set:/with: — marshal happens once, after every leaf evaluates)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: |
		          printf '%s' '{"msg":"he said \"hi\""}'
		        into:
		          - OUT:
		              msg: stdout.msg
		      - run: |
		          echo '{{.OUT.msg}}'
	`, "t")
	res.OK(t)
	res.Lines(t, `{"msg":"he said \"hi\""}`, `he said "hi"`)
}

func TestE2E_Run_ScopeVarOverridesInheritedEnv(t *testing.T) {
	// given an env var and a scope var of the same name, when a run: command
	// reads it, then the scope value wins (why: exec's first-occurrence
	// semantics means a naive append-only env build would silently lose the
	// override)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - set:
			          - FOO: from-scope
			      - run: printf '%s' "$FOO"
			        into:
			          - RESULT: stdout
			      - run: echo result={{.RESULT}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"FOO": "from-env"},
	})
	res.OK(t)
	res.Lines(t, "from-scope", "result=from-scope")
}

func TestE2E_Run_EnvVarNotInScopeStillVisible(t *testing.T) {
	// given an env var never referenced by scope, when a run: command reads
	// it, then it still passes through to the child process
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: printf '%s' "$BAR"
			        into:
			          - RESULT: stdout
			      - run: echo result={{.RESULT}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"BAR": "from-env"},
	})
	res.OK(t)
	res.Lines(t, "from-env", "result=from-env")
}

func TestE2E_Run_IfFalseSkipsAndPrintsSkipLine(t *testing.T) {
	// given a run: step whose if: evaluates false, when executed, then the
	// command doesn't run and a skip line explains why, rather than silence
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo should-not-run
		        if: '[ "{{.ENABLED}}" = "true" ]'
	`, "t", "ENABLED=false")
	res.OK(t)
	res.Out(t, "run:", "[t]", "skipped")
	res.NotOut(t, "should-not-run")
}

func TestE2E_Run_SoftSwallowsFailure(t *testing.T) {
	// given a soft: true run: step whose command exits non-zero, when
	// executed, then the error is swallowed and execution continues past it
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: exit 1
		        soft: true
		      - run: echo after
	`, "t")
	res.OK(t)
	res.Lines(t, "after")
}
