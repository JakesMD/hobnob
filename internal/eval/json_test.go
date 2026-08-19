package eval

import "testing"

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

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test.input is the arrangement)

			// Act
			got, err := ParseList(test.input)

			// Assert
			if test.expectError {
				if err == nil {
					t.Errorf("expected error, got nil (result=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(test.expected) {
				t.Fatalf("len: got %d, want %d (got=%v want=%v)", len(got), len(test.expected), got, test.expected)
			}
			for i, wantItem := range test.expected {
				if got[i] != wantItem {
					t.Errorf("[%d]: got %q, want %q", i, got[i], wantItem)
				}
			}
		})
	}
}
