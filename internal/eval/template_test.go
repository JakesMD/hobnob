package eval

import (
	"testing"

	"hobnob/internal/value"
)

func TestEvalTemplate(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        string
		vars        map[string]value.Value
		expected    string
		expectError bool
	}{
		{
			name:     "given static value, when evaluated, then returns value unchanged (why: plain strings pass through)",
			tmpl:     "1048576",
			vars:     sv(map[string]string{}),
			expected: "1048576",
		},
		{
			name:     "given var reference, when var present, then substitutes value (why: set-vars derive from prior vars)",
			tmpl:     "{{.BASE}}",
			vars:     sv(map[string]string{"BASE": "hello"}),
			expected: "hello",
		},
		{
			name:     "given default func with empty var, when evaluated, then returns default (why: global vars use | default for fallback)",
			tmpl:     `{{.MISSING | default "fallback"}}`,
			vars:     sv(map[string]string{}),
			expected: "fallback",
		},
		{
			name:     "given default func with set var, when evaluated, then returns var value (why: env value overrides default)",
			tmpl:     `{{.TIMEOUT | default "30"}}`,
			vars:     sv(map[string]string{"TIMEOUT": "60"}),
			expected: "60",
		},
		{
			name:        "given invalid template syntax, when evaluated, then returns error (why: bad templates surface early)",
			tmpl:        "{{ .Unclosed",
			vars:        sv(map[string]string{}),
			expectError: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if test.expectError {
				if err == nil {
					t.Errorf("expected error but got nil (result: %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

// TestAccessor covers what internal/value/path_test.go's direct Path() calls
// don't already: real template/EvalTemplate integration (negative index,
// wildcard, chained dotted access, deferred-error-via-default). Basic
// path/nested/numeric-leaf/slice/missing-key semantics live in path_test.go
// — see that file for the base grammar. Replaces the old pluck-filter suite
// (TestPluck) — its two RFC 9535 filter-expression cases have no accessor
// equivalent and are deliberately not replaced; see DESIGN-PATH.md "What is
// lost".
func TestAccessor(t *testing.T) {
	dataVar := func(json string) map[string]value.Value {
		v, err := value.Parse(json)
		if err != nil {
			t.Fatalf("Parse(%q): %v", json, err)
		}
		return map[string]value.Value{"DATA": v}
	}

	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
		wantErr  bool
	}{
		{
			name:     "given bool leaf, when accessed, then returns plain bool text (why: chainable into shell if: conditions)",
			tmpl:     `{{ .DATA.active }}`,
			vars:     dataVar(`{"active":true}`),
			expected: "true",
		},
		{
			name:     "given nested object leaf, when accessed by chained dots, then returns the leaf directly (why: no re-parsing needed between levels, unlike pluck | pluck)",
			tmpl:     `{{ .DATA.meta.region }}`,
			vars:     dataVar(`{"meta":{"region":"eu"}}`),
			expected: "eu",
		},
		{
			name:    "given out-of-range array index, when accessed with no default, then returns error (why: fail fast on bad path)",
			tmpl:    `{{ .DATA.items[5] }}`,
			vars:    dataVar(`{"items":["a"]}`),
			wantErr: true,
		},
		{
			name:     "given present key with a default, when accessed, then returns the actual value, not the default (why: default only kicks in on a missing path)",
			tmpl:     `{{ .DATA.name | default "fallback" }}`,
			vars:     dataVar(`{"name":"hobnob"}`),
			expected: "hobnob",
		},
		{
			name:     "given a negative index, when accessed, then counts from the end",
			tmpl:     `{{ .DATA.items[-1] }}`,
			vars:     dataVar(`{"items":["a","b","c"]}`),
			expected: "c",
		},
		{
			name:     "given a wildcard step, when accessed, then returns every matched value as a JSON array",
			tmpl:     `{{ .DATA.items[*].name }}`,
			vars:     dataVar(`{"items":[{"name":"a"},{"name":"b"}]}`),
			expected: `["a","b"]`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if test.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}
