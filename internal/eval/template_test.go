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

// TestPluck covers only what internal/value/filter_test.go's TestFilterPluck
// (and friends) don't already: RFC 9535 selectors deep enough to need real
// template/JSONPath integration (negative index, wildcard, filter
// expressions), the object-leaf/chained-pluck case, and two behaviors with no
// direct-call equivalent. Basic path/nested/numeric-leaf/slice/missing-key
// cases live in filter_test.go — see that file for the base grammar.
func TestPluck(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
		wantErr  bool
	}{
		{
			name:     "given bool leaf, when plucked, then returns plain bool text (why: chainable into shell if: conditions)",
			tmpl:     `{{ .DATA | json | pluck "active" }}`,
			vars:     sv(map[string]string{"DATA": `{"active":true}`}),
			expected: "true",
		},
		{
			name:     "given object leaf, when plucked, then returns compact JSON so result stays chainable (why: pluck | pluck)",
			tmpl:     `{{ .DATA | json | pluck "meta" | pluck "region" }}`,
			vars:     sv(map[string]string{"DATA": `{"meta":{"region":"eu"}}`}),
			expected: "eu",
		},
		{
			name:    "given out-of-range array index, when plucked, then returns error (why: fail fast on bad path)",
			tmpl:    `{{ .DATA | json | pluck "items[5]" }}`,
			vars:    sv(map[string]string{"DATA": `{"items":["a"]}`}),
			wantErr: true,
		},
		{
			name:     "given present key with a default, when plucked, then returns the actual value, not the default (why: default only kicks in on failure)",
			tmpl:     `{{ .DATA | json | pluck "name" "fallback" }}`,
			vars:     sv(map[string]string{"DATA": `{"name":"hobnob"}`}),
			expected: "hobnob",
		},
		{
			name:     "given a negative index, when plucked, then counts from the end (why: RFC 9535 negative index selector)",
			tmpl:     `{{ .DATA | json | pluck "items[-1]" }}`,
			vars:     sv(map[string]string{"DATA": `{"items":["a","b","c"]}`}),
			expected: "c",
		},
		{
			name:     "given a wildcard path segment, when plucked, then returns every matched value as a JSON array (why: RFC 9535 wildcard selector)",
			tmpl:     `{{ .DATA | json | pluck "items[*].name" }}`,
			vars:     sv(map[string]string{"DATA": `{"items":[{"name":"a"},{"name":"b"}]}`}),
			expected: `["a","b"]`,
		},
		{
			name:     "given a filter expression matching one element, when plucked, then returns that element unwrapped, same as any single match (why: RFC 9535 filter selector; bare @.field is an existence test, not truthy, so the comparison is explicit)",
			tmpl:     `{{ .DATA | json | pluck "items[?@.active == true]" }}`,
			vars:     sv(map[string]string{"DATA": `{"items":[{"name":"a","active":true},{"name":"b","active":false}]}`}),
			expected: `{"active":true,"name":"a"}`,
		},
		{
			name:     "given a filter expression matching multiple elements, when plucked, then returns a JSON array (why: multi-match results stay chainable like keys/values)",
			tmpl:     `{{ .DATA | json | pluck "items[?@.active == true]" }}`,
			vars:     sv(map[string]string{"DATA": `{"items":[{"name":"a","active":true},{"name":"b","active":false},{"name":"c","active":true}]}`}),
			expected: `[{"active":true,"name":"a"},{"active":true,"name":"c"}]`,
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
