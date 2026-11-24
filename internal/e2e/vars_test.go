package e2e

import "testing"

func TestE2E_Vars_SelfReferenceErrors(t *testing.T) {
	// given a vars: entry referencing its own key, when the file loads,
	// then it errors (why: this is the old pre-v0.3 {{ .HOST | default
	// "localhost" }} defaults-hack under the new name — vars: already IS
	// the fallback layer, so the pattern is always redundant)
	// Arrange
	yml := `
		vars:
		  - HOST: '{{ .HOST | default "localhost" }}'

		tasks:
		  t:
		    steps:
		      - run: echo hi
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "vars: HOST references itself")
}

func TestE2E_Vars_TopToBottomResolution(t *testing.T) {
	// given a vars: block, when a later entry references an earlier one,
	// then it resolves against the value just assigned above it (why:
	// matches set:'s own top-to-bottom rule, and confirms vars: entries
	// evaluate sequentially rather than all against the same frozen base)
	// Arrange
	yml := `
		vars:
		  - BASE_URL: https://api.example.com
		  - AUTH_URL: "{{.BASE_URL}}/v1/auth"

		tasks:
		  t:
		    steps:
		      - run: echo {{.AUTH_URL}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "https://api.example.com/v1/auth")
}

func TestE2E_Vars_SecretFlagMasksValue(t *testing.T) {
	// given a vars: entry using the expanded { value:, secret: true } form,
	// when referenced in a run: command, then it's masked (why: vars: takes
	// the same entry shapes as set:)
	// Arrange
	yml := `
		vars:
		  - TOKEN:
		      value: hunter2
		      secret: true

		tasks:
		  t:
		    steps:
		      - run: ": {{.TOKEN}}"
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Vars_ReadsValueFromEnvFile(t *testing.T) {
	// given an env: file setting a var and a vars: entry building a default
	// from it, when the task runs, then vars: sees the env: file's value
	// (why: the whole point of this issue — vars: can now read upward)
	// Arrange
	c := Case{
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
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "https://staging.example.com/v1")
}

func TestE2E_Vars_ReadsCLIArgAndConst(t *testing.T) {
	// given a vars: entry built from a CLI arg and a const: value, when the
	// task runs, then vars: sees both (why: upward reads cover every layer
	// above vars:, not just env: files)
	// Arrange
	yml := `
		const:
		  - REGION: eu

		vars:
		  - LABEL: "{{.STAGE}}-{{.REGION}}"

		tasks:
		  t:
		    steps:
		      - run: echo {{.LABEL}}
	`

	// Act
	res := Yml(t, yml, "t", "STAGE=prod")

	// Assert
	res.OK(t)
	res.Lines(t, "prod-eu")
}

func TestE2E_Vars_OverriddenEntryIsNeverEvaluated(t *testing.T) {
	// given a vars: entry whose template would error, and a CLI arg for the
	// same name, when the task runs, then it succeeds (why: an overridden
	// vars: entry is skipped, not evaluated, so a template that only makes
	// sense standing alone never runs just because it lost)
	// Arrange
	yml := `
		vars:
		  - HOST: "{{.MISSING.field}}"

		tasks:
		  t:
		    steps:
		      - run: echo host={{.HOST}}
	`

	// Act
	res := Yml(t, yml, "t", "HOST=from-cli")

	// Assert
	res.OK(t)
	res.Lines(t, "host=from-cli")
}

func TestE2E_Vars_ListLiteralStaysTyped(t *testing.T) {
	// given a vars: entry holding a YAML list literal, when looped over, then
	// it's a real Array (why: vars: entries stay typed, same as set:)
	// Arrange
	yml := `
		vars:
		  - PORTS: [8080, 9090]

		tasks:
		  t:
		    steps:
		      - loop: .PORTS
		        steps:
		          - run: echo port={{.ITEM}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "port=8080", "port=9090")
}

func TestE2E_Vars_ListLiteralSpreadsEarlierVar(t *testing.T) {
	// given a vars: list literal spreading an earlier vars: list, when the
	// file loads, then the default is one flat list (why: vars: builds
	// structure, so it takes spread like every other literal)

	// Arrange
	yml := `
		vars:
		  - CURL_OPTS: [-s]
		  - VERBOSE_OPTS: [.CURL_OPTS..., --verbose]

		tasks:
		  t:
		    steps:
		      - run: echo '{{ .VERBOSE_OPTS }}'
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, `["-s","--verbose"]`)
}
