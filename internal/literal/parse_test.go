package literal

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		yml       string
		wantValue any
		wantRefs  []string
		wantErr   string
	}{
		{"given a scalar reference, when evaluated, then it keeps its type (why: a scalar entry is a single value)", ".A", []any{"a", "b"}, []string{"A"}, ""},
		{"given a typed scalar, when evaluated, then it reads as text (why: a scalar entry renders like any single field)", "5", "5", nil, ""},
		{"given a list of references and typed scalars, when evaluated, then each element keeps its type (why: a literal nests what it holds, as JSON does)", "[.N, b, 3, true, null]", []any{json.Number("3"), "b", json.Number("3"), true, nil}, []string{"N"}, ""},
		{"given a list holding a list reference, when evaluated, then it nests (why: without ... an element nests)", "[.A, d]", []any{[]any{"a", "b"}, "d"}, []string{"A"}, ""},
		{"given a spread beside a plain element, when evaluated, then its items join the list (why: [.A..., d] splices A in)", "[.A..., d]", []any{"a", "b", "d"}, []string{"A"}, ""},
		{"given a spread of a parenthesised pipeline, when evaluated, then the parsed list joins (why: (.X | json)... spreads parsed text)", "[(.JSON | json)..., x]", []any{"p", "q", "x"}, []string{"JSON"}, ""},
		{"given a spread inside a map's list, when evaluated, then it splices at that depth (why: a spread belongs to the list it is written in)", "{k: [.A..., z], n: .N}", map[string]any{"k": []any{"a", "b", "z"}, "n": json.Number("3")}, []string{"A", "N"}, ""},
		{"given text ending in ... in a list, when evaluated, then it stays one element (why: Loading... is not a spread)", "[Loading...]", []any{"Loading..."}, nil, ""},
		{"given a template leaf holding a quote, when evaluated, then the structure survives (why: leaves are assembled as values, never JSON text)", `['{{ .S }}"\']`, []any{`text"\`}, []string{"S"}, ""},
		{"given an alias inside a literal, when evaluated, then it reads the anchored node (why: YAML anchors work in literals)", "{a: &x [1], b: *x}", map[string]any{"a": []any{json.Number("1")}, "b": []any{json.Number("1")}}, nil, ""},
		{"given a spread of a string, when evaluated, then it errors naming | json (why: only a list can be spread)", "[.S...]", nil, []string{"S"}, "cannot spread .S: it is a string; pipe through | json to parse it first"},
		{"given a spread of an object, when evaluated, then it errors naming the kind (why: only a list can be spliced)", "[.O...]", nil, []string{"O"}, "cannot spread .O: it is an object"},
		{"given a spread of an unset variable, when evaluated, then it errors saying so (why: a typo should read as a typo)", "[.MISSING...]", nil, []string{"MISSING"}, "cannot spread .MISSING: it is null or unset"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := valueNode(t, tc.yml)

			// Act
			expr, err := Parse("B", n)

			// Assert
			assertErr(t, err, "")
			refs, err := expr.Refs()
			assertErr(t, err, "")
			if !reflect.DeepEqual(refs, tc.wantRefs) {
				t.Fatalf("Refs() = %v, want %v", refs, tc.wantRefs)
			}
			val, err := expr.Value(testVars())
			assertErr(t, err, tc.wantErr)
			if tc.wantErr == "" && !reflect.DeepEqual(val.Any(), tc.wantValue) {
				t.Fatalf("Value() = %#v, want %#v", val.Any(), tc.wantValue)
			}
		})
	}
}

func TestParse_LoadErrors(t *testing.T) {
	tests := []struct {
		name       string
		yml        string
		want       string
		wantSpread bool
	}{
		{"given a scalar spread, when parsed, then it errors (why: B: .A... has no list to splice into)", ".A...", `".A..." spreads a list, so it only works as a list element`, true},
		{"given a spread as an object field, when parsed, then it errors naming the field path (why: an object field holds one value)", "{k: .A...}", `B.k: ".A..." spreads a list`, true},
		{"given a nested spread as an object field, when parsed, then the path names every level (why: a deep failure must say where)", "[{k: .A...}]", `B[0].k: ".A..." spreads a list`, true},
		{"given a map key with template syntax, when parsed, then it errors (why: keys are names, never rendered)", "{'{{ .S }}': 1}", `B.{{ .S }}: variable name "{{ .S }}" must not contain template syntax`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := valueNode(t, tc.yml)

			// Act
			_, err := Parse("B", n)

			// Assert
			assertErr(t, err, tc.want)
			if errors.Is(err, ErrSpread) != tc.wantSpread {
				t.Fatalf("errors.Is(%v, ErrSpread) = %v, want %v", err, !tc.wantSpread, tc.wantSpread)
			}
		})
	}
}
