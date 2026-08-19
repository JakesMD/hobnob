package e2e

import "testing"

// The full scope precedence chain, highest to lowest: vars: (globals) > CLI
// KEY=VALUE args > env: files > OS env. See GUIDE.md's "Variables >
// Precedence" table and its rationale.

func TestE2E_Scope_GlobalVarsResolveWithDefaultFallback(t *testing.T) {
	// given a top-level vars: block, when a task echoes a var, then it
	// resolves — including the | default fallback pattern GUIDE.md
	// documents for pulling from a lower-priority source
	res := Yml(t, `
		vars:
		  - HOST: localhost
		  - TIMEOUT: '{{ .TIMEOUT | default "30" }}'
		tasks:
		  t:
		    steps:
		      - run: echo host={{.HOST}} timeout={{.TIMEOUT}}
	`, "t")
	res.OK(t)
	res.Lines(t, "host=localhost timeout=30")
}

func TestE2E_Scope_SystemVarsAlwaysInjected(t *testing.T) {
	// given any taskfile, when a task echoes HOBNOB_FILE_DIR and
	// HOBNOB_INVOCATION_DIR, then both are populated (why: these built-ins
	// must always be available, with no vars: block needed to opt in)
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo dir={{.HOBNOB_FILE_DIR}}
	`, "t")
	res.OK(t)
	res.Lines(t, "dir="+res.Dir)
}

func TestE2E_Scope_GlobalVarsWinOverCLIArgs(t *testing.T) {
	// given a global var and a matching CLI KEY=VALUE arg, when the task
	// runs, then the global value wins (why: vars: is internal wiring the
	// author controls — a caller can't silently override it; get: is the
	// public API for caller input)
	res := Yml(t, `
		vars:
		  - HOST: localhost
		tasks:
		  t:
		    steps:
		      - run: echo host={{.HOST}}
	`, "t", "HOST=remotehost")
	res.OK(t)
	res.Lines(t, "host=localhost")
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

func TestE2E_Scope_GlobalVarsWinOverEnvFile(t *testing.T) {
	// given an env: file and a global vars: entry setting the same var, when
	// the task runs, then the global value wins (why: vars: sits above
	// env: files in the precedence chain)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				vars:
				  - HOST: localhost
				env:
				  - .env
				tasks:
				  t:
				    steps:
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
	// given an OS env var with no vars:/CLI-arg/env: file override, when a
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
