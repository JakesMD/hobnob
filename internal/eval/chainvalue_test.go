package eval

import (
	"strings"
	"testing"

	"hobnob/internal/value"
)

func TestEvalValue_AccessorPreservesType(t *testing.T) {
	// given a parsed object, when fields and elements are accessed, then each
	// keeps its own kind (why: numbers and bools must stay typed for
	// comparisons and later accessors)
	// Arrange
	resp, err := value.Parse(`{"count":3,"active":true,"items":["a","b","c"]}`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	vars := map[string]value.Value{"RESP": resp}

	tests := []struct {
		name string
		expr string
		kind value.Kind
		want string
	}{
		{"number field", "{{ .RESP.count }}", value.KindNumber, "3"},
		{"bool field", "{{ .RESP.active }}", value.KindBool, "true"},
		{"array index", "{{ .RESP.items[0] }}", value.KindString, "a"},
		{"negative index", "{{ .RESP.items[-1] }}", value.KindString, "c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got, err := EvalValue(tc.expr, vars)

			// Assert
			if err != nil {
				t.Fatalf("EvalValue(%q): %v", tc.expr, err)
			}
			if got.Kind() != tc.kind {
				t.Fatalf("EvalValue(%q).Kind() = %v, want %v", tc.expr, got.Kind(), tc.kind)
			}
			if got.String() != tc.want {
				t.Fatalf("EvalValue(%q) = %q, want %q", tc.expr, got.String(), tc.want)
			}
		})
	}
}

func TestEvalValue_ParenHeadPreservesType(t *testing.T) {
	// given an accessor on a parsed pipeline, when evaluated, then the
	// element keeps its own kind (why: values must stay typed through
	// accessors, not collapse to text)
	// Arrange
	vars := map[string]value.Value{"TAGS": value.Str(`["a","b"]`)}

	// Act
	got, err := EvalValue("{{ (.TAGS | json)[0] }}", vars)

	// Assert
	if err != nil {
		t.Fatalf("EvalValue: %v", err)
	}
	if got.Kind() != value.KindString || got.String() != "a" {
		t.Fatalf("got %v (%v), want a (string)", got.String(), got.Kind())
	}
}

func TestEvalValue_DefaultCatchesMissingPath(t *testing.T) {
	// given a missing path, when piped through | default, then the fallback
	// is returned (why: default is the sanctioned way to make a path
	// optional)
	// Arrange
	vars := map[string]value.Value{"RESP": mustParseVal(t, `{"a":1}`)}

	// Act
	got, err := EvalValue(`{{ .RESP.b | default "fallback" }}`, vars)

	// Assert
	if err != nil {
		t.Fatalf("EvalValue: %v", err)
	}
	if got.String() != "fallback" {
		t.Fatalf("got %q, want fallback", got.String())
	}
}

func TestEvalValue_MissingPathWithoutDefaultErrors(t *testing.T) {
	// given a missing path, when evaluated with no default, then it errors
	// (why: a typo in a path must not become an empty value)
	// Arrange
	vars := map[string]value.Value{"RESP": mustParseVal(t, `{"a":1}`)}

	// Act
	_, err := EvalValue("{{ .RESP.b }}", vars)

	// Assert
	if err == nil {
		t.Fatal("expected an error for a missing path with no default")
	}
}

func TestEvalValue_WrongKindNotCaughtByDefault(t *testing.T) {
	// given a plain (uncaptured) string masquerading as JSON, when accessed
	// with a default fallback, then it still errors naming | json (why:
	// wrong-kind is never catchable by default, so a missing | json can't
	// hide behind a fallback)
	// Arrange
	vars := map[string]value.Value{"STR": value.Str(`{"a":1}`)}

	// Act
	_, err := EvalValue(`{{ .STR.a | default "x" }}`, vars)

	// Assert
	if err == nil {
		t.Fatal("expected wrong-kind access to error even with | default")
	}
	if !strings.Contains(err.Error(), "| json") {
		t.Fatalf("expected error to name | json, got %v", err)
	}
}

func TestEvalTemplate_MissingPathAtBareRenderErrors(t *testing.T) {
	// given a missing path rendered inside text, when evaluated, then it
	// errors (why: the missing marker must never leak into rendered output)
	// Arrange
	vars := map[string]value.Value{"RESP": mustParseVal(t, `{"a":1}`)}

	// Act
	_, err := EvalTemplate("value: {{ .RESP.b }}", vars)

	// Assert
	if err == nil {
		t.Fatal("expected an error rendering a missing path with no filter")
	}
}

func TestEvalTemplate_EqOnMissingPathErrors(t *testing.T) {
	// given a missing path compared with eq, when evaluated, then it errors
	// (why: comparing the marker text would silently answer false)
	// Arrange
	vars := map[string]value.Value{"RESP": mustParseVal(t, `{"a":1}`)}

	// Act
	_, err := EvalTemplate(`{{ eq .RESP.b "x" }}`, vars)

	// Assert
	if err == nil {
		t.Fatal("expected eq on a missing path to error rather than compare the marker text")
	}
}

func mustParseVal(t *testing.T, s string) value.Value {
	t.Helper()
	v, err := value.Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return v
}
