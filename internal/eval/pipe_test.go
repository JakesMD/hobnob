package eval

import (
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
	got, err := EvalRunIntoPipe(`stdout[.KEY]`, `{"name":"Ada"}`, "", vars)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.String() != "Ada" {
		t.Fatalf("got %q, want Ada", got.String())
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
	got, err := EvalRunIntoPipe(`stdout[.JIRA.name]`, `{"customfield_12345":"Fix login"}`, "", vars)

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
	got, err := EvalRunIntoPipe(`stdout.name`, `{"name":"Ada"}`, "", nil)

	// Assert
	if err != nil {
		t.Fatalf("EvalRunIntoPipe: %v", err)
	}
	if got.String() != "Ada" {
		t.Fatalf("got %q, want Ada", got.String())
	}
}
