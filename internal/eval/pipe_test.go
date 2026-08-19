package eval

import "testing"

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
			name:     "given stdout | pluck, when stdout is JSON, then extracts the field (why: into: pipes share the same filter registry as {{ }} templates, not a separate hand-rolled list)",
			expr:     `stdout | pluck "name"`,
			stdout:   `{"name":"hobnob"}`,
			stderr:   "",
			expected: "hobnob",
		},
		{
			name:     "given stdout | lines | first, when stdout is multiline, then returns the first line (why: filters chain the same way they do in {{ }} templates)",
			expr:     "stdout | lines | first",
			stdout:   "alpha\nbeta\n",
			stderr:   "",
			expected: "alpha",
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got, err := EvalRunIntoPipe(test.expr, test.stdout, test.stderr)

			// Assert
			if test.expectError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%q)", got)
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
