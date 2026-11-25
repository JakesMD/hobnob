package tui

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewSelectModel_MultiWithDefault_PreSelectsDefault(t *testing.T) {
	// given multi-select with a default value matching second item,
	// when model is initialized, then that item is pre-selected
	// (why: default must be toggled on at open, not just cursor-positioned)

	// Act
	model := newSelectModel("RELEASES", "", []string{"v0.1.0", "v0.2.0", "v0.3.0"}, true, "v0.2.0")

	// Assert
	if !model.selected[1] {
		t.Errorf("selected[1]: got false, want true (why: default item v0.2.0 at index 1 must be pre-checked)")
	}
	if model.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", model.cursor)
	}
}

func TestNewSelectModel_SingleWithDefault_DoesNotPreSelect(t *testing.T) {
	// given single-select with a default value, when model initialized, then selected map is empty
	// (why: single-select uses cursor position, not selected map)

	// Act
	model := newSelectModel("ENV", "", []string{"staging", "production"}, false, "production")

	// Assert
	if len(model.selected) != 0 {
		t.Errorf("selected: got %v, want empty (why: single-select must not pre-populate selected map)", model.selected)
	}
	if model.cursor != 1 {
		t.Errorf("cursor: got %d, want 1", model.cursor)
	}
}

func TestSelectModel_MultiEnter_NoSelections(t *testing.T) {
	// given multi-select model with no items toggled, when enter pressed, then result is an empty (non-nil) slice (why: empty selection must produce an empty array var, not an error and not nil)
	// Arrange
	model := selectModel{
		varName:  "TAGS",
		items:    []string{"a", "b", "c"},
		multi:    true,
		selected: make(map[int]bool),
	}

	// Act
	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	got := updated.(selectModel).values
	if got == nil || len(got) != 0 {
		t.Errorf("values: got %v, want a non-nil empty slice", got)
	}
}

func pressSel(model selectModel, msg tea.KeyMsg) selectModel {
	next, _ := model.Update(msg)
	return next.(selectModel)
}

func typeInto(model selectModel, text string) selectModel {
	for _, r := range text {
		key := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		if r == ' ' {
			key = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{r}}
		}
		model = pressSel(model, key)
	}
	return model
}

func numberedItems(count int) []string {
	items := make([]string, count)
	for i := range items {
		items[i] = fmt.Sprintf("item-%02d", i+1)
	}
	return items
}

func TestSelectModel_TypingFiltersAndPutsBestMatchFirst(t *testing.T) {
	// given a menu, when letters are typed and enter pressed, then the best fuzzy match is chosen, not the first item in the list
	// (why: ranking by match quality, not list order, is what makes typing worth it)
	// Arrange
	model := newSelectModel("ENV", "", []string{"staging", "pre-ordered-nodes", "production"}, false, "")

	// Act
	model = typeInto(model, "prod")
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if model.value != "production" {
		t.Errorf("value: got %q, want production (why: the contiguous match outranks the scattered one)", model.value)
	}
}

func TestSelectModel_SpaceFirstDoesNotStartAFilter(t *testing.T) {
	// given an empty filter, when space is pressed, then no filter line appears and every item still lists
	// (why: a leading space is not a filter, so a stray space cannot hide the menu)
	// Arrange
	model := newSelectModel("ENV", "", []string{"a", "b"}, false, "")

	// Act
	model = typeInto(model, " ")
	view := model.View()

	// Assert
	if strings.Contains(view, "▏") {
		t.Errorf("got %q, want no filter line", view)
	}
}

func TestSelectModel_SpaceAfterFirstCharacterIsAFilterCharacter(t *testing.T) {
	// given a filter that already has a letter, when space then more letters are typed, then the filter holds the space
	// (why: tab toggles in multi, so the filter is free to contain spaces)
	// Arrange
	model := newSelectModel("ENV", "", []string{"ab", "a b"}, false, "")

	// Act
	model = typeInto(model, "a b")
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if model.value != "a b" {
		t.Errorf("value: got %q, want %q", model.value, "a b")
	}
}

func TestSelectModel_QDoesNotQuit(t *testing.T) {
	// given a menu, when q is typed, then it filters and the menu stays open
	// (why: q is a filter letter now; ctrl+c is the only abort)
	// Arrange
	model := newSelectModel("ENV", "", []string{"queue", "other"}, false, "")

	// Act
	model = typeInto(model, "q")

	// Assert
	if model.quit {
		t.Errorf("quit: got true, want false")
	}
	if !strings.Contains(model.View(), "› q") {
		t.Errorf("got %q, want the filter line to show q", model.View())
	}
}

func TestSelectModel_CtrlCAborts(t *testing.T) {
	// given a menu, when ctrl+c is pressed, then it aborts
	// (why: ctrl+c is the only abort key)
	// Arrange
	model := newSelectModel("ENV", "", []string{"a"}, false, "")

	// Act
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyCtrlC})

	// Assert
	if !model.quit {
		t.Errorf("quit: got false, want true")
	}
}

func TestSelectModel_EscClearsFilterThenSkipsWhenOptional(t *testing.T) {
	// given an optional menu with filter text, when esc is pressed twice, then the first clears the filter and the second skips
	// (why: esc backs out one level at a time, so a skip never throws away typing by surprise)
	// Arrange
	model := newSelectModel("ENV", "", []string{"alpha", "beta"}, false, "")
	model.optional = true
	model = typeInto(model, "al")

	// Act
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEsc})
	afterFirst := model

	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEsc})

	// Assert
	if afterFirst.filter != "" || afterFirst.done {
		t.Errorf("after first esc: filter %q done %v, want cleared and still open", afterFirst.filter, afterFirst.done)
	}
	if !model.done || !model.skipped {
		t.Errorf("after second esc: done %v skipped %v, want both true", model.done, model.skipped)
	}
}

func TestSelectModel_EscDoesNothingWhenRequired(t *testing.T) {
	// given a required menu, when esc is pressed, then it stays open
	// (why: only optional prompts can be skipped)
	// Arrange
	model := newSelectModel("ENV", "", []string{"a"}, false, "")

	// Act
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEsc})

	// Assert
	if model.done {
		t.Errorf("done: got true, want false")
	}
}

func TestSelectModel_MultiTabTogglesAndSelectionSurvivesFiltering(t *testing.T) {
	// given a multi menu, when an item is toggled, the filter changed and another toggled, then enter returns both in list order
	// (why: a pick made under one filter must not vanish when the filter changes)
	// Arrange
	model := newSelectModel("TAGS", "", []string{"alpha", "beta", "gamma"}, true, "")
	model = typeInto(model, "gam")
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyTab})
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEsc})
	model = typeInto(model, "alp")
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyTab})

	// Act
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	want := []string{"alpha", "gamma"}
	if !reflect.DeepEqual(model.values, want) {
		t.Errorf("values: got %v, want %v", model.values, want)
	}
}

func TestSelectModel_FailingCheckStaysInPromptAndShowsError(t *testing.T) {
	// given a menu whose check rejects the choice, when enter is pressed, then the prompt stays open with the validation error shown, and a passing choice then completes it
	// (why: the menu must not close and reopen silently when a check fails)
	// Arrange
	model := newSelectModel("ENV", "", []string{"bad", "good"}, false, "")
	model.checkDesc = `ENV == "good"`
	model.validate = func(v value.Value) (bool, error) { return v.String() == "good", nil }

	// Act
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})
	afterBad := model
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyDown})
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if afterBad.done || !strings.Contains(afterBad.View(), `✗ validation failed: ENV == "good"`) {
		t.Errorf("after bad: done %v view %q, want open with the error line", afterBad.done, afterBad.View())
	}
	if !model.done || model.value != "good" {
		t.Errorf("after good: done %v value %q, want done with good", model.done, model.value)
	}
}

func TestSelectModel_MultiCheckSeesTheWholeList(t *testing.T) {
	// given a multi menu with a validator, when enter is pressed, then the validator gets the list of picks
	// (why: check: with multi: judges the whole list)
	// Arrange
	var seen value.Value
	model := newSelectModel("TAGS", "", []string{"a", "b"}, true, "")
	model.validate = func(v value.Value) (bool, error) { seen = v; return true, nil }
	model = pressSel(model, tea.KeyMsg{Type: tea.KeyTab})

	// Act
	pressSel(model, tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	if !reflect.DeepEqual(seen.Any(), []any{"a"}) {
		t.Errorf("validator saw %v, want [a]", seen.Any())
	}
}

func TestSelectModel_View(t *testing.T) {
	// given menus in various states, when rendered, then the card shows the right body, window and hints
	// (why: the filter line, more-lines and nothing-matches states are what people navigate by)
	long := numberedItems(15)
	scrolled := newSelectModel("PICK", "", long, false, "item-15")
	tests := []struct {
		name     string
		model    selectModel
		wantHas  []string
		wantLack []string
	}{
		{
			name:     "given a short menu with no filter text, when rendered, then there is no filter line and the hints offer filtering",
			model:    newSelectModel("ENV", "Where to?", []string{"staging", "production"}, false, ""),
			wantHas:  []string{"╭─ ", "ENV", "│  Where to?", "▶ staging", "  production", "type to filter • ↑↓ move • enter select"},
			wantLack: []string{"▏", "more", "esc skip", "tab toggle"},
		},
		{
			name:     "given a long menu, when rendered, then ten rows show with a down-more line",
			model:    newSelectModel("PICK", "", long, false, ""),
			wantHas:  []string{"item-10", "↓ 5 more"},
			wantLack: []string{"item-11", "↑ 5"},
		},
		{
			name:     "given the cursor at the end of a long menu, when rendered, then the window follows it with an up-more line",
			model:    scrolled,
			wantHas:  []string{"↑ 5 more", "item-15"},
			wantLack: []string{"item-05", "↓ 5"},
		},
		{
			name:     "given a filter matching nothing, when rendered, then nothing matches shows under the filter line",
			model:    typeInto(newSelectModel("PICK", "", long, false, ""), "zzz"),
			wantHas:  []string{"› zzz▏", "───", "nothing matches"},
			wantLack: []string{"item-01"},
		},
		{
			name:    "given a multi menu, when rendered, then the hints name tab and enter confirm",
			model:   newSelectModel("TAGS", "", []string{"a", "b"}, true, ""),
			wantHas: []string{"type to filter • ↑↓ move • tab toggle • enter confirm", "○ a"},
		},
		{
			name: "given an optional menu, when rendered, then the title and hints say it can be skipped",
			model: func() selectModel {
				m := newSelectModel("ENV", "", []string{"a"}, false, "")
				m.optional = true
				return m
			}(),
			wantHas: []string{"(optional)", "esc skip"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			model := test.model

			// Act
			got := model.View()

			// Assert
			for _, want := range test.wantHas {
				if !strings.Contains(got, want) {
					t.Errorf("got %q, want it to contain %q", got, want)
				}
			}
			for _, lack := range test.wantLack {
				if strings.Contains(got, lack) {
					t.Errorf("got %q, want it to NOT contain %q", got, lack)
				}
			}
		})
	}
}
