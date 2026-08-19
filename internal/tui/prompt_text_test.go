package tui

import (
	"strings"
	"testing"
)

func TestTextModel_View_OptionalHint(t *testing.T) {
	tests := []struct {
		name      string
		optional  bool
		wantHas   string
		wantLacks string
	}{
		{
			name:     "given optional text prompt, when rendered, then header shows leave-blank hint (why: optional fields must be obviously skippable)",
			optional: true,
			wantHas:  "optional, leave blank to skip",
		},
		{
			name:      "given required text prompt, when rendered, then no optional hint shown",
			optional:  false,
			wantLacks: "optional",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			model := textModel{varName: "NOTES", optional: test.optional}

			// Act
			got := model.View()

			// Assert
			if test.wantHas != "" && !strings.Contains(got, test.wantHas) {
				t.Errorf("got %q, want it to contain %q", got, test.wantHas)
			}
			if test.wantLacks != "" && strings.Contains(got, test.wantLacks) {
				t.Errorf("got %q, want it to NOT contain %q", got, test.wantLacks)
			}
		})
	}
}
