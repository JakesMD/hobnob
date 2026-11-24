package eval

import (
	"reflect"
	"testing"
)

func TestReferencedVars(t *testing.T) {
	// given template expressions, when scanned, then each referenced var is
	// named once (why: load-time rules such as upward reads depend on knowing
	// every reference)
	// Arrange
	tests := []struct {
		name string
		expr string
		want []string
	}{
		{"explicit action", "{{ .HOST }}", []string{"HOST"}},
		{"plain literal, no refs", "https://api.example.com", nil},
		{"accessor chain", "{{ .RESP.profile.name }}", []string{"RESP"}},
		{"dynamic key", "{{ .JSON[.KEY] }}", []string{"JSON", "KEY"}},
		{"filter chain", `{{ .HOST | default "x" }}`, []string{"HOST"}},
		{"multiple actions", "{{.A}}-{{.B}}", []string{"A", "B"}},
		{"self reference", "{{ .HOST | default \"x\" }}", []string{"HOST"}},
		{"no refs", "plain text", nil},
		{"dedupes", "{{.A}}{{.A}}", []string{"A"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got, err := ReferencedVars(tc.expr)

			// Assert
			if err != nil {
				t.Fatalf("ReferencedVars(%q): %v", tc.expr, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ReferencedVars(%q) = %v, want %v", tc.expr, got, tc.want)
			}
		})
	}
}
