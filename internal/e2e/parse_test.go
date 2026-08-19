package e2e

import "testing"

func TestE2E_Parse_EmptyFileIsValidWithZeroTasks(t *testing.T) {
	// given a completely empty taskfile, when run with no args, then it's
	// treated as valid with zero tasks rather than a parse error
	res := Yml(t, ``, "--list")
	res.OK(t)
	res.Out(t, "No tasks found")
}

func TestE2E_Parse_CommentOnlyFileIsValid(t *testing.T) {
	// given a taskfile containing only comments, when parsed, then it's
	// valid with zero tasks
	res := Yml(t, `
		# just a comment, no actual content
	`, "--list")
	res.OK(t)
	res.Out(t, "No tasks found")
}

func TestE2E_Parse_NullDocumentIsValid(t *testing.T) {
	// given a taskfile whose only content is a literal null, when parsed,
	// then it's valid with zero tasks (why: YAML's `null`/`~` is a
	// legitimate empty document, not malformed input)
	res := Yml(t, "null\n", "--list")
	res.OK(t)
	res.Out(t, "No tasks found")
}

func TestE2E_Parse_BareTopLevelKeysAreValid(t *testing.T) {
	// given vars:/modules:/tasks: present but each with no children, when
	// parsed, then it's valid and yields nothing (why: a bare key is a
	// legitimate way to say "none of these yet", not malformed structure)
	res := Yml(t, `
		vars:
		tasks:
	`, "--list")
	res.OK(t)
	res.Out(t, "No tasks found")
}

func TestE2E_Parse_MissingTaskfileErrors(t *testing.T) {
	// given --file pointing at a taskfile that doesn't exist, when run, then
	// it fails rather than silently falling back to discovery
	res := Run(t, Case{Yml: "tasks: {}", Args: []string{"--file", "does-not-exist.yml", "t"}, Discover: true})
	res.Fails(t)
}

func TestE2E_Parse_UnknownTaskNameErrors(t *testing.T) {
	// given a task name that isn't declared anywhere in the file, when run,
	// then it fails rather than silently doing nothing
	res := Yml(t, `
		tasks:
		  build:
		    steps:
		      - run: echo building
	`, "deploy")
	res.Fails(t)
}

func TestE2E_Parse_EnvBlockMalformedMultiKeyEntryErrors(t *testing.T) {
	// given an env: entry that's mis-indented into holding two path keys
	// instead of one, when parsed, then it's a hard error rather than a
	// silently-dropped or partially-loaded file
	res := Yml(t, `
		env:
		  - first.env:
		    second.env:
		tasks:
		  t:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
}

func TestE2E_Parse_ListSortsTasksAlphabeticallyRegardlessOfDeclarationOrder(t *testing.T) {
	// given tasks declared out of alphabetical order, when --list runs, then
	// they're shown sorted (why: visibleTaskNames sorts for display —
	// declaration order is preserved internally in cfg.TaskNames, but that's
	// not what the user sees in the menu)
	res := Yml(t, `
		tasks:
		  zebra:
		    steps:
		      - run: echo z
		  apple:
		    steps:
		      - run: echo a
	`, "--list")
	res.OK(t)
	res.Order(t, "apple", "zebra")
}

func TestE2E_Parse_TemplateSyntaxInIntoKeyRejected(t *testing.T) {
	// given an into: entry whose parent key contains template syntax, when
	// parsed, then it's rejected — same rule as set:, applied at the into:
	// site too
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - run: echo hi
		        into:
		          - "{{.BAD}}": stdout
	`, "t")
	res.Fails(t)
	res.Err(t, "must not contain template syntax")
}

func TestE2E_Parse_TemplateSyntaxInBareGetNameRejected(t *testing.T) {
	// given get: [ "{{.BAD}}" ] (bare shorthand form), when parsed, then
	// it's rejected — same rule applied at the bare get: site
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get: ["{{.BAD}}"]
	`, "t")
	res.Fails(t)
	res.Err(t, "must not contain template syntax")
}

func TestE2E_Parse_TemplateSyntaxInFullGetNameRejected(t *testing.T) {
	// given a full-syntax get: entry whose var name contains template
	// syntax, when parsed, then it's rejected — same rule applied at the
	// mapping get: site
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - get:
		          - "{{.BAD}}":
		              info: nope
	`, "t")
	res.Fails(t)
	res.Err(t, "must not contain template syntax")
}

func TestE2E_Parse_TemplateSyntaxInLoopMatrixKeyRejected(t *testing.T) {
	// given a loop: matrix key containing template syntax, when parsed,
	// then it's rejected — same rule applied at the matrix-key site
	res := Yml(t, `
		tasks:
		  t:
		    steps:
		      - loop:
		          "{{.BAD}}": [a, b]
		        steps:
		          - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "must not contain template syntax")
}
