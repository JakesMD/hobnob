package literal

import (
	"reflect"
	"testing"

	"hobnob/internal/value"
)

func TestParseArgv(t *testing.T) {
	tests := []struct {
		name    string
		yml     string
		want    []string
		wantErr string
	}{
		{"given plain elements, when resolved, then each is one argument (why: the list form execs without a shell)", "[echo, 'a b']", []string{"echo", "a b"}, ""},
		{"given a list reference, when resolved, then its items splice in (why: argv lists splice arrays)", "[echo, .A, c]", []string{"echo", "a", "b", "c"}, ""},
		{"given a spread of a list reference, when resolved, then it splices the same (why: ... in argv is accepted and changes nothing)", "[echo, .A..., c]", []string{"echo", "a", "b", "c"}, ""},
		{"given an empty element, when resolved, then it stays one empty argument (why: later positions must not shift)", "[echo, '', c]", []string{"echo", "", "c"}, ""},
		{"given an empty list reference, when resolved, then it adds nothing (why: an empty splice has no items)", "[echo, .EMPTY, c]", []string{"echo", "c"}, ""},
		{"given a number reference, when resolved, then it is its text (why: argv is strings)", "[echo, .N]", []string{"echo", "3"}, ""},
		{"given a relative path, when resolved, then it stays text (why: ./infra is not a reference)", "[ls, ./infra]", []string{"ls", "./infra"}, ""},
		{"given an object reference, when resolved, then it errors naming the accessor fix (why: an object has no argv form)", "[echo, .O]", nil, "run: argument 1 resolved to an object; use an accessor (.VAR.field) to select the field you mean to pass"},
		{"given a list holding an object, when resolved, then it errors the same (why: each spliced item must be a scalar)", "[echo, .OBJS]", nil, "run: argument 1 resolved to an object"},
		{"given an empty command, when resolved, then it errors (why: there is nothing to execute)", "['', x]", nil, "run: no command to execute"},
		{"given an element that fails to evaluate, when resolved, then the error names its index (why: the user must find the bad element)", "[echo, '{{ .A.b }}']", nil, "run argv[1]:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			n := valueNode(t, tc.yml)
			vars := testVars()
			vars["EMPTY"] = value.Of([]any{})
			vars["OBJS"] = value.Of([]any{map[string]any{"k": "v"}})

			// Act
			argv, err := ParseArgv(n)

			// Assert
			assertErr(t, err, "")
			got, err := argv.Resolve(vars)
			assertErr(t, err, tc.wantErr)
			if tc.wantErr == "" && !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Resolve() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseArgv_EmptyListErrors(t *testing.T) {
	// given run: [], when parsed, then it errors (why: there is no command)

	// Act
	_, err := ParseArgv(valueNode(t, "[]"))

	// Assert
	assertErr(t, err, "run: list form needs at least one element")
}
