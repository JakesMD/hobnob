package tui

import (
	"strings"
	"testing"
)

func TestRunDisplayLines(t *testing.T) {
	tests := []struct {
		name           string
		cmd            string
		task           string
		dir            string
		wantFirstHas   string
		wantFirstLacks string
		wantRestLacks  string
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
		{
			name:          "given dir set, when rendered, then first line shows dir (why: lets user see where the command runs)",
			cmd:           "echo hello",
			task:          "mytask",
			dir:           "./infra",
			wantFirstHas:  "(dir: ./infra)",
			wantRestLacks: "",
		},
		{
			name:           "given dir unset, when rendered, then first line has no dir hint (why: don't clutter output for the common case)",
			cmd:            "echo hello",
			task:           "mytask",
			dir:            "",
			wantFirstHas:   "[mytask]",
			wantFirstLacks: "(dir:",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			// (test fields are the arrangement)

			// Act
			got := RunDisplayLines(test.cmd, test.task, test.dir)

			// Assert
			if len(got) == 0 {
				t.Fatal("got no display lines")
			}
			if !strings.Contains(got[0], test.wantFirstHas) {
				t.Errorf("line 0: want %q in %q", test.wantFirstHas, got[0])
			}
			if test.wantFirstLacks != "" && strings.Contains(got[0], test.wantFirstLacks) {
				t.Errorf("line 0: must not contain %q, got %q", test.wantFirstLacks, got[0])
			}
			if test.wantRestLacks != "" {
				for i, line := range got[1:] {
					if strings.Contains(line, test.wantRestLacks) {
						t.Errorf("line %d: must not contain %q, got %q", i+1, test.wantRestLacks, line)
					}
				}
			}
		})
	}
}

func TestRunSkipLine(t *testing.T) {
	// given task name, when RunSkipLine rendered,
	// then output has run:, task prefix, and skipped marker
	// (why: user must see why a run: step didn't execute)

	// Act
	got := RunSkipLine("mytask")

	// Assert
	if !strings.Contains(got, "run:") {
		t.Errorf("got %q, want it to contain %q", got, "run:")
	}
	if !strings.Contains(got, "[mytask]") {
		t.Errorf("got %q, want it to contain %q", got, "[mytask]")
	}
	if !strings.Contains(got, "skipped") {
		t.Errorf("got %q, want it to contain %q", got, "skipped")
	}
}
