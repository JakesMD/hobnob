package e2e

import "testing"

// The full scope precedence chain, highest to lowest: timeline (set:/get:
// default/loop/use:, in execution order) > CLI KEY=VALUE args > env: files >
// OS env. See GUIDE.md's "Variables > Precedence" table.

func TestE2E_Scope_SetStepWinsOverCLIArgs(t *testing.T) {
	// given a set: step and a matching CLI KEY=VALUE arg, when the task
	// runs, then the set: value wins (why: set: is a known value the task
	// author controls — a caller can't silently override it; get: is the
	// public API for caller input)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - set:
		          - HOST: localhost
		      - run: echo host={{.HOST}}
	`, "t", "HOST=remotehost")
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_GetDefaultLosesToCLIArgs(t *testing.T) {
	// given a get: step with a default: and a matching CLI KEY=VALUE arg,
	// when the task runs, then the CLI arg satisfies the prompt and wins
	// (why: this is how a caller overrides an overridable default — the
	// replacement for the old vars: '{{ .HOST | default "localhost" }}' idiom)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - HOST:
		              default: localhost
		      - run: echo host={{.HOST}}
	`, "t", "HOST=remotehost")
	res.OK(t)
	res.Lines(t, "host=remotehost")
}

func TestE2E_Scope_GetDefaultUsedWhenNoOverride(t *testing.T) {
	// given a get: step with a default: and no CLI arg, when the task runs,
	// then the default is used (why: this is the fallback half of the
	// overridable-default idiom)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - HOST:
		              default: localhost
		      - run: echo host={{.HOST}}
	`, "t")
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_SystemVarsAlwaysInjected(t *testing.T) {
	// given any taskfile, when a task echoes HOBNOB_FILE_DIR and
	// HOBNOB_INVOCATION_DIR, then both are populated (why: these built-ins
	// must always be available, with no opt-in step needed)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo dir={{.HOBNOB_FILE_DIR}}
	`, "t")
	res.OK(t)
	res.Lines(t, "dir="+res.Dir)
}

func TestE2E_Scope_CLIArgsWinOverEnvFile(t *testing.T) {
	// given an env: file setting a var and a matching CLI arg, when the task
	// runs, then the CLI arg wins (why: a caller's explicit override always
	// beats a sourced default)
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "foo=fromcli")
}

func TestE2E_Scope_SetStepWinsOverEnvFile(t *testing.T) {
	// given an env: file and a set: step assigning the same var, when the
	// task runs, then the set: value wins (why: the timeline sits above
	// every BuildScope-layer source, env: files included)
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "host=localhost")
}

func TestE2E_Scope_EnvFileWinsOverOSEnv(t *testing.T) {
	// given an OS-level env var and an env: file overriding it, when the
	// task runs, then the env: file's value wins (why: env: files are
	// explicit project config; ambient OS env shouldn't silently change
	// behavior across machines)
	res := Run(t, Case{
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
	})
	res.OK(t)
	res.Lines(t, "foo=fromfile")
}

func TestE2E_Scope_OSEnvVisibleWhenNothingElseSetsIt(t *testing.T) {
	// given an OS env var with no CLI-arg/env: file/timeline override, when a
	// task reads it, then the ambient value still passes through (why: OS
	// env is the lowest-priority layer, not an excluded one)
	res := Run(t, Case{
		Yml: `
			tasks:
			  t:
			    steps:
			      - run: echo bar={{.BAR}}
		`,
		Args: []string{"t"},
		Env:  map[string]string{"BAR": "from-os-env"},
	})
	res.OK(t)
	res.Lines(t, "bar=from-os-env")
}
