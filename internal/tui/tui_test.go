package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			// (tc fields are the arrangement)

			// Act
			got := RunDisplayLines(tc.cmd, tc.task, tc.dir)

			// Assert
			if len(got) == 0 {
				t.Fatal("got no display lines")
			}
			if !strings.Contains(got[0], tc.wantFirstHas) {
				t.Errorf("line 0: want %q in %q", tc.wantFirstHas, got[0])
			}
			if tc.wantFirstLacks != "" && strings.Contains(got[0], tc.wantFirstLacks) {
				t.Errorf("line 0: must not contain %q, got %q", tc.wantFirstLacks, got[0])
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

func TestNewSelectModel_MultiWithDefault_PreSelectsDefault(t *testing.T) {
	// given multi-select with a default value matching second item,
	// when model is initialized, then that item is pre-selected
	// (why: default must be toggled on at open, not just cursor-positioned)

	// Act
	m := newSelectModel("RELEASES", "", []string{"v0.1.0", "v0.2.0", "v0.3.0"}, true, "v0.2.0", "", false)

	// Assert
	if !m.selected[1] {
		t.Errorf("selected[1]: got false, want true (why: default item v0.2.0 at index 1 must be pre-checked)")
	}
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", m.cursor)
	}
}

func TestNewSelectModel_SingleWithDefault_DoesNotPreSelect(t *testing.T) {
	// given single-select with a default value, when model initialized, then selected map is empty
	// (why: single-select uses cursor position, not selected map)

	// Act
	m := newSelectModel("ENV", "", []string{"staging", "production"}, false, "production", "", false)

	// Assert
	if len(m.selected) != 0 {
		t.Errorf("selected: got %v, want empty (why: single-select must not pre-populate selected map)", m.selected)
	}
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", m.cursor)
	}
}

func TestSelectModel_MultiEnter_NoSelections(t *testing.T) {
	// Arrange
	m := selectModel{
		varName:  "TAGS",
		items:    []string{"a", "b", "c"},
		multi:    true,
		selected: make(map[int]bool),
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	got := updated.(selectModel).value
	if got != "[]" {
		t.Errorf("value: got %q, want %q (why: empty multi-select must marshal to an empty JSON array, not null)", got, "[]")
	}
}
