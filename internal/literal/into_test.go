package literal

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"hobnob/internal/eval"
	"hobnob/internal/value"
)

// fakeSource is an into: source holding stdout (a captured JSON list), exit
// and NAME, and nothing else.
func fakeSource() eval.IntoSource {
	return func(name string) (value.Value, error) {
		switch name {
		case "stdout":
			return value.Of([]any{"x", "y"}), nil
		case "exit":
			return value.Num(0), nil
		case "NAME":
			return value.Str(" child "), nil
		}
		return value.Missing(fmt.Sprintf("%q not found", name)), nil
	}
}

func TestParseInto(t *testing.T) {
	tests := []struct {
		name      string
		yml       string
		wantValue any
		wantErr   string
	}{
		{"given a source head, when evaluated, then it reads the source (why: into: names stdout, stderr, exit or a child var)", "exit", json.Number("0"), ""},
		{"given a source head with an accessor, when evaluated, then the accessor runs on the source (why: stdout[0] reads captured JSON)", "stdout[1]", "y", ""},
		{"given an unspaced filter after the head, when evaluated, then it runs (why: NAME|trim works unspaced)", "NAME|trim", "child", ""},
		{"given a dotted head, when evaluated, then the dot is optional (why: .NAME and NAME both name the source)", ".NAME | trim", "child", ""},
		{"given a template leaf, when evaluated, then it renders over the caller's vars (why: {{ }} reads the caller's scope, not the source)", "'{{ .S }}'", "text", ""},
		{"given a map of leaves, when evaluated, then each reads the source (why: into: literals build structure)", "{out: stdout, code: exit}", map[string]any{"out": []any{"x", "y"}, "code": json.Number("0")}, ""},
		{"given a spread of the source in a list, when evaluated, then its items join (why: into: takes spread like every literal)", "[stdout..., exit]", []any{"x", "y", json.Number("0")}, ""},
		{"given a template followed by ... in a list, when evaluated, then it stays one text element (why: a {{ }} leaf is text)", "['{{ .S }}...']", []any{"text..."}, ""},
		{"given a typed scalar in a list, when evaluated, then it passes through (why: only string leaves read the source)", "[stdout, 7]", []any{[]any{"x", "y"}, json.Number("7")}, ""},
		{"given a head the source lacks, when evaluated, then it errors as not found (why: an absent name is Missing, caught only by default)", "NOPE", nil, `"NOPE" not found`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := valueNode(t, tc.yml)

			// Act
			into, err := ParseInto("OUT", n)

			// Assert
			assertErr(t, err, "")
			val, err := into.Eval(fakeSource(), testVars())
			assertErr(t, err, tc.wantErr)
			if tc.wantErr == "" && !reflect.DeepEqual(val.Any(), tc.wantValue) {
				t.Fatalf("Eval() = %#v, want %#v", val.Any(), tc.wantValue)
			}
		})
	}
}

func TestParseInto_LoadErrors(t *testing.T) {
	tests := []struct {
		name       string
		yml        string
		want       string
		wantSpread bool
	}{
		{"given a scalar spread, when parsed, then it errors (why: OUT: stdout... has no list to splice into)", "stdout...", `"stdout..." spreads a list, so it only works as a list element`, true},
		{"given a spread as an object field, when parsed, then it errors naming the field path (why: an object field holds one value)", "{k: stdout...}", `OUT.k: "stdout..." spreads a list`, true},
		{"given a map key with template syntax, when parsed, then it errors (why: keys are names, never rendered)", "{'{{ .S }}': exit}", "must not contain template syntax", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := valueNode(t, tc.yml)

			// Act
			_, err := ParseInto("OUT", n)

			// Assert
			assertErr(t, err, tc.want)
			if errors.Is(err, ErrSpread) != tc.wantSpread {
				t.Fatalf("errors.Is(%v, ErrSpread) = %v, want %v", err, !tc.wantSpread, tc.wantSpread)
			}
		})
	}
}

func TestIntoLeaf(t *testing.T) {
	// given a bare into: name, when evaluated, then it reads the same name
	// from the source (why: a bare NAME is short for NAME: NAME)

	// Arrange
	into := IntoLeaf("NAME")

	// Act
	val, err := into.Eval(fakeSource(), testVars())

	// Assert
	assertErr(t, err, "")
	if val.Any() != " child " {
		t.Fatalf("Eval() = %#v, want %q", val.Any(), " child ")
	}
}
