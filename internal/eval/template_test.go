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

func TestTrim(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given string with leading/trailing whitespace, when trim called, then whitespace removed (why: shell output often has padding)",
			tmpl:     `{{ "  hello  " | trim }}`,
			vars:     sv(map[string]string{}),
			expected: "hello",
		},
		{
			name:     "given string with no whitespace, when trim called, then unchanged (why: trim must be a no-op on clean strings)",
			tmpl:     `{{ "hello" | trim }}`,
			vars:     sv(map[string]string{}),
			expected: "hello",
		},
		{
			name:     "given var with trailing newline, when trim called, then newline removed (why: piping run output without splitLines)",
			tmpl:     `{{ .VAL | trim }}`,
			vars:     sv(map[string]string{"VAL": "value\n"}),
			expected: "value",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given comma-separated string, when split on comma, then returns JSON array (why: generic delimiter support)",
			tmpl:     `{{ split "," "a,b,c" }}`,
			vars:     sv(map[string]string{}),
			expected: `["a","b","c"]`,
		},
		{
			name:     "given newline-separated string, when split on newline, then returns JSON array (why: equivalent to splitLines without trim)",
			tmpl:     "{{ split \"\\n\" \"x\\ny\\nz\" }}",
			vars:     sv(map[string]string{}),
			expected: `["x","y","z"]`,
		},
		{
			name:     "given string with empty tokens, when split called, then empty tokens filtered (why: avoid empty ITEM in for loop)",
			tmpl:     `{{ split "," "a,,b" }}`,
			vars:     sv(map[string]string{}),
			expected: `["a","b"]`,
		},
		{
			name:     "given var with delimited string, when split called via template, then returns JSON array (why: real usage pattern)",
			tmpl:     `{{ .PKGS | split "," }}`,
			vars:     sv(map[string]string{"PKGS": "foo,bar,baz"}),
			expected: `["foo","bar","baz"]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestLines(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given multiline string, when lines called, then returns JSON array (why: for loop: iterates a typed Array's elements directly)",
			tmpl:     `{{ "line1\nline2\nline3" | lines }}`,
			vars:     sv(map[string]string{}),
			expected: `["line1","line2","line3"]`,
		},
		{
			name:     "given string with blank lines, when lines called, then blank lines filtered (why: empty entries would produce empty ITEM in for loop)",
			tmpl:     `{{ "a\n\nb\n\nc" | lines }}`,
			vars:     sv(map[string]string{}),
			expected: `["a","b","c"]`,
		},
		{
			name:     "given string with trailing newline, when lines called, then trailing newline ignored (why: shell output always ends with newline)",
			tmpl:     "{{ \"hello\\nworld\\n\" | lines }}",
			vars:     sv(map[string]string{}),
			expected: `["hello","world"]`,
		},
		{
			name:     "given single line string, when lines called, then returns single-element JSON array (why: consistent type for for loop)",
			tmpl:     `{{ "only" | lines }}`,
			vars:     sv(map[string]string{}),
			expected: `["only"]`,
		},
		{
			name:     "given string with leading/trailing spaces on lines, when lines called, then lines are trimmed (why: shell output may have whitespace padding)",
			tmpl:     `{{ "  foo  \n  bar  " | lines }}`,
			vars:     sv(map[string]string{}),
			expected: `["foo","bar"]`,
		},
		{
			name:     "given var holding multiline string, when lines called via template, then returns JSON array (why: real usage pattern with run output stored in var)",
			tmpl:     `{{ .PKGS | lines }}`,
			vars:     sv(map[string]string{"PKGS": "pkg_a\npkg_b\npkg_c"}),
			expected: `["pkg_a","pkg_b","pkg_c"]`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestUpper(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given lowercase string, when upper called, then returns uppercase (why: case normalisation for comparisons)",
			tmpl:     `{{ "hello world" | upper }}`,
			vars:     sv(map[string]string{}),
			expected: "HELLO WORLD",
		},
		{
			name:     "given mixed-case var, when upper called via template, then returns uppercase (why: real usage pattern)",
			tmpl:     `{{ .VAL | upper }}`,
			vars:     sv(map[string]string{"VAL": "Hello"}),
			expected: "HELLO",
		},
		{
			name:     "given already-uppercase string, when upper called, then unchanged (why: upper is idempotent)",
			tmpl:     `{{ "DONE" | upper }}`,
			vars:     sv(map[string]string{}),
			expected: "DONE",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given uppercase string, when lower called, then returns lowercase (why: case normalisation for comparisons)",
			tmpl:     `{{ "HELLO WORLD" | lower }}`,
			vars:     sv(map[string]string{}),
			expected: "hello world",
		},
		{
			name:     "given mixed-case var, when lower called via template, then returns lowercase (why: real usage pattern)",
			tmpl:     `{{ .VAL | lower }}`,
			vars:     sv(map[string]string{"VAL": "Hello"}),
			expected: "hello",
		},
		{
			name:     "given already-lowercase string, when lower called, then unchanged (why: lower is idempotent)",
			tmpl:     `{{ "done" | lower }}`,
			vars:     sv(map[string]string{}),
			expected: "done",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestJSONEscape(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
	}{
		{
			name:     "given a value with a double quote, when jsonEscape called, then quote is backslash-escaped (why: embedding raw data inside hand-written JSON text must not terminate the string early)",
			tmpl:     `{"name": "{{ .VAL | jsonEscape }}"}`,
			vars:     sv(map[string]string{"VAL": `he said "hi"`}),
			expected: `{"name": "he said \"hi\""}`,
		},
		{
			name:     "given a value with backslashes, when jsonEscape called, then each backslash is doubled (why: a Windows path must survive as valid JSON without the caller manually doubling anything)",
			tmpl:     `{"path": "{{ .VAL | jsonEscape }}"}`,
			vars:     sv(map[string]string{"VAL": `C:\Users\foo`}),
			expected: `{"path": "C:\\Users\\foo"}`,
		},
		{
			name:     "given a value with no special characters, when jsonEscape called, then unchanged (why: jsonEscape must be a no-op on already-safe strings)",
			tmpl:     `{{ "hello" | jsonEscape }}`,
			vars:     sv(map[string]string{}),
			expected: "hello",
		},
		{
			name:     "given a value with a newline, when jsonEscape called, then newline becomes \\n (why: a literal newline is invalid inside a JSON string)",
			tmpl:     `{{ .VAL | jsonEscape }}`,
			vars:     sv(map[string]string{"VAL": "line1\nline2"}),
			expected: `line1\nline2`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalTemplate(test.tmpl, test.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != test.expected {
				t.Errorf("got %q, want %q", got, test.expected)
			}
		})
	}
}

func TestFirst(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
		wantErr  bool
	}{
		{
			name:     "given JSON array var, when first called via pipe, then returns first element (why: extracts first item from lines/split output)",
			tmpl:     `{{ .LIST | json | first }}`,
			vars:     sv(map[string]string{"LIST": `["v0.3.0","v0.2.1","v0.1.0"]`}),
			expected: "v0.3.0",
		},
		{
			name:     "given inline JSON array, when first called, then returns first element (why: consistent with var usage)",
			tmpl:     `{{ "[\"a\",\"b\",\"c\"]" | json | first }}`,
			vars:     sv(map[string]string{}),
			expected: "a",
		},
		{
			name:     "given empty JSON array, when first called, then returns empty string (why: no panic on empty list)",
			tmpl:     `{{ .LIST | json | first }}`,
			vars:     sv(map[string]string{"LIST": `[]`}),
			expected: "",
		},
		{
			name:    "given a string that isn't an array, when first called, then returns error (why: first requires an array — it never auto-parses a string, even one holding valid JSON — see | json)",
			tmpl:    `{{ first "not-json" }}`,
			vars:    sv(map[string]string{}),
			wantErr: true,
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

func TestPluck(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
		wantErr  bool
	}{
		{
			name:     "given top-level key, when plucked, then returns string value (why: basic flat-map lookup)",
			tmpl:     `{{ .DATA | json | pluck "name" }}`,
			vars:     sv(map[string]string{"DATA": `{"name":"hobnob"}`}),
			expected: "hobnob",
		},
		{
			name:     "given nested path with array index, when plucked, then traverses object and array (why: real API responses nest objects inside arrays)",
			tmpl:     `{{ .DATA | json | pluck "data.items[0].name" }}`,
			vars:     sv(map[string]string{"DATA": `{"data":{"items":[{"name":"first"},{"name":"second"}]}}`}),
			expected: "first",
		},
		{
			name:     "given numeric leaf, when plucked, then returns plain number text (why: no float precision artifacts like 3.0)",
			tmpl:     `{{ .DATA | json | pluck "count" }}`,
			vars:     sv(map[string]string{"DATA": `{"count":3}`}),
			expected: "3",
		},
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
			name:    "given missing key, when plucked, then returns error (why: fail fast rather than silently returning empty)",
			tmpl:    `{{ .DATA | json | pluck "missing" }}`,
			vars:    sv(map[string]string{"DATA": `{"name":"hobnob"}`}),
			wantErr: true,
		},
		{
			name:    "given out-of-range array index, when plucked, then returns error (why: fail fast on bad path)",
			tmpl:    `{{ .DATA | json | pluck "items[5]" }}`,
			vars:    sv(map[string]string{"DATA": `{"items":["a"]}`}),
			wantErr: true,
		},
		{
			name:    "given malformed JSON, when plucked, then returns error (why: fail fast on bad source data)",
			tmpl:    `{{ .DATA | json | pluck "name" }}`,
			vars:    sv(map[string]string{"DATA": `not-json`}),
			wantErr: true,
		},
		{
			name:     "given missing key with a default, when plucked, then returns the default instead of erroring (why: caller opts into a fallback per-call)",
			tmpl:     `{{ .DATA | json | pluck "missing" "fallback" }}`,
			vars:     sv(map[string]string{"DATA": `{"name":"hobnob"}`}),
			expected: "fallback",
		},
		{
			name:     "given malformed JSON with a default, when plucked, then returns the default instead of erroring (why: bad source data is also covered by the fallback — now on json itself, since pluck never sees unparsed text)",
			tmpl:     `{{ .DATA | json "fallback" | pluck "name" "fallback" }}`,
			vars:     sv(map[string]string{"DATA": `not-json`}),
			expected: "fallback",
		},
		{
			name:     "given present key with a default, when plucked, then returns the actual value, not the default (why: default only kicks in on failure)",
			tmpl:     `{{ .DATA | json | pluck "name" "fallback" }}`,
			vars:     sv(map[string]string{"DATA": `{"name":"hobnob"}`}),
			expected: "hobnob",
		},
		{
			name:     "given a slice path, when plucked, then returns matched elements as a JSON array (why: RFC 9535 slice selector, multi-match results stay chainable like keys/values)",
			tmpl:     `{{ .DATA | json | pluck "items[1:3]" }}`,
			vars:     sv(map[string]string{"DATA": `{"items":["a","b","c","d"]}`}),
			expected: `["b","c"]`,
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

func TestKeysValues(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]value.Value
		expected string
		wantErr  bool
	}{
		{
			name:     "given JSON object, when keys called, then returns sorted JSON array of keys (why: deterministic iteration order)",
			tmpl:     `{{ .DATA | json | keys }}`,
			vars:     sv(map[string]string{"DATA": `{"us":"us-east-1","eu":"eu-west-1"}`}),
			expected: `["eu","us"]`,
		},
		{
			name:     "given JSON object, when values called, then returns values in sorted-key order (why: pairs with keys for consistent zipping)",
			tmpl:     `{{ .DATA | json | values }}`,
			vars:     sv(map[string]string{"DATA": `{"us":"us-east-1","eu":"eu-west-1"}`}),
			expected: `["eu-west-1","us-east-1"]`,
		},
		{
			name:     "given keys result, when piped into loop-style first, then chains like any other list (why: keys/values compose with existing list filters)",
			tmpl:     `{{ .DATA | json | keys | first }}`,
			vars:     sv(map[string]string{"DATA": `{"b":1,"a":2}`}),
			expected: "a",
		},
		{
			name:    "given JSON array (not object), when keys called, then returns error (why: keys only makes sense on objects)",
			tmpl:    `{{ .DATA | json | keys }}`,
			vars:    sv(map[string]string{"DATA": `["a","b"]`}),
			wantErr: true,
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
