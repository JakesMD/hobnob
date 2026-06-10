package eval

import (
	"testing"
)

func TestEvalTemplate(t *testing.T) {
	tests := []struct {
		name        string
		tmpl        string
		vars        map[string]string
		expected    string
		expectError bool
	}{
		{
			name:     "given static value, when evaluated, then returns value unchanged (why: plain strings pass through)",
			tmpl:     "1048576",
			vars:     map[string]string{},
			expected: "1048576",
		},
		{
			name:     "given var reference, when var present, then substitutes value (why: set-vars derive from prior vars)",
			tmpl:     "{{.BASE}}",
			vars:     map[string]string{"BASE": "hello"},
			expected: "hello",
		},
		{
			name:     "given default func with empty var, when evaluated, then returns default (why: global vars use | default for fallback)",
			tmpl:     `{{.MISSING | default "fallback"}}`,
			vars:     map[string]string{},
			expected: "fallback",
		},
		{
			name:     "given default func with set var, when evaluated, then returns var value (why: env value overrides default)",
			tmpl:     `{{.TIMEOUT | default "30"}}`,
			vars:     map[string]string{"TIMEOUT": "60"},
			expected: "60",
		},
		{
			name:        "given invalid template syntax, when evaluated, then returns error (why: bad templates surface early)",
			tmpl:        "{{ .Unclosed",
			vars:        map[string]string{},
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error but got nil (result: %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestTrim(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "given string with leading/trailing whitespace, when trim called, then whitespace removed (why: shell output often has padding)",
			tmpl:     `{{ "  hello  " | trim }}`,
			vars:     map[string]string{},
			expected: "hello",
		},
		{
			name:     "given string with no whitespace, when trim called, then unchanged (why: trim must be a no-op on clean strings)",
			tmpl:     `{{ "hello" | trim }}`,
			vars:     map[string]string{},
			expected: "hello",
		},
		{
			name:     "given var with trailing newline, when trim called, then newline removed (why: piping run output without splitLines)",
			tmpl:     `{{ .VAL | trim }}`,
			vars:     map[string]string{"VAL": "value\n"},
			expected: "value",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "given comma-separated string, when split on comma, then returns JSON array (why: generic delimiter support)",
			tmpl:     `{{ split "," "a,b,c" }}`,
			vars:     map[string]string{},
			expected: `["a","b","c"]`,
		},
		{
			name:     "given newline-separated string, when split on newline, then returns JSON array (why: equivalent to splitLines without trim)",
			tmpl:     "{{ split \"\\n\" \"x\\ny\\nz\" }}",
			vars:     map[string]string{},
			expected: `["x","y","z"]`,
		},
		{
			name:     "given string with empty tokens, when split called, then empty tokens filtered (why: avoid empty ITEM in for loop)",
			tmpl:     `{{ split "," "a,,b" }}`,
			vars:     map[string]string{},
			expected: `["a","b"]`,
		},
		{
			name:     "given var with delimited string, when split called via template, then returns JSON array (why: real usage pattern)",
			tmpl:     `{{ .PKGS | split "," }}`,
			vars:     map[string]string{"PKGS": "foo,bar,baz"},
			expected: `["foo","bar","baz"]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestLines(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "given multiline string, when lines called, then returns JSON array (why: for loop expects JSON array from ParseList)",
			tmpl:     `{{ "line1\nline2\nline3" | lines }}`,
			vars:     map[string]string{},
			expected: `["line1","line2","line3"]`,
		},
		{
			name:     "given string with blank lines, when lines called, then blank lines filtered (why: empty entries would produce empty ITEM in for loop)",
			tmpl:     `{{ "a\n\nb\n\nc" | lines }}`,
			vars:     map[string]string{},
			expected: `["a","b","c"]`,
		},
		{
			name:     "given string with trailing newline, when lines called, then trailing newline ignored (why: shell output always ends with newline)",
			tmpl:     "{{ \"hello\\nworld\\n\" | lines }}",
			vars:     map[string]string{},
			expected: `["hello","world"]`,
		},
		{
			name:     "given single line string, when lines called, then returns single-element JSON array (why: consistent type for for loop)",
			tmpl:     `{{ "only" | lines }}`,
			vars:     map[string]string{},
			expected: `["only"]`,
		},
		{
			name:     "given string with leading/trailing spaces on lines, when lines called, then lines are trimmed (why: shell output may have whitespace padding)",
			tmpl:     `{{ "  foo  \n  bar  " | lines }}`,
			vars:     map[string]string{},
			expected: `["foo","bar"]`,
		},
		{
			name:     "given var holding multiline string, when lines called via template, then returns JSON array (why: real usage pattern with run output stored in var)",
			tmpl:     `{{ .PKGS | lines }}`,
			vars:     map[string]string{"PKGS": "pkg_a\npkg_b\npkg_c"},
			expected: `["pkg_a","pkg_b","pkg_c"]`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestUpper(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "given lowercase string, when upper called, then returns uppercase (why: case normalisation for comparisons)",
			tmpl:     `{{ "hello world" | upper }}`,
			vars:     map[string]string{},
			expected: "HELLO WORLD",
		},
		{
			name:     "given mixed-case var, when upper called via template, then returns uppercase (why: real usage pattern)",
			tmpl:     `{{ .VAL | upper }}`,
			vars:     map[string]string{"VAL": "Hello"},
			expected: "HELLO",
		},
		{
			name:     "given already-uppercase string, when upper called, then unchanged (why: upper is idempotent)",
			tmpl:     `{{ "DONE" | upper }}`,
			vars:     map[string]string{},
			expected: "DONE",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestLower(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
	}{
		{
			name:     "given uppercase string, when lower called, then returns lowercase (why: case normalisation for comparisons)",
			tmpl:     `{{ "HELLO WORLD" | lower }}`,
			vars:     map[string]string{},
			expected: "hello world",
		},
		{
			name:     "given mixed-case var, when lower called via template, then returns lowercase (why: real usage pattern)",
			tmpl:     `{{ .VAL | lower }}`,
			vars:     map[string]string{"VAL": "Hello"},
			expected: "hello",
		},
		{
			name:     "given already-lowercase string, when lower called, then unchanged (why: lower is idempotent)",
			tmpl:     `{{ "done" | lower }}`,
			vars:     map[string]string{},
			expected: "done",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestFirst(t *testing.T) {
	tests := []struct {
		name     string
		tmpl     string
		vars     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "given JSON array var, when first called via pipe, then returns first element (why: extracts first item from lines/split output)",
			tmpl:     `{{ .LIST | first }}`,
			vars:     map[string]string{"LIST": `["v0.3.0","v0.2.1","v0.1.0"]`},
			expected: "v0.3.0",
		},
		{
			name:     "given inline JSON array, when first called, then returns first element (why: consistent with var usage)",
			tmpl:     `{{ first "[\"a\",\"b\",\"c\"]" }}`,
			vars:     map[string]string{},
			expected: "a",
		},
		{
			name:     "given empty JSON array, when first called, then returns empty string (why: no panic on empty list)",
			tmpl:     `{{ .LIST | first }}`,
			vars:     map[string]string{"LIST": `[]`},
			expected: "",
		},
		{
			name:    "given invalid JSON, when first called, then returns error (why: fail fast on bad pipe data)",
			tmpl:    `{{ first "not-json" }}`,
			vars:    map[string]string{},
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalTemplate(tc.tmpl, tc.vars)

			// Assert
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (result: %q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestEvalRunIntoPipe(t *testing.T) {
	tests := []struct {
		name        string
		expr        string
		stdout      string
		stderr      string
		expected    string
		expectError bool
	}{
		{
			name:     "given stdout source, when no pipe, then returns raw stdout (why: basic capture)",
			expr:     "stdout",
			stdout:   "hello\n",
			stderr:   "",
			expected: "hello\n",
		},
		{
			name:     "given stderr source, when no pipe, then returns raw stderr (why: basic error capture)",
			expr:     "stderr",
			stdout:   "",
			stderr:   "error msg\n",
			expected: "error msg\n",
		},
		{
			name:     "given stdout | trim, when stdout has trailing newline, then trims (why: common pattern for single-value capture)",
			expr:     "stdout | trim",
			stdout:   "hello\n",
			stderr:   "",
			expected: "hello",
		},
		{
			name:     "given stdout | lines, when stdout is multiline, then returns JSON array (why: capture list output into loop-ready var)",
			expr:     "stdout | lines",
			stdout:   "alpha\nbeta\ngamma\n",
			stderr:   "",
			expected: `["alpha","beta","gamma"]`,
		},
		{
			name:     "given stdout | upper, when stdout is lowercase, then returns uppercase (why: normalise captured output)",
			expr:     "stdout | upper",
			stdout:   "hello",
			stderr:   "",
			expected: "HELLO",
		},
		{
			name:     "given stdout | lower, when stdout is uppercase, then returns lowercase (why: normalise captured output)",
			expr:     "stdout | lower",
			stdout:   "HELLO",
			stderr:   "",
			expected: "hello",
		},
		{
			name:     "given stdout | trim | upper, when chained, then applies both transforms (why: pipe chain works left to right)",
			expr:     "stdout | trim | upper",
			stdout:   "  hello  ",
			stderr:   "",
			expected: "HELLO",
		},
		{
			name:        "given unknown source, when evaluated, then returns error (why: typo guard)",
			expr:        "stdin",
			expectError: true,
		},
		{
			name:        "given unknown pipe func, when evaluated, then returns error (why: typo guard)",
			expr:        "stdout | reverse",
			expectError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalRunIntoPipe(tc.expr, tc.stdout, tc.stderr)

			// Assert
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%q)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("got %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestEvalCondition(t *testing.T) {
	tests := []struct {
		name      string
		condTmpl  string
		vars      map[string]string
		wantTrue  bool
		wantError bool
	}{
		{
			name:     "given string equality match, when evaluated, then returns true (why: if condition gates step execution)",
			condTmpl: `[ "{{.METHOD}}" = "Chunked Upload" ]`,
			vars:     map[string]string{"METHOD": "Chunked Upload"},
			wantTrue: true,
		},
		{
			name:     "given string equality mismatch, when evaluated, then returns false (why: step should be skipped)",
			condTmpl: `[ "{{.METHOD}}" = "Chunked Upload" ]`,
			vars:     map[string]string{"METHOD": "Direct Upload"},
			wantTrue: false,
		},
		{
			name:     "given string inequality match, when evaluated, then returns true (why: != skips excluded value)",
			condTmpl: `[ "{{.MOTOR}}" != "Z-Axis Lead" ]`,
			vars:     map[string]string{"MOTOR": "X-Axis Stepper"},
			wantTrue: true,
		},
		{
			name:     "given string inequality match on excluded value, when evaluated, then returns false (why: Z-Axis Lead is excluded)",
			condTmpl: `[ "{{.MOTOR}}" != "Z-Axis Lead" ]`,
			vars:     map[string]string{"MOTOR": "Z-Axis Lead"},
			wantTrue: false,
		},
		{
			name:     "given numeric less-than-equal pass, when evaluated, then returns true (why: check validates numeric bounds)",
			condTmpl: `[ {{.SPEED}} -le {{.LIMIT}} ]`,
			vars:     map[string]string{"SPEED": "1500", "LIMIT": "3000"},
			wantTrue: true,
		},
		{
			name:     "given numeric less-than-equal fail, when evaluated, then returns false (why: value exceeds limit)",
			condTmpl: `[ {{.SPEED}} -le {{.LIMIT}} ]`,
			vars:     map[string]string{"SPEED": "9999", "LIMIT": "3000"},
			wantTrue: false,
		},
		{
			name:     "given numeric less-than pass, when evaluated, then returns true (why: chunk size within limit)",
			condTmpl: `[ {{.SIZE}} -lt {{.MAX}} ]`,
			vars:     map[string]string{"SIZE": "1024", "MAX": "10485760"},
			wantTrue: true,
		},
		{
			name:     "given numeric less-than fail, when evaluated, then returns false (why: oversized chunk must be rejected)",
			condTmpl: `[ {{.SIZE}} -lt {{.MAX}} ]`,
			vars:     map[string]string{"SIZE": "99999999", "MAX": "10485760"},
			wantTrue: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got, err := EvalCondition(tc.condTmpl, tc.vars)

			// Assert
			if tc.wantError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantTrue {
				t.Errorf("got %v, want %v", got, tc.wantTrue)
			}
		})
	}
}

func TestParseList(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    []string
		expectError bool
	}{
		{
			name:     "given JSON array, when parsed, then returns items (why: list vars stored as JSON)",
			input:    `["pin_a1","pin_a2","pin_b1"]`,
			expected: []string{"pin_a1", "pin_a2", "pin_b1"},
		},
		{
			name:     "given JSON array with spaces in values, when parsed, then preserves spaces (why: list items may contain spaces)",
			input:    `["X-Axis Stepper","Y-Axis Stepper","Z-Axis Lead"]`,
			expected: []string{"X-Axis Stepper", "Y-Axis Stepper", "Z-Axis Lead"},
		},
		{
			name:     "given empty string, when parsed, then returns nil (why: unset list var is valid)",
			input:    "",
			expected: nil,
		},
		{
			name:     "given single value (non-JSON), when parsed, then returns single-item slice (why: single template value used as list)",
			input:    "only_value",
			expected: []string{"only_value"},
		},
		{
			name:        "given invalid JSON array, when parsed, then returns error (why: malformed list must surface early)",
			input:       `["unclosed`,
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc.input is the arrangement)

			// Act
			got, err := ParseList(tc.input)

			// Assert
			if tc.expectError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tc.expected) {
				t.Fatalf("len: got %d, want %d (got=%v want=%v)", len(got), len(tc.expected), got, tc.expected)
			}
			for i, w := range tc.expected {
				if got[i] != w {
					t.Errorf("[%d]: got %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

func TestCopyVars(t *testing.T) {
	// given source map, when CopyVars called and dst mutated, then source unchanged (why: scope isolation depends on deep copy semantics)
	// Arrange
	src := map[string]string{"A": "1", "B": "2"}

	// Act
	dst := CopyVars(src)
	dst["A"] = "changed"

	// Assert
	if src["A"] != "1" {
		t.Errorf("CopyVars mutated source: src[A]=%q", src["A"])
	}
	if dst["B"] != "2" {
		t.Errorf("CopyVars missing key: dst[B]=%q", dst["B"])
	}
}
