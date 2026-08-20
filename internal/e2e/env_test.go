package e2e

import (
	"strings"
	"testing"
)

func TestE2E_Env_DotenvParsingIgnoresCommentsAndBlanksHandlesExportAndQuotes(t *testing.T) {
	// given a .env file mixing comments, blank lines, export-prefixed lines,
	// and quoted values, when loaded, then only real KEY=VALUE lines are
	// parsed with quotes and the export prefix stripped
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}} baz={{.BAZ}} quoted={{.QUOTED}} single={{.SINGLE}}
			`,
			".env": "# a comment\n\nFOO=bar\nexport BAZ=qux\nQUOTED=\"hello world\"\nSINGLE='single quoted'\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=bar baz=qux quoted=hello world single=single quoted")
}

func TestE2E_Env_MissingFileWarnsAndContinues(t *testing.T) {
	// given an env: entry pointing at a file that doesn't exist, when run,
	// then it warns on stderr naming the missing path, rather than aborting
	// the task
	res := Yml(t, `
		env:
		  - does-not-exist.env
		tasks:
		  t:
		    steps:
		      - run: echo ran-anyway
	`, "t")
	res.OK(t)
	res.Lines(t, "ran-anyway")
	if !strings.Contains(res.Stderr, "does-not-exist.env") {
		t.Errorf("expected stderr to mention the missing path, got: %s", res.Stderr)
	}
	if !strings.Contains(res.Stderr, "not found") {
		t.Errorf("expected stderr to explain the file was not found, got: %s", res.Stderr)
	}
}

func TestE2E_Env_MissingEntryDoesNotBlockLaterEntries(t *testing.T) {
	// given a missing env: entry followed by a real one, when run, then the
	// later entry still loads despite the earlier miss
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - does-not-exist.env
				  - present.env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			"present.env": "FOO=bar\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=bar")
}

func TestE2E_Env_RelativePathResolvesAgainstTaskfileDirNotCwd(t *testing.T) {
	// given a relative env: path pointing into a subdirectory, when run,
	// then it resolves against the taskfile's own directory
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - sub/.env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			"sub/.env": "FOO=nested\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=nested")
}

func TestE2E_Env_LaterFileWinsOverEarlier(t *testing.T) {
	// given two env: files that both set the same var, when run, then the
	// later file's value wins — env: is an ordered list
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - first.env
				  - second.env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			"first.env":  "FOO=first\n",
			"second.env": "FOO=second\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=second")
}

func TestE2E_Env_NotSecretByDefault(t *testing.T) {
	// given an env: entry with no secret: setting, when its var is echoed,
	// then it prints unmasked — masking is opt-in for every env: entry
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
			".env": "FOO=bar\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=bar")
}

func TestE2E_Env_ExplicitSecretTrueMasksValue(t *testing.T) {
	// given an env: entry with secret: true, when its var is referenced in a
	// run: command, then it's masked in the displayed command
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env:
				      secret: true
				tasks:
				  t:
				    steps:
				      - run: ": {{.FOO}}"
			`,
			".env": "FOO=hunter2\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_Env_LaterEntrySecretOverrideDeterminesStatus(t *testing.T) {
	// given a secret: true .env entry setting FOO, followed by a plain
	// (non-secret) entry that overrides FOO, when run, then FOO is NOT
	// masked (why: secret status follows whichever entry the final value
	// actually came from, not an earlier one that happened to set it first)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - first.env:
				      secret: true
				  - second.env
				tasks:
				  t:
				    steps:
				      - run: echo foo={{.FOO}}
			`,
			"first.env":  "FOO=fromfirst\n",
			"second.env": "FOO=fromsecond\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "foo=fromsecond")
	res.NotOut(t, "****")
}

func TestE2E_Env_PathTemplateReferencesEarlierEntryVar(t *testing.T) {
	// given an earlier env: entry sets a var and a later entry's path is a
	// template referencing it, when run, then the later path resolves
	// against the accumulated vars — env: follows the same top-to-bottom
	// resolution as set:
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - first.env
				  - "{{.STAGE}}.env"
				tasks:
				  t:
				    steps:
				      - run: echo target={{.DEPLOY_TARGET}}
			`,
			"first.env": "STAGE=prod\n",
			"prod.env":  "DEPLOY_TARGET=prod-cluster\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "target=prod-cluster")
}
