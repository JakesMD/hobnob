package e2e

import (
	"path/filepath"
	"testing"
)

func TestE2E_Call_WithAndIntoRoundTrip(t *testing.T) {
	// given a call: step that passes a var in via with: and maps one back
	// via into:, when it completes, then the parent sees the child's result
	// under the parent's own var name
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - INPUT: hello
		      - call: child
		        with:
		          - MSG: "{{.INPUT}}"
		        into:
		          - RESULT: OUTPUT
		      - run: echo result={{.RESULT}}
		  child:
		    steps:
		      - set:
		          - OUTPUT: "{{.MSG}}_processed"
	`, "parent")
	res.OK(t)
	res.Lines(t, "result=hello_processed")
}

func TestE2E_Call_LaterIntoEntryTemplatesOverEarlier(t *testing.T) {
	// given two into: entries in one call: step, when the second templates
	// over the first, then it sees the first entry's already-mapped value
	// (why: into: resolves top-to-bottom, same as set:)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        into:
		          - LOG_PATH: LOG_FILE
		          - ARCHIVE_PATH: "{{.LOG_PATH}}/archive"
		      - run: echo path={{.LOG_PATH}} archive={{.ARCHIVE_PATH}}
		  child:
		    steps:
		      - set:
		          - LOG_FILE: /var/log/app.log
	`, "parent")
	res.OK(t)
	res.Lines(t, "path=/var/log/app.log archive=/var/log/app.log/archive")
}

func TestE2E_Call_ScopeIsolation(t *testing.T) {
	// given a called child task that sets its own local vars, when the call
	// returns, then only the vars explicitly pulled back via into: exist in
	// the parent — everything else the child set stays sandboxed (why:
	// Scope.Copy() before every call: means child mutations don't leak
	// unless the caller opted in)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        into:
		          - RESULT: OUTPUT
		      - run: echo result={{.RESULT}} leaked={{.CHILD_ONLY | default "no"}} output={{.OUTPUT | default "no"}}
		  child:
		    steps:
		      - set:
		          - OUTPUT: processed
		          - CHILD_ONLY: should-not-leak
	`, "parent")
	res.OK(t)
	res.Lines(t, "result=processed leaked=no output=no")
}

func TestE2E_Call_IntoDotPrefixIsOptional(t *testing.T) {
	// given into: written with a leading dot (".OUTPUT") vs without, when
	// the call completes, then both resolve to the same child var
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        into:
		          - RESULT: .OUTPUT
		      - run: echo result={{.RESULT}}
		  child:
		    steps:
		      - set:
		          - OUTPUT: ok
	`, "parent")
	res.OK(t)
	res.Lines(t, "result=ok")
}

func TestE2E_Call_IntoNestedObjectMixesBareAndTemplateLeaves(t *testing.T) {
	// given a nested into: object with one bare .FIELD leaf and one {{ }}
	// template leaf, when the call completes, then both resolve into a
	// single assembled JSON var (why: into:'s nested form reuses the same
	// dual-mode leaf grammar as the flat form, not a third one)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - PREFIX: log
		      - call: child
		        into:
		          - CUSTOM:
		              output: .OUTPUT
		              label: "{{.PREFIX}}-entry"
		      - run: echo output={{.CUSTOM | pluck "output"}} label={{.CUSTOM | pluck "label"}}
		  child:
		    steps:
		      - set:
		          - OUTPUT: ok
	`, "parent")
	res.OK(t)
	res.Lines(t, "output=ok label=log-entry")
}

func TestE2E_Call_WithMapLiteralQuoteEscaped(t *testing.T) {
	// given a with: map literal whose template leaf value contains a double
	// quote, when the child reads it back via pluck, then the quote is
	// escaped rather than corrupting the JSON (why: with: shares parseSetNode
	// with set:, so it inherited — and must stay fixed for — the same
	// marshal-after-eval discipline)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - NAME: 'he said "hi"'
		      - call: child
		        with:
		          - CUSTOM:
		              name: "{{.NAME}}"
		        into:
		          - RESULT: .PLUCKED
		      - run: |
		          echo '{{.RESULT}}'
		  child:
		    steps:
		      - set:
		          - PLUCKED: '{{ .CUSTOM | json | pluck "name" }}'
	`, "parent")
	res.OK(t)
	res.Lines(t, `he said "hi"`)
}

func TestE2E_Call_SecretPassedThroughWithStaysMaskedUnderNewName(t *testing.T) {
	// given a secret passed into a child via with: under a different name,
	// when the child's run: command references the new name, then it's
	// still masked (why: masking matches on value, not var name — with:
	// takes no secret: flag of its own precisely because Scope.Copy() already
	// carries the parent's secret-ness across)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - VAULT_TOKEN:
		              value: s3cr3t-value
		              secret: true
		      - call: child
		        with:
		          - TOKEN: "{{.VAULT_TOKEN}}"
		  child:
		    steps:
		      - run: ": {{.TOKEN}}"
	`, "parent")
	res.OK(t)
	res.Masked(t, "s3cr3t-value")
}

func TestE2E_Call_ComposedSecretThroughWithMasksOnlySecretComponent(t *testing.T) {
	// given a with: value composing a secret and a non-secret var, when the
	// child displays the run: command, then only the secret component is
	// masked, keeping the rest of the line readable
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - set:
		          - USER: admin
		          - PASS:
		              value: hunter2
		              secret: true
		      - call: child
		        with:
		          - DSN: "postgres://{{.USER}}:{{.PASS}}@db"
		  child:
		    steps:
		      - run: "true # {{.DSN}}"
	`, "parent")
	res.OK(t)
	res.Out(t, "postgres://admin:****@db")
	res.NotOut(t, "hunter2")
}

func TestE2E_Call_SecretInWithRejectedAtParseTime(t *testing.T) {
	// given a with: entry marked secret: true, when parsed, then it's
	// rejected rather than silently ignored (why: with: doesn't need its own
	// secret: flag — see the masking tests above — so allowing one would
	// just be a confusing dead knob)
	res := Yml(t, `
		tasks:
		  parent:
		    steps:
		      - call: child
		        with:
		          - TOKEN:
		              value: abc
		              secret: true
		  child:
		    steps:
		      - run: echo hi
	`, "parent")
	res.Fails(t)
	res.Err(t, "secret")
}

func TestE2E_Call_DirTemplateUsesWithVar(t *testing.T) {
	// given a call: step's dir: template references a with: variable, when
	// the call runs, then dir: resolves using that with: value (why: the
	// child's scope — including with: vars — is populated before dir: is
	// evaluated)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  parent:
				    steps:
				      - call: child
				        dir: "{{.TARGET_DIR}}"
				        with:
				          - TARGET_DIR: target
				  child:
				    steps:
				      - run: pwd
				        into:
				          - _PWD: stdout | trim
				      - run: echo pwd={{._PWD}}
			`,
			"target/.keep": "",
		},
		Args: []string{"parent"},
	})
	res.OK(t)
	want := filepath.Join(res.Dir, "target")
	res.Lines(t, want, "pwd="+want)
}

func TestE2E_Call_StepInteractiveFalseDisablesPrompts(t *testing.T) {
	// given a call: step with interactive: false, when the called task has a
	// get: with a default, when run even on a simulated TTY, then the
	// default is used without prompting (why: interactive: false works the
	// same at a call site as it does on a task itself)
	res := Run(t, Case{
		Yml: `
			tasks:
			  parent:
			    steps:
			      - call: child
			        interactive: false
			  child:
			    steps:
			      - get:
			          - FOO:
			              default: silent-val
			      - run: echo foo={{.FOO}}
		`,
		Args: []string{"parent"},
		TTY:  true,
	})
	res.OK(t)
	res.Lines(t, "foo=silent-val")
	res.Prompted(t /* none */)
}

func TestE2E_Call_StepInteractiveFalsePropagatesThroughSubTree(t *testing.T) {
	// given a call: step with interactive: false calling a task that itself
	// calls another task, when the grandchild has a get:, then it also skips
	// prompting (why: interactive: false propagates through the entire
	// sub-tree, not just the immediate callee)
	res := Run(t, Case{
		Yml: `
			tasks:
			  root:
			    steps:
			      - call: mid
			        interactive: false
			  mid:
			    steps:
			      - call: leaf
			  leaf:
			    steps:
			      - get:
			          - X:
			              default: deep
			      - run: echo x={{.X}}
		`,
		Args: []string{"root"},
		TTY:  true,
	})
	res.OK(t)
	res.Lines(t, "x=deep")
	res.Prompted(t /* none */)
}

func TestE2E_Call_SoftSwallowsOrdinaryChildFailure(t *testing.T) {
	// given a soft: true call: step whose child fails with an ordinary
	// command error, when executed, then the error is swallowed and
	// execution continues past it
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - call: child
		        soft: true
		      - run: echo after
		  child:
		    steps:
		      - run: exit 1
	`, "t")
	res.OK(t)
	res.Lines(t, "after")
}
