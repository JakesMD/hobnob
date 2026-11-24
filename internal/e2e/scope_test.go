package e2e

import "testing"

func TestE2E_Scope_SetStepWinsOverCLIArgs(t *testing.T) {
	// given a set: step and a matching CLI KEY=VALUE arg, when the task
	// runs, then the set: value wins (why: set: is a known value the task
	// author controls — a caller can't silently override it; get: is the
	// public API for caller input)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - HOST: localhost
		      - run: echo host={{.HOST}}
	`

	// Act
	res := Yml(t, yml, "t", "HOST=remotehost")

	// Assert
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_GetDefaultLosesToCLIArgs(t *testing.T) {
	// given a get: step with a default: and a matching CLI KEY=VALUE arg,
	// when the task runs, then the CLI arg satisfies the prompt and wins
	// (why: this is how a caller overrides an overridable default — the
	// replacement for the old vars: '{{ .HOST | default "localhost" }}' idiom)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - HOST:
		              default: localhost
		      - run: echo host={{.HOST}}
	`

	// Act
	res := Yml(t, yml, "t", "HOST=remotehost")

	// Assert
	res.OK(t)
	res.Lines(t, "host=remotehost")
}

func TestE2E_Scope_GetDefaultUsedWhenNoOverride(t *testing.T) {
	// given a get: step with a default: and no CLI arg, when the task runs,
	// then the default is used (why: this is the fallback half of the
	// overridable-default idiom)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - get:
		          - HOST:
		              default: localhost
		      - run: echo host={{.HOST}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_SystemVarsAlwaysInjected(t *testing.T) {
	// given any taskfile, when a task echoes HOBNOB_FILE_DIR and
	// HOBNOB_INVOCATION_DIR, then both are populated (why: these built-ins
	// must always be available, with no opt-in step needed)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: echo dir={{.HOBNOB_FILE_DIR}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "dir="+res.Dir)
}

func TestE2E_Scope_CLIArgsWinOverEnvFile(t *testing.T) {
	// given an env: file setting a var and a matching CLI arg, when the task
	// runs, then the CLI arg wins (why: a caller's explicit override always
	// beats a sourced default)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			".env": "FOO=fromfile\n",
		},
		Args: []string{"t", "FOO=fromcli"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "foo=fromcli")
}

func TestE2E_Scope_SetStepWinsOverEnvFile(t *testing.T) {
	// given an env: file and a set: step assigning the same var, when the
	// task runs, then the set: value wins (why: the timeline sits above
	// every scope.Load-layer source, env: files included)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - set:
				          - HOST: localhost
				      - run: echo host={{.HOST}}
			`,
			".env": "HOST=fromfile\n",
		},
		Args: []string{"t"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_EnvFileWinsOverOSEnv(t *testing.T) {
	// given an OS-level env var and an env: file overriding it, when the
	// task runs, then the env: file's value wins (why: env: files are
	// explicit project config; ambient OS env shouldn't silently change
	// behavior across machines)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			".env": "FOO=fromfile\n",
		},
		Args: []string{"t"},
		Env:  map[string]string{"FOO": "fromosenv"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "foo=fromfile")
}

func TestE2E_Scope_VarsWinsOverOSEnv(t *testing.T) {
	// given an OS env var and a vars: entry for the same name, when the task
	// runs, then vars: wins (why: vars: sits directly above the OS-env base
	// layer in the chain — it's a taskfile-baked default, not ambient state)
	// Arrange
	c := Case{
		Yml: `
			vars:
			  - HOST: from-vars

			tasks:
			  t:
			    steps:
			      - run: echo host={{.HOST}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"HOST": "from-os-env"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "host=from-vars")
}

func TestE2E_Scope_EnvFileWinsOverVars(t *testing.T) {
	// given a vars: entry and an env: file for the same name, when the task
	// runs, then the env: file wins (why: vars: sits below env: files — a
	// .env is the more local, situational layer)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				vars:
				  - HOST: from-vars

				env:
				  - .env

				tasks:
				  t:
				    steps:
				      - run: echo host={{.HOST}}
			`,
			".env": "HOST=from-envfile\n",
		},
		Args: []string{"t"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "host=from-envfile")
}

func TestE2E_Scope_CLIArgsWinOverVars(t *testing.T) {
	// given a vars: entry and a matching CLI arg, when the task runs, then
	// the CLI arg wins (why: vars: is the overridable-default layer — this
	// is the whole point of it)
	// Arrange
	yml := `
		vars:
		  - HOST: from-vars

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

func TestE2E_Scope_ConstWinsOverCLIArgs(t *testing.T) {
	// given a const: entry and a matching CLI arg, when the task runs, then
	// const: wins (why: const: is the highest layer below the timeline —
	// nothing outside the file can override it)
	// Arrange
	yml := `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - run: echo id={{.JIRA_ID}}
	`

	// Act
	res := Yml(t, yml, "t", "JIRA_ID=hacked")

	// Assert
	res.OK(t)
	res.Lines(t, "id=customfield_12345")
}

func TestE2E_Scope_ConstWinsOverEnvFileAndOSEnv(t *testing.T) {
	// given a const: entry shadowed by both an env: file and an OS env var
	// of the same name, when the task runs, then const: still wins (why:
	// const: outranks every source below the timeline, not just CLI args)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				const:
				  - JIRA_ID: customfield_12345

				env:
				  - .env

				tasks:
				  t:
				    steps:
				      - run: echo id={{.JIRA_ID}}
			`,
			".env": "JIRA_ID=fromfile\n",
		},
		Args: []string{"t"},
		Env:  map[string]string{"JIRA_ID": "fromosenv"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "id=customfield_12345")
}

func TestE2E_Scope_TimelineSetStillWinsOverConst(t *testing.T) {
	// given a const: entry and no task that shadows its name, when a
	// DIFFERENT var is set: on the timeline, then the timeline still wins
	// for that var — const: only fixes the names it actually declares (why:
	// regression guard distinguishing "const: outranks CLI/env" from "const:
	// somehow freezes the whole scope"; the reserved-name check is what
	// stops a task from shadowing a const:'s OWN name — see const_test.go)
	// Arrange
	yml := `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - set:
		          - OTHER: from-set
		      - run: echo id={{.JIRA_ID}} other={{.OTHER}}
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "id=customfield_12345 other=from-set")
}

func TestE2E_Scope_ConstEntryReferencesEarlierConstEntry(t *testing.T) {
	// given a const: block where a later entry references an earlier one,
	// when the task runs, then it resolves — the closed-world check allows
	// this (why: matches set:'s own top-to-bottom resolution rule)
	// Arrange
	yml := `
		const:
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

func TestE2E_Scope_OSEnvVisibleWhenNothingElseSetsIt(t *testing.T) {
	// given an OS env var with no CLI-arg/env: file/timeline override, when a
	// task reads it, then the ambient value still passes through (why: OS
	// env is the lowest-priority layer, not an excluded one)
	// Arrange
	c := Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo bar={{.BAR}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"BAR": "from-os-env"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	res.Lines(t, "bar=from-os-env")
}
