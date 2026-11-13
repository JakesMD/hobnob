package literal

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"hobnob/internal/value"
)

// valueNode returns the YAML node written as the value of a one-key map, so a
// test can spell a field the way a taskfile would.
func valueNode(t *testing.T, src string) *yaml.Node {
	t.Helper()
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte("v: "+src), &doc); err != nil {
		t.Fatalf("yaml %q: %v", src, err)
	}
	return doc.Content[0].Content[1]
}

// assertErr checks err against want: nil when want is "", otherwise an error
// containing want.
func assertErr(t *testing.T, err error, want string) {
	t.Helper()
	if want == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want one containing %q", err, want)
	}
}

func testVars() map[string]value.Value {
	return map[string]value.Value{
		"A":    value.Of([]any{"a", "b"}),
		"N":    value.Num(3),
		"S":    value.Str("text"),
		"O":    value.Of(map[string]any{"k": "v"}),
		"_P":   value.Str("private"),
		"JSON": value.Str(`["p", "q"]`),
	}
}

func TestSingle(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantValue any
		wantText  string
		wantRefs  []string
	}{
		{"given a bare reference, when evaluated, then it keeps the variable's type (why: a reference holds a value, a template renders text)", ".N", json.Number("3"), "3", []string{"N"}},
		{"given a bare reference with an accessor chain, when evaluated, then the accessor result keeps its type (why: options: .CFG.items[0] reads into structure)", ".A[1]", "b", "b", []string{"A"}},
		{"given a bare reference with a filter, when evaluated, then the filter runs (why: .S | upper drops its braces like any reference)", ".S | upper", "TEXT", "TEXT", []string{"S"}},
		{"given a private bare reference, when evaluated, then it resolves (why: ._NAME is a reference to a private variable)", "._P", "private", "private", []string{"_P"}},
		{"given a template, when evaluated, then it renders text (why: {{ }} is the text form)", "{{ .S }}-x", "text-x", "text-x", []string{"S"}},
		{"given a parent-relative path, when evaluated, then it stays text (why: ../tests is not a reference)", "../tests", "../tests", "../tests", nil},
		{"given a dotted file name, when evaluated, then it stays text (why: .env.local is not a reference)", ".env.local", ".env.local", ".env.local", nil},
		{"given a mixed-case dotfile, when evaluated, then it stays text (why: .DS_Store has a lowercase tail in its name)", ".DS_Store", ".DS_Store", ".DS_Store", nil},
		{"given a lone dot, when evaluated, then it stays text (why: a reference needs a name)", ".", ".", ".", nil},
		{"given a relative path, when evaluated, then it stays text (why: ./infra is not a reference)", "./infra", "./infra", "./infra", nil},
		{"given a dotfile name, when evaluated, then it stays text (why: lowercase-leading .git is not a reference)", ".git", ".git", ".git", nil},
		{"given a name that runs into lowercase, when evaluated, then it stays text (why: .Sx is not an upper-case variable name)", ".Sx", ".Sx", ".Sx", nil},
		{"given a reference with a dangling pipe, when evaluated, then it stays text (why: a pipe needs a filter after it)", ".S |", ".S |", ".S |", nil},
		{"given text ending in ..., when evaluated, then it stays text (why: Loading... is not a spread)", "Loading...", "Loading...", "Loading...", nil},
		{"given a template followed by ..., when evaluated, then it renders text (why: a template is how text ending in ... is written)", "{{ .S }}...", "text...", "text...", []string{"S"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			vars := testVars()

			// Act
			expr, err := Single(tc.raw)

			// Assert
			assertErr(t, err, "")
			refs, err := expr.Refs()
			assertErr(t, err, "")
			if !reflect.DeepEqual(refs, tc.wantRefs) {
				t.Fatalf("Refs() = %v, want %v", refs, tc.wantRefs)
			}
			val, err := expr.Value(vars)
			assertErr(t, err, "")
			if !reflect.DeepEqual(val.Any(), tc.wantValue) {
				t.Fatalf("Value() = %#v, want %#v", val.Any(), tc.wantValue)
			}
			text, err := expr.Text(vars)
			assertErr(t, err, "")
			if text != tc.wantText {
				t.Fatalf("Text() = %q, want %q", text, tc.wantText)
			}
		})
	}
}

func TestSingle_Empty(t *testing.T) {
	// given an empty field, when parsed, then it is the zero Expr and
	// evaluates to empty text (why: an absent field and "" behave alike)

	// Act
	expr, err := Single("")

	// Assert
	assertErr(t, err, "")
	if !expr.IsZero() || expr != (Expr{}) {
		t.Fatalf("Single(\"\") = %#v, want the zero Expr", expr)
	}
	text, err := expr.Text(testVars())
	assertErr(t, err, "")
	if text != "" {
		t.Fatalf("Text() = %q, want empty", text)
	}
}

func TestSingle_SpreadErrors(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"given a bare reference spread, when parsed, then it errors (why: a single value has no list to splice into)", ".A...", `".A..." spreads a list, so it only works as a list element; drop the ... or wrap it in [ ]`},
		{"given an accessor chain spread, when parsed, then it errors (why: spread applies to the whole element reference)", " .A[*].name... ", `".A[*].name..." spreads a list`},
		{"given a parenthesised pipeline spread, when parsed, then it errors (why: (.X | json)... is a reference too)", "(.JSON | json)...", `"(.JSON | json)..." spreads a list`},
		{"given a filtered reference spread, when parsed, then it errors (why: filters are part of the element reference)", ".JSON | json...", `".JSON | json..." spreads a list`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			_, err := Single(tc.raw)

			// Assert
			assertErr(t, err, tc.want)
			if !errors.Is(err, ErrSpread) {
				t.Fatalf("error %v does not wrap ErrSpread", err)
			}
		})
	}
}

func TestSingle_String(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"given a bare reference, when printed, then its braces are restored (why: error messages print the stored template)", ".A[0] | upper", "{{.A[0] | upper}}"},
		{"given a filter tail that opens a template, when printed, then it is stored as written (why: a brace in the tail is not reference syntax)", ".S | upper {{", ".S | upper {{"},
		{"given a template, when printed, then it is stored as written (why: only bare references are rewritten)", "{{ .S }}", "{{ .S }}"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			expr, err := Single(tc.raw)

			// Assert
			assertErr(t, err, "")
			if expr.String() != tc.want {
				t.Fatalf("String() = %q, want %q", expr.String(), tc.want)
			}
		})
	}
}
