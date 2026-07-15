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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			m := textModel{varName: "NOTES", optional: tt.optional}

			// Act
			got := m.View()

			// Assert
			if tt.wantHas != "" && !strings.Contains(got, tt.wantHas) {
				t.Errorf("got %q, want it to contain %q", got, tt.wantHas)
			}
			if tt.wantLacks != "" && strings.Contains(got, tt.wantLacks) {
				t.Errorf("got %q, want it to NOT contain %q", got, tt.wantLacks)
			}
		})
	}
}

func TestNewSelectModel_MultiWithDefault_PreSelectsDefault(t *testing.T) {
	// given multi-select with a default value matching second item,
	// when model is initialized, then that item is pre-selected
	// (why: default must be toggled on at open, not just cursor-positioned)

	// Act
	m := newSelectModel("RELEASES", "", []string{"v0.1.0", "v0.2.0", "v0.3.0"}, true, "v0.2.0")

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
	m := newSelectModel("ENV", "", []string{"staging", "production"}, false, "production")

	// Assert
	if len(m.selected) != 0 {
		t.Errorf("selected: got %v, want empty (why: single-select must not pre-populate selected map)", m.selected)
	}
	if m.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", m.cursor)
	}
}

func TestSelectModel_MultiEnter_NoSelections(t *testing.T) {
	// given multi-select model with no items toggled, when enter pressed, then result is empty string (why: empty selection must produce empty var not an error)
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

func TestTaskSelectModel_EnterSelectsCurrentTask(t *testing.T) {
	// given task select model with cursor on second item, when enter pressed, then returns that task name
	// (why: enter must select the focused task)

	// Arrange
	m := taskSelectModel{
		tasks: []TaskItem{
			{Name: "build", Info: "Build the project"},
			{Name: "deploy", Info: "Deploy to prod"},
			{Name: "test", Info: "Run tests"},
		},
		cursor: 1,
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	got := updated.(taskSelectModel).value
	if got != "deploy" {
		t.Errorf("value: got %q, want %q", got, "deploy")
	}
}

func TestTaskSelectModel_NavigationWraps(t *testing.T) {
	// given task select model at first item, when up pressed, then cursor stays at 0
	// (why: cursor must not go negative)

	// Arrange
	m := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		cursor: 0,
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})

	// Assert
	got := updated.(taskSelectModel).cursor
	if got != 0 {
		t.Errorf("cursor: got %d, want 0", got)
	}
}

func TestTaskSelectModel_DownMovesForward(t *testing.T) {
	// given task select model at first item, when down pressed, then cursor advances
	// (why: down key must navigate to next task)

	// Arrange
	m := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		cursor: 0,
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Assert
	got := updated.(taskSelectModel).cursor
	if got != 1 {
		t.Errorf("cursor: got %d, want 1", got)
	}
}

func TestTaskSelectModel_DownStopsAtEnd(t *testing.T) {
	// given task select model at last item, when down pressed, then cursor stays
	// (why: cursor must not exceed item count)

	// Arrange
	m := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}},
		cursor: 1,
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})

	// Assert
	got := updated.(taskSelectModel).cursor
	if got != 1 {
		t.Errorf("cursor: got %d, want 1", got)
	}
}

func TestTaskSelectModel_QuitSetsQuitFlag(t *testing.T) {
	// given task select model, when q pressed, then quit is true
	// (why: q must abort selection without returning a value)

	// Arrange
	m := taskSelectModel{
		tasks: []TaskItem{{Name: "build"}},
	}

	// Act
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

	// Assert
	final := updated.(taskSelectModel)
	if !final.quit {
		t.Error("quit: got false, want true")
	}
	if final.value != "" {
		t.Errorf("value: got %q, want empty", final.value)
	}
}

func TestTaskSelectModel_ViewShowsTaskNamesAndInfo(t *testing.T) {
	// given task select model with info, when View called, then output contains task names and descriptions
	// (why: selector must display enough context to choose)

	// Arrange
	m := taskSelectModel{
		tasks: []TaskItem{
			{Name: "build", Info: "Build the project"},
			{Name: "deploy", Info: ""},
		},
	}

	// Act
	view := m.View()

	// Assert
	if !strings.Contains(view, "build") {
		t.Error("view missing task name 'build'")
	}
	if !strings.Contains(view, "Build the project") {
		t.Error("view missing task info")
	}
	if !strings.Contains(view, "deploy") {
		t.Error("view missing task name 'deploy'")
	}
}

func TestTaskSelectModel_ViewEmptyAfterSelection(t *testing.T) {
	// given task select model with a value set, when View called, then returns empty string
	// (why: TUI must clear after selection, matching selectModel behavior)

	// Arrange
	m := taskSelectModel{
		tasks: []TaskItem{{Name: "build"}},
		value: "build",
	}

	// Act
	view := m.View()

	// Assert
	if view != "" {
		t.Errorf("view: got %q, want empty", view)
	}
}
