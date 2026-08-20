package e2e

import "testing"

func TestE2E_Const_ClosedWorldRejectsOutsideReference(t *testing.T) {
	// given a const: entry referencing a var that isn't an earlier const:
	// key or a builtin, when the file loads, then it errors naming vars:
	// as the fix (why: this is the check that keeps const: from rotting
	// back into the old pre-v0.3 vars: block, where an entry could silently
	// read a lower-priority layer disguised as a fixed value)
	res := Yml(t, `
		const:
		  - HOST: '{{ .HOST | default "x" }}'

		tasks:
		  t:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, "const: HOST references .HOST", "use vars:")
}

func TestE2E_Const_ClosedWorldAllowsBuiltins(t *testing.T) {
	// given a const: entry referencing HOBNOB_FILE_DIR, when the file loads,
	// then it's allowed — builtins are always safe to reference
	res := Yml(t, `
		const:
		  - CONFIG_DIR: "{{.HOBNOB_FILE_DIR}}/config"

		tasks:
		  t:
		    steps:
		      - run: echo dir={{.CONFIG_DIR}}
	`, "t")
	res.OK(t)
	res.Lines(t, "dir="+res.Dir+"/config")
}

func TestE2E_Const_DuplicateKeyInOneBlockErrors(t *testing.T) {
	// given two const: entries with the same key in one block, when the
	// file loads, then it errors (why: unlike set:, where a step
	// re-assigning a var later in the timeline is ordinary, const: is a
	// one-time declaration — two entries with the same key can only be a
	// copy-paste mistake)
	res := Yml(t, `
		const:
		  - JIRA_ID: first
		  - JIRA_ID: second

		tasks:
		  t:
		    steps:
		      - run: echo {{.JIRA_ID}}
	`, "t")
	res.Fails(t)
	res.Err(t, "const: JIRA_ID declared twice")
}

func TestE2E_Const_ReservedNameRejectsSetCollision(t *testing.T) {
	// given a const: entry and a task that set:'s the same name, when the
	// file loads, then it errors (why: without this, const: would only be
	// constant from OUTSIDE the file — the timeline still outranks it in
	// the precedence chain, so a task's own set: could otherwise overwrite
	// it silently)
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - set:
		          - JIRA_ID: hacked
	`, "t")
	res.Fails(t)
	res.Err(t, "task t sets JIRA_ID, declared in const:")
}

func TestE2E_Const_ReservedNameRejectsGetCollision(t *testing.T) {
	// given a const: entry and a task that get:'s the same name, when the
	// file loads, then it errors — same rule as set:, extended to every
	// step kind that can introduce a scope var
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - get: [JIRA_ID]
	`, "t")
	res.Fails(t)
	res.Err(t, "task t sets JIRA_ID, declared in const:")
}

func TestE2E_Const_ReservedNameRejectsIntoCollision(t *testing.T) {
	// given a const: entry and a call: step whose into: writes the same
	// name, when the file loads, then it errors
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - call: sub
		        into:
		          - JIRA_ID: .X

		  sub:
		    steps:
		      - set:
		          - X: 1
	`, "t")
	res.Fails(t)
	res.Err(t, "task t sets JIRA_ID, declared in const:")
}

func TestE2E_Const_ReservedNameRejectsMatrixLoopCollision(t *testing.T) {
	// given a const: entry and a matrix loop: whose var name matches it,
	// when the file loads, then it errors
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - loop:
		          JIRA_ID: [a, b]
		        steps:
		          - run: echo {{.JIRA_ID}}
	`, "t")
	res.Fails(t)
	res.Err(t, "task t sets JIRA_ID, declared in const:")
}

func TestE2E_Const_ReservedNameCheckedInsideNestedLoopSteps(t *testing.T) {
	// given a const: entry and a set: step nested inside a loop:'s own
	// steps, when the file loads, then it still errors — the check
	// recurses into nested step bodies, not just a task's top-level steps
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - loop: [a, b]
		        steps:
		          - set:
		              - JIRA_ID: hacked
	`, "t")
	res.Fails(t)
	res.Err(t, "task t sets JIRA_ID, declared in const:")
}

func TestE2E_Const_ReservedNameIsPerFileNotGlobal(t *testing.T) {
	// given a const: entry in the root file and a DIFFERENT task in the
	// SAME file that doesn't touch that name, when the file loads, then it
	// loads fine — the check only rejects an actual collision, not the
	// mere presence of a const: block elsewhere in the file
	res := Yml(t, `
		const:
		  - JIRA_ID: customfield_12345

		tasks:
		  t:
		    steps:
		      - set:
		          - OTHER: fine
		      - run: echo {{.OTHER}}
	`, "t")
	res.OK(t)
	res.Lines(t, "fine")
}

func TestE2E_Const_UnknownTopLevelKeyErrors(t *testing.T) {
	// given a typo'd top-level key, when the file loads, then it errors
	// naming the bad key (why: the safety net for the vars: rename —
	// before this, an unrecognized key was silently dropped, so a v0.2
	// file's vars: block would parse "successfully" with every var
	// rendering empty instead of failing loudly)
	res := Yml(t, `
		taks:
		  t:
		    steps:
		      - run: echo hi
	`, "t")
	res.Fails(t)
	res.Err(t, `unknown top-level key "taks"`)
}
