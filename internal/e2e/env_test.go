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

func TestE2E_Env_PathTemplateReadsCLIArg(t *testing.T) {
	// given an env: path built from a CLI arg, when run, then the path
	// resolves against it — an env: path can read CLI args (upward read, see
	// docs/adr/0001)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - "{{.STAGE}}.env"
				tasks:
				  t:
				    steps:
				      - run: echo target={{.DEPLOY_TARGET}}
			`,
			"prod.env": "DEPLOY_TARGET=prod-cluster\n",
		},
		Args: []string{"t", "STAGE=prod"},
	})
	res.OK(t)
	res.Lines(t, "target=prod-cluster")
}

func TestE2E_Env_PathTemplateReferencingVarsNameFailsAtLoad(t *testing.T) {
	// given an env: path template referencing a vars: name, when the file
	// loads, then it errors naming the rule (why: vars: can now read env:
	// files, so the reverse would be a cycle — this used to work and is
	// called out as a breaking change)
	res := Yml(t, `
		vars:
		  - STAGE: dev

		env:
		  - "{{.STAGE}}.env"

		tasks:
		  t:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, `env: "{{.STAGE}}.env" references .STAGE, declared in vars:`)
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

func TestE2E_Env_DotenvExpandsReferencesForScriptsLikeSource(t *testing.T) {
	// given a .env whose values reference earlier lines with $VAR and
	// ${VAR}, when a run: step reads them from its environment, then it sees
	// the expanded values (why: a script that expects `source .env` must get
	// the same values from env:)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo "url=$URL api=$API"
			`,
			".env": "HOST=example.com\nURL=https://$HOST/a\nAPI=\"${URL}/v2\"\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "url=https://example.com/a api=https://example.com/a/v2")
}

func TestE2E_Env_DotenvReferenceReadsEarlierFileAndOSEnv(t *testing.T) {
	// given a .env referencing a var from an earlier env: file and one from
	// the OS env, when run, then both expand (why: a .env commonly builds on
	// $HOME or a shared base file, which `source` would see too)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - first.env
				  - second.env
				tasks:
				  t:
				    steps:
				      - run: echo 'combo={{.COMBO}}'
			`,
			"first.env":  "FIRST=one\n",
			"second.env": "COMBO=$FIRST-$FROM_OS\n",
		},
		Env:  map[string]string{"FROM_OS": "two"},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "combo=one-two")
}

func TestE2E_Env_DotenvReferenceReadsFinalValueOfCLIArg(t *testing.T) {
	// given a .env that sets HOST and builds URL from it, when HOST is also
	// passed on the CLI, then URL uses the CLI value (why: upward read — a
	// layer reads the final value of a name a higher layer set, see
	// docs/adr/0001)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo 'url={{.URL}}'
			`,
			".env": "HOST=from-file\nURL=https://$HOST\n",
		},
		Args: []string{"t", "HOST=from-cli"},
	})
	res.OK(t)
	res.Lines(t, "url=https://from-cli")
}

func TestE2E_Env_DotenvDefaultAndUnsetReferences(t *testing.T) {
	// given references to unset names, plain, with ${NAME:-default} /
	// ${NAME-default}, and with a default that is itself a reference, when
	// run, then plain ones expand to empty and the default forms fall back
	// (why: these are the shell's defaulting forms, so a file written for
	// `source` reads the same)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo 'a=[{{.A}}] b={{.B}} c={{.C}} d=[{{.D}}] e={{.E}}'
			`,
			".env": "EMPTY=\"\"\nA=$NOPE\nB=${NOPE:-fallback}\nC=${NOPE-dash}\nD=${EMPTY-unused}\nBASE=b\nE=${NOPE:-${BASE}-x}\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "a=[] b=fallback c=dash d=[] e=b-x")
}

func TestE2E_Env_DotenvSingleQuotesAndEscapesStayLiteral(t *testing.T) {
	// given a dollar in single quotes, an escaped \$ in double quotes and
	// unquoted, and a $ not followed by a name, when run, then none of them
	// expand (why: quoting is how a file keeps a literal $, e.g. in a
	// password)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo 'single={{.SINGLE}} double={{.DOUBLE}} bare={{.BARE}} price={{.PRICE}}'
			`,
			".env": "HOST=h\nSINGLE='$HOST'\nDOUBLE=\"\\$HOST \\\"q\\\"\"\nBARE=\\$HOST\nPRICE=5$\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, `single=$HOST double=$HOST "q" bare=$HOST price=5$`)
}

func TestE2E_Env_DotenvStripsInlineCommentsOnlyOutsideQuotes(t *testing.T) {
	// given unquoted values with a space-led # comment, a # inside a word or
	// straight after the =, a value that is only a comment, and a quoted
	// value containing " #", when run, then only whitespace-led comments
	// outside quotes are stripped (why: that is where a shell starts a
	// comment, so a URL fragment or channel name survives)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo "a=[{{.A}}] b=[{{.B}}] c=[{{.C}}] d=[{{.D}}] e=[{{.E}}] f=[{{.F}}]"
			`,
			".env": "A=value # comment\nB=chan#1\nC=\"keep # this\"\nD='x' # after quote\nE= # only a comment\nF=#kept\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "a=[value] b=[chan#1] c=[keep # this] d=[x] e=[] f=[#kept]")
}

func TestE2E_Env_DotenvQuotedValueSpansLines(t *testing.T) {
	// given a double-quoted value that continues over several lines, when
	// run, then the value keeps its newlines and the next line still parses
	// (why: certificates and keys are often pasted in this way)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: printf '%s|%s\n' "$CERT" "$NEXT"
			`,
			".env": "CERT=\"line1\nline2\"\nNEXT=after\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "line1", "line2|after")
}

func TestE2E_Env_DotenvUnterminatedQuoteFailsNamingFileAndLine(t *testing.T) {
	// given a .env whose quoted value never closes, when the file loads,
	// then it errors naming the file and the line the value started on
	// (why: silently swallowing the rest of the file would hide every later
	// variable)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo hi
			`,
			".env": "OK=1\nBROKEN=\"never closed\nLATER=2\n",
		},
		Args: []string{"t"},
	})
	res.Fails(t)
	res.Err(t, ".env")
	res.Err(t, `line 2: BROKEN: unterminated " quote`)
}

func TestE2E_Env_DotenvDoubleQuotedBackslashNIsNewline(t *testing.T) {
	// given a double-quoted value with \n and a single-quoted one, when run,
	// then only the double-quoted \n becomes a newline (why: godotenv, which
	// Task's dotenv: uses, and Compose read it that way, so a one-line PEM
	// key works)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: printf '%s|%s\n' "$DQ" "$SQ"
			`,
			".env": "DQ=\"a\\nb\"\nSQ='c\\nd'\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "a", `b|c\nd`)
}

func TestE2E_Env_DotenvMalformedLineFailsNamingLine(t *testing.T) {
	// given a .env line that would otherwise lose text silently, when the
	// file loads, then it errors naming the line (why: a silently dropped or
	// truncated value is the bug this parser exists to avoid)
	cases := []struct {
		name, env, want string
	}{
		{"text after closing quote", "OK=1\nV='it\\'s'\n", `line 2: V: unexpected "s'" after closing ' quote`},
		{"no equals sign", "OK=1\nJUSTAWORD\n", `line 2: expected KEY=VALUE, got "JUSTAWORD"`},
		{"invalid key", "MY-KEY=1\n", `line 1: "MY-KEY" is not a valid variable name`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			c := Case{
				Files: Files{
					"hobnob.yml": `
						env:
						  - .env
						tasks:
						  t:
						    steps:
						      - run: echo hi
					`,
					".env": tc.env,
				},
				Args: []string{"t"},
			}

			// Act
			res := Run(t, c)

			// Assert
			res.Fails(t)
			res.Err(t, ".env", tc.want)
		})
	}
}

func TestE2E_Env_DotenvCRLFLineEndingsParse(t *testing.T) {
	// given a .env saved with Windows CRLF line endings, when run, then
	// values carry no trailing carriage return (why: a stray \r breaks
	// URLs and comparisons in ways that are invisible in output)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo "[$A][$B]"
			`,
			".env": "A=one\r\nB=\"two\"\r\n",
		},
		Args: []string{"t"},
	})
	res.OK(t)
	res.Lines(t, "[one][two]")
}

func TestE2E_Env_DotenvInModuleReadsImporterValue(t *testing.T) {
	// given a module whose .env builds URL from HOST, and an importing file
	// whose vars: sets HOST, when the module task runs, then URL uses the
	// importer's HOST (why: the importer's scope sits above a module's env:
	// layer, and HOST resolves to the importer's value at run time too)
	res := Run(t, Case{
		Files: Files{
			"hobnob.yml": `
				vars:
				  - HOST: from-root
				modules:
				  - api: ./api/hobnob.yml
			`,
			"api/hobnob.yml": `
				env:
				  - .env
				tasks:
				  t:
				    steps:
				      - run: echo 'host={{.HOST}} url={{.URL}}'
			`,
			"api/.env": "HOST=from-module\nURL=https://$HOST\n",
		},
		Args: []string{"api:t"},
	})
	res.OK(t)
	res.Lines(t, "host=from-root url=https://from-root")
}
