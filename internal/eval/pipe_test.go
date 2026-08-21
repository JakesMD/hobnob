package eval

import (
	"strings"
	"testing"

	"hobnob/internal/value"
)

func TestEvalRunIntoPipe_DynamicKeyResolvesAgainstCallerVars(t *testing.T) {
	// given a run: into: pipe whose accessor uses a dynamic key ([.KEY]),
	// when evaluated, then the key resolves against the caller's scope (why:
	// regression guard — evalChainOn used to evaluate the accessor against a
	// vars map holding only the captured source, so any [.KEY] silently saw
	// an empty scope instead of the caller's vars)

	// Arrange
	vars := map[string]value.Value{"KEY": value.Str("name")}

	// Act
	got, err := EvalRunIntoPipe(`stdout[.KEY]`, `{"name":"Ada"}`, "", 0, vars)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.String() != "Ada" {
		t.Fatalf("got %q, want Ada", got.String())
	}
}

func TestEvalRunIntoPipe_ExitSourceIsTypedNumber(t *testing.T) {
	// given a run: into: pipe reading the exit source, when evaluated, then
	// it yields a typed Number, not a stringified code (why: lets
	// {{ ne .CODE 0 }} compare numerically, not lexically)

	// Act
	got, err := EvalRunIntoPipe(`exit`, "", "", 3, nil)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.Kind() != value.KindNumber {
		t.Fatalf("got kind %v, want KindNumber", got.Kind())
	}
	if got.String() != "3" {
		t.Fatalf("got %q, want 3", got.String())
	}
}

func TestEvalRunIntoPipe_UnknownSourceNamesAllThree(t *testing.T) {
	// given an into: source that isn't stdout/stderr/exit, when evaluated,
	// then the error names all three valid sources

	// Act
	_, err := EvalRunIntoPipe(`exitcode`, "", "", 0, nil)

	// Assert
	if err == nil || !strings.Contains(err.Error(), "must be stdout, stderr or exit") {
		t.Fatalf("got %v, want error naming stdout, stderr or exit", err)
	}
}

func TestEvalRunIntoPipe_DynamicKeyNestedInCallerObject(t *testing.T) {
	// given a dynamic key nested inside an object var (.JIRA.name), when
	// evaluated as a run: into: accessor, then it resolves the same way it
	// does in a {{ }} template

	// Arrange
	jira := mustParseVal(t, `{"name":"customfield_12345"}`)
	vars := map[string]value.Value{"JIRA": jira}

	// Act
	got, err := EvalRunIntoPipe(`stdout[.JIRA.name]`, `{"customfield_12345":"Fix login"}`, "", 0, vars)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.String() != "Fix login" {
		t.Fatalf("got %q, want %q", got.String(), "Fix login")
	}
}

func TestEvalRunIntoPipe_StaticAccessorStillWorksWithNilVars(t *testing.T) {
	// given a run: into: pipe with no dynamic key, when evaluated with no
	// caller vars available, then it still resolves (why: guards against the
	// fix regressing the common static-accessor case)

	// Act
	got, err := EvalRunIntoPipe(`stdout.name`, `{"name":"Ada"}`, "", 0, nil)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.String() != "Ada" {
		t.Fatalf("got %q, want Ada", got.String())
	}
}
