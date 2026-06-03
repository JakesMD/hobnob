package tui

import (
	"strings"
	"testing"
)

func TestRunDisplayLines(t *testing.T) {
	tests := []struct {
		name          string
		cmd           string
		task          string
		wantFirstHas  string
		wantRestLacks string
	}{
		{
			name:          "given single-line cmd, when rendered, then first line has run: and task prefix (why: standard display)",
			cmd:           "echo hello",
			task:          "mytask",
			wantFirstHas:  "[mytask]",
			wantRestLacks: "",
		},
		{
			name:          "given multi-line cmd, when rendered, then only first line has task prefix (why: continuation lines must not repeat prefix)",
			cmd:           "echo first\necho second\necho third",
			task:          "mytask",
			wantFirstHas:  "[mytask]",
			wantRestLacks: "[mytask]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got := RunDisplayLines(tc.cmd, tc.task)

			// Assert
			if len(got) == 0 {
				t.Fatal("got no display lines")
			}
			if !strings.Contains(got[0], tc.wantFirstHas) {
				t.Errorf("line 0: want %q in %q", tc.wantFirstHas, got[0])
			}
			if tc.wantRestLacks != "" {
				for i, line := range got[1:] {
					if strings.Contains(line, tc.wantRestLacks) {
						t.Errorf("line %d: must not contain %q, got %q", i+1, tc.wantRestLacks, line)
					}
				}
			}
		})
	}
}
