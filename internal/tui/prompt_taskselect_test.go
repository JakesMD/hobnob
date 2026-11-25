package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

var sampleTasks = []TaskItem{
	{Name: "build", Info: "Build the project"},
	{Name: "deploy", Info: "Deploy to prod"},
	{Name: "test", Info: "Run tests"},
}

func pressTask(model taskSelectModel, msg tea.KeyMsg) taskSelectModel {
	next, _ := model.Update(msg)
	return next.(taskSelectModel)
}

func typeIntoTask(model taskSelectModel, text string) taskSelectModel {
	for _, r := range text {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{r}}
		}
		model = pressTask(model, key)
	}
	return model
}

func TestTaskSelectModel_EnterPicksHighlightedTask(t *testing.T) {
	// given the cursor moved down once, when enter is pressed, then that task is the value
	// (why: enter must run the focused task)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyDown})
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if model.value != "deploy" {
		t.Errorf("value: got %q, want deploy", model.value)
	}
}

func TestTaskSelectModel_CursorStaysInsideTheList(t *testing.T) {
	// given a three-task list, when up is pressed at the top and down five times, then the cursor stops at both ends
	// (why: the cursor must never leave the list)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyUp})
	atTop := model.cursor
	for range 5 {
		model = pressTask(model, tea.KeyMsg{Type: tea.KeyDown})
	}

	// Assert
	if atTop != 0 || model.cursor != 2 {
		t.Errorf("cursor: got %d then %d, want 0 then 2", atTop, model.cursor)
	}
}

func TestTaskSelectModel_TypingFiltersAndPicksBestMatch(t *testing.T) {
	// given several tasks, when letters are typed and enter pressed, then the best fuzzy match is picked
	// (why: filtering is how a long task list is navigated)
	// Arrange
	model := taskSelectModel{tasks: []TaskItem{{Name: "lint-docs"}, {Name: "deploy"}, {Name: "docs"}}}

	// Act
	model = typeIntoTask(model, "docs")
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if model.value != "docs" {
		t.Errorf("value: got %q, want docs (why: the contiguous prefix match outranks the later one)", model.value)
	}
}

func TestTaskSelectModel_EnterWithNothingMatchingStaysOpen(t *testing.T) {
	// given a filter that matches nothing, when enter is pressed, then nothing is picked and the view says so
	// (why: there is no task to run)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	model = typeIntoTask(model, "zzz")
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if model.value != "" {
		t.Errorf("value: got %q, want empty", model.value)
	}
	if !strings.Contains(model.View(), "nothing matches") {
		t.Errorf("got %q, want nothing matches", model.View())
	}
}

func TestTaskSelectModel_QFiltersInsteadOfQuitting(t *testing.T) {
	// given the picker, when q is typed, then it filters and does not abort
	// (why: ctrl+c is the only abort key)
	// Arrange
	model := taskSelectModel{tasks: []TaskItem{{Name: "quick"}, {Name: "build"}}}

	// Act
	model = typeIntoTask(model, "q")

	// Assert
	if model.quit {
		t.Error("quit: got true, want false")
	}
	if model.value != "" || !strings.Contains(model.View(), "› q") {
		t.Errorf("got %q, want the filter line showing q", model.View())
	}
}

func TestTaskSelectModel_CtrlCAborts(t *testing.T) {
	// given the picker, when ctrl+c is pressed, then it aborts without a value
	// (why: ctrl+c is the only abort key)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyCtrlC})

	// Assert
	if !model.quit || model.value != "" {
		t.Errorf("quit %v value %q, want true and empty", model.quit, model.value)
	}
}

func TestTaskSelectModel_EscClearsTheFilter(t *testing.T) {
	// given filter text, when esc is pressed, then the filter clears and the picker stays open
	// (why: esc first backs out of the filter; a required picker has nothing to skip)
	// Arrange
	model := typeIntoTask(taskSelectModel{tasks: sampleTasks}, "dep")

	// Act
	model = pressTask(model, tea.KeyMsg{Type: tea.KeyEsc})

	// Assert
	if model.filter != "" || model.quit || model.value != "" {
		t.Errorf("filter %q quit %v value %q, want cleared and open", model.filter, model.quit, model.value)
	}
}

func TestTaskSelectModel_SpaceFirstDoesNotStartAFilter(t *testing.T) {
	// given an empty filter, when space is pressed, then no filter line appears
	// (why: a stray leading space must not hide the list)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	model = typeIntoTask(model, " ")

	// Assert
	if strings.Contains(model.View(), "▏") {
		t.Errorf("got %q, want no filter line", model.View())
	}
}

func TestTaskSelectModel_ViewIsACardWithItsOwnTitleAndHints(t *testing.T) {
	// given the picker, when viewed, then it shows the card titled "Select a task to run." with names, infos and the run hints
	// (why: the picker looks like every other prompt, with no var name in the title)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks}

	// Act
	view := model.View()

	// Assert
	for _, want := range []string{"╭─ ", "Select a task to run.", "build", "Build the project", "deploy", "type to filter", "↑↓", "move", "enter", "run"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "»") || strings.Contains(view, "skip") {
		t.Errorf("view has a var name mark or skip hint:\n%s", view)
	}
}

func TestTaskSelectModel_ViewEmptyAfterSelection(t *testing.T) {
	// given a picked task, when viewed, then the card is cleared
	// (why: the TUI must leave nothing behind, matching the other prompts)
	// Arrange
	model := taskSelectModel{tasks: sampleTasks, value: "build"}

	// Act
	view := model.View()

	// Assert
	if view != "" {
		t.Errorf("view: got %q, want empty", view)
	}
}
