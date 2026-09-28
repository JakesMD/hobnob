package e2e

import "testing"

// The full const:/vars: precedence chain is covered in scope_test.go,
// alongside the rest of the layer chain. This file covers vars:'s own
// parse-time rule and its top-to-bottom resolution.

func TestE2E_Vars_SelfReferenceErrors(t *testing.T) {
	// given a vars: entry referencing its own key, when the file loads,
	// then it errors (why: this is the old pre-v0.3 {{ .HOST | default
	// "localhost" }} defaults-hack under the new name — vars: already IS
	// the fallback layer, so the pattern is always redundant)
	res := Yml(t, `
		vars:
		  - HOST: '{{ .HOST | default "localhost" }}'

		tasks:
		  t:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "vars: HOST references itself")
}

func TestE2E_Vars_TopToBottomResolution(t *testing.T) {
	// given a vars: block, when a later entry references an earlier one,
	// then it resolves against the value just assigned above it (why:
	// matches set:'s own top-to-bottom rule, and confirms vars: entries
	// evaluate sequentially rather than all against the same frozen base)
	res := Yml(t, `
		vars:
		  - BASE_URL: https://api.example.com
		  - AUTH_URL: "{{.BASE_URL}}/v1/auth"

		tasks:
		  t:
		    steps:
		      - run: echo {{.AUTH_URL}}
	`, "t")
	res.OK(t)
	res.Lines(t, "https://api.example.com/v1/auth")
}

func TestE2E_Vars_SecretFlagMasksValue(t *testing.T) {
	// given a vars: entry using the expanded { value:, secret: true } form,
	// when referenced in a run: command, then it's masked — vars: takes the
	// same entry shapes as set:
	res := Yml(t, `
		vars:
		  - TOKEN:
		      value: hunter2
		      secret: true

		tasks:
		  t:
		    steps:
		      - run: ": {{.TOKEN}}"
	`, "t")
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Vars_ReadsValueFromEnvFile(t *testing.T) {
	// given an env: file setting a var and a vars: entry building a default
	// from it, when the task runs, then vars: sees the env: file's value
	// (why: the whole point of this issue — vars: can now read upward)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				vars:
				  - URL: "https://{{.API_HOST}}/v1"
				tasks:
				  t:
				    steps:
				      - run: echo {{.URL}}
			`,
			".env": "API_HOST=staging.example.com\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "https://staging.example.com/v1")
}

func TestE2E_Vars_ReadsCLIArgAndConst(t *testing.T) {
	// given a vars: entry built from a CLI arg and a const: value, when the
	// task runs, then vars: sees both — upward reads cover every layer above
	// it, not just env: files
	res := Yml(t, `
		const:
		  - REGION: eu

		vars:
		  - LABEL: "{{.STAGE}}-{{.REGION}}"

		tasks:
		  t:
		    steps:
		      - run: echo {{.LABEL}}
	`, "t", "STAGE=prod")
	res.OK(t)
	res.Lines(t, "prod-eu")
}

func TestE2E_Vars_OverriddenEntryIsNeverEvaluated(t *testing.T) {
	// given a vars: entry whose template would error, and a CLI arg for the
	// same name, when the task runs, then it succeeds — an overridden vars:
	// entry is skipped entirely, not evaluated, so a template that would only
	// make sense standing alone never runs just because it lost
	res := Yml(t, `
		vars:
		  - HOST: "{{.MISSING.field}}"

		tasks:
		  t:
		    steps:
		      - run: echo host={{.HOST}}
	`, "t", "HOST=from-cli")
	res.OK(t)
	res.Lines(t, "host=from-cli")
}

func TestE2E_Vars_ListLiteralStaysTyped(t *testing.T) {
	// given a vars: entry holding a YAML list literal, when looped over,
	// then it's a real Array — vars: entries stay typed, same as set:
	res := Yml(t, `
		vars:
		  - PORTS: [8080, 9090]

		tasks:
		  t:
		    steps:
		      - loop: .PORTS
		        steps:
		          - run: echo port={{.ITEM}}
	`, "t")
	res.OK(t)
	res.Lines(t, "port=8080", "port=9090")
}
