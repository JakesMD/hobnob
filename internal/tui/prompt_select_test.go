package tui

import (
	"testing"

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
	// given multi-select model with no items toggled, when enter pressed, then result is empty string (why: empty selection must produce empty var not an error)
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
	got := updated.(selectModel).value
	if got != "[]" {
		t.Errorf("value: got %q, want %q (why: empty multi-select must marshal to an empty JSON array, not null)", got, "[]")
	}
}
