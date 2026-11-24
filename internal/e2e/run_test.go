package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestE2E_RunArgv_InjectionIsInert(t *testing.T) {
	// given a var holding shell metacharacters, when passed as a run: list
	// form element, then it arrives as one literal argument rather than being
	// parsed by a shell (why: this is the whole point of the argv form — no
	// sh -c means no word-splitting, no command substitution)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - NAME: "a; touch pwned"
		      - run: [echo, .NAME]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "a; touch pwned")
	if _, err := os.Stat(filepath.Join(res.Dir, "pwned")); !os.IsNotExist(err) {
		t.Fatalf("expected 'pwned' to not exist, stat err: %v", err)
	}
}

func TestE2E_RunArgv_SpaceStaysOneArgument(t *testing.T) {
	// given a var containing a space, when passed as a list-form element,
	// then it isn't word-split and stays exactly one argument (why: avoiding
	// shell quoting pitfalls is the point of the list form)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - NAME: "a b"
		      - run: [echo, .NAME]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "a b")
}

func TestE2E_RunArgv_ArrayElementSplices(t *testing.T) {
	// given an Array-typed var used as a list-form element, when the step
	// runs, then it splices into one argument per element rather than being
	// stringified into one (why: this is the direct payoff of typed values —
	// "-s -w" arrives as one argument, not two)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - FLAGS: ["-ldflags", "-s -w"]
		      - run: [printf, "%s\n", .FLAGS]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "-ldflags", "-s -w")
}

func TestE2E_RunArgv_EmptyElementPreserved(t *testing.T) {
	// given a var that resolves to an empty string, when it's a list-form
	// element positioned between two non-empty ones, then it's preserved as
	// an empty argument rather than dropped (why: dropping it would silently
	// shift every later positional argument)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - A: "1"
		          - B: ""
		          - C: "3"
		      - run: ["printf", "%s|%s|%s\n", .A, .B, .C]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "1||3")
}

func TestE2E_RunArgv_EmptyArraySplicesToNothing(t *testing.T) {
	// given an empty Array used as a list-form element, when the step runs,
	// then it contributes zero arguments rather than one empty argument (why:
	// an empty list of flags means no flags, not a blank one)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - FLAGS: []
		      - run: ["printf", "%s|%s\n", "a", .FLAGS, "b"]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "a|b")
}

func TestE2E_RunArgv_ObjectElementErrors(t *testing.T) {
	// given an Object-typed var used as a list-form element, when the step
	// runs, then it errors instead of being silently stringified (why: an
	// argument boundary should never be a guess)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - RESP: { a: 1 }
		      - run: [echo, .RESP]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "select the field")
}

func TestE2E_RunArgv_EmptyListIsParseError(t *testing.T) {
	// given run: as an empty YAML sequence, when the file is parsed, then it
	// fails (why: silently running nothing hides a broken step)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: []
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "run: list form needs at least one element")
}

func TestE2E_RunArgv_MappingIsParseError(t *testing.T) {
	// given run: as a YAML mapping, when the file is parsed, then it fails
	// rather than silently becoming an empty command (why: silently running
	// nothing hides a broken step)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: { a: b }
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.Fails(t)
	res.Err(t, "expected a command string or a list of arguments, got a mapping")
}

func TestE2E_RunArgv_IntoWorks(t *testing.T) {
	// given a list-form run: step, when into: captures its stdout, then
	// capture behaves identically to the string form (why: the two forms
	// differ only in how the command is built)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: [echo, hello]
		        into:
		          - OUT: stdout | trim
		      - run: [echo, .OUT]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "hello", "hello")
}

func TestE2E_RunArgv_DirWorks(t *testing.T) {
	// given a list-form run: step with dir:, when it runs, then dir:
	// resolution behaves identically to the string form (why: the two forms
	// differ only in how the command is built)
	// Arrange
	c := Case{
		Files: Files{
			"hobnob.yml": `
				tasks:
				  t:
				    steps:
				      - run: [pwd]
				        dir: sub
				        into:
				          - _PWD: stdout | trim
				      - run: [echo, "{{._PWD}}"]
			`,
			"sub/.keep": "",
		},
		Args: []string{"t"},
	}

	// Act
	res := Run(t, c)

	// Assert
	res.OK(t)
	want := filepath.Join(res.Dir, "sub")
	res.Lines(t, want, want)
}

func TestE2E_RunArgv_IfWorks(t *testing.T) {
	// given a list-form run: step with if:, when the condition is false, then
	// the step is skipped, same as the string form (why: the two forms differ
	// only in how the command is built)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: [echo, should-not-print]
		        if: '[ 1 -eq 2 ]'
		      - run: [echo, ran]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "ran")
}

func TestE2E_RunArgv_SecretMasked(t *testing.T) {
	// given a secret var referenced as a list-form element, when the step's
	// command is displayed, then it's masked, same as the string form (why:
	// secrets must not leak through the displayed command line; the test runs
	// true, not echo, because masking only covers the displayed command, not
	// the command's own output)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - TOKEN:
		              value: hunter2
		              secret: true
		      - run: [true, .TOKEN]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Masked(t, "hunter2")
}

func TestE2E_RunArgv_DisplayLineQuotesOnlyWhenNeeded(t *testing.T) {
	// given a list-form run: step with a plain element and a space-containing
	// element, when the command is echoed, then only the ambiguous element is
	// quoted (why: keeps the common case pasteable without noise)
	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - run: [echo, "hello world"]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Out(t, "run: [t] echo 'hello world'")
}

func TestE2E_RunArgv_SpreadIsANoOp(t *testing.T) {
	// given an argv element written with ..., when the step runs, then it
	// splices exactly as the bare element would (why: argv lists already
	// splice arrays, so both kinds of list share one spelling)

	// Arrange
	yml := `
		tasks:
		  t:
		    steps:
		      - set:
		          - WORDS: [hello, world]
		      - run: [echo, .WORDS..., "!"]
	`

	// Act
	res := Yml(t, yml, "t")

	// Assert
	res.OK(t)
	res.Lines(t, "hello world !")
}
