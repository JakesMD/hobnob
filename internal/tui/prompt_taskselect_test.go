package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTaskSelectModel_EnterSelectsCurrentTask(t *testing.T) {
	// given task select model with cursor on second item, when enter pressed, then returns that task name
	// (why: enter must select the focused task)

	// Arrange
	model := taskSelectModel{
		tasks: []TaskItem{
			{Name: "build", Info: "Build the project"},
			{Name: "deploy", Info: "Deploy to prod"},
			{Name: "test", Info: "Run tests"},
		},
		cursor: 1,
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

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
	model := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		cursor: 0,
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyUp})

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
	model := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}, {Name: "c"}},
		cursor: 0,
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})

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
	model := taskSelectModel{
		tasks:  []TaskItem{{Name: "a"}, {Name: "b"}},
		cursor: 1,
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyDown})

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
	model := taskSelectModel{
		tasks: []TaskItem{{Name: "build"}},
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})

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
	model := taskSelectModel{
		tasks: []TaskItem{
			{Name: "build", Info: "Build the project"},
			{Name: "deploy", Info: ""},
		},
	}

	// Act
	view := model.View()

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
	model := taskSelectModel{
		tasks: []TaskItem{{Name: "build"}},
		value: "build",
	}

	// Act
	view := model.View()

	// Assert
	if view != "" {
		t.Errorf("view: got %q, want empty", view)
	}
}
