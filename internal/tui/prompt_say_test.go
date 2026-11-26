package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSayModel_ShowsTheMessageAndEnterContinues(t *testing.T) {
	// given a say: card, when rendered and Enter is pressed, then it shows
	// the task and the message with an enter hint, and Enter ends it without
	// aborting (why: a message only waits for the person to go on)
	// Arrange
	model := newSayModel(card{task: "deploy", body: "All done."})

	// Act
	view := model.View()
	next, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Assert
	for _, want := range []string{"╭─ deploy ─", "│  All done.", "enter continue"} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "»") {
		t.Errorf("view names a variable:\n%s", view)
	}
	if got := next.(sayModel); !got.done || got.quit || cmd == nil {
		t.Errorf("after enter: done=%v quit=%v cmd=%v", got.done, got.quit, cmd != nil)
	}
}
