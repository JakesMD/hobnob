package tui

import (
	"strings"
	"testing"

	"hobnob/internal/ask"
	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

func keyPress(model tea.Model, msg tea.KeyMsg) textModel {
	next, _ := model.Update(msg)
	return next.(textModel)
}

func typeText(model textModel, text string) textModel {
	return keyPress(model, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(text)})
}

func TestTextModel_View_Card(t *testing.T) {
	// given a text prompt, when rendered, then it is a card with the title in
	// the top border, the info inside, the body and a hint line under the box
	// (why: info must be impossible to overlook)
	tests := []struct {
		name     string
		model    textModel
		wantHas  []string
		wantLack []string
	}{
		{
			name:  "given a required prompt with info, when rendered, then title, info and enter hint show",
			model: newTextModel(ask.Question{Task: "deploy", VarName: "REPLICAS", Info: "How many replicas?"}),
			wantHas: []string{
				"╭─ deploy » REPLICAS ─", "│  How many replicas?", "› ", "╰─", "\n enter confirm",
			},
			wantLack: []string{"optional", "esc skip"},
		},
		{
			name:    "given an optional prompt, when rendered, then the title and hints say it can be skipped",
			model:   newTextModel(ask.Question{Task: "deploy", VarName: "NOTES", Optional: true}),
			wantHas: []string{"deploy » NOTES (optional) ─", "enter confirm • esc skip"},
		},
		{
			name:    "given a default, when rendered, then the field is prefilled with it",
			model:   newTextModel(ask.Question{Task: "deploy", VarName: "ENV", Default: "staging"}),
			wantHas: []string{"› staging"},
		},
		{
			name:     "given a secret default, when rendered, then the field shows bullets, never the value",
			model:    newTextModel(ask.Question{Task: "deploy", VarName: "TOKEN", Default: "hunter2", Secret: true}),
			wantHas:  []string{"› ••••"},
			wantLack: []string{"hunter2", "•••••"},
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

func TestTextModel_View_WidthFollowsTerminalCappedAt76(t *testing.T) {
	// given terminals of different widths, when the card renders, then its
	// top border is as wide as the terminal allows up to the cap (why: wide
	// terminals must not stretch the card across the screen)
	tests := []struct {
		name      string
		termWidth int
		wantWidth int
	}{
		{name: "given a wide terminal, when rendered, then the card is 76 wide", termWidth: 200, wantWidth: 76},
		{name: "given a narrow terminal, when rendered, then the card fits it", termWidth: 40, wantWidth: 40},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			model := newTextModel(ask.Question{Task: "t", VarName: "X", Info: "An info line that is long enough to wrap in a narrow terminal window."})
			next, _ := model.Update(tea.WindowSizeMsg{Width: test.termWidth})

			// Act
			lines := strings.Split(next.View(), "\n")[1:]

			// Assert
			if got := len([]rune(lines[0])); got != test.wantWidth {
				t.Errorf("top border is %d wide, want %d: %q", got, test.wantWidth, lines[0])
			}
		})
	}
}

func TestTextModel_Update_Answers(t *testing.T) {
	// given a text prompt, when keys are pressed, then it hands the text to
	// the question's judge and ends or shows the refusal (why: the prompt
	// presents the question; only enter, esc and ctrl+c mean anything to it)
	failing := func(value.Value) (bool, error) { return false, nil }
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	esc := tea.KeyMsg{Type: tea.KeyEsc}
	tests := []struct {
		name       string
		model      textModel
		keys       []tea.KeyMsg
		typed      string
		wantValue  string
		wantDone   bool
		wantQuit   bool
		wantInView string
	}{
		{
			name:      "given a default, when enter is pressed, then the default is the answer",
			model:     newTextModel(ask.Question{Task: "t", VarName: "ENV", Default: "staging"}),
			keys:      []tea.KeyMsg{enter},
			wantValue: "staging", wantDone: true,
		},
		{
			name:      "given a typed value, when enter is pressed, then it is the answer",
			model:     newTextModel(ask.Question{Task: "t", VarName: "ENV"}),
			typed:     "prod",
			keys:      []tea.KeyMsg{enter},
			wantValue: "prod", wantDone: true,
		},
		{
			name:     "given an optional prompt, when enter is pressed on an empty field, then it ends with no answer",
			model:    newTextModel(ask.Question{Task: "t", VarName: "N", Optional: true}),
			keys:     []tea.KeyMsg{enter},
			wantDone: true,
		},
		{
			name:       "given a required prompt with no default, when enter is pressed on an empty field, then the refusal shows",
			model:      newTextModel(ask.Question{Task: "t", VarName: "N"}),
			keys:       []tea.KeyMsg{enter},
			wantInView: "✗ an answer is required",
		},
		{
			name:     "given an optional prompt with a default, when esc is pressed, then it ends with no answer",
			model:    newTextModel(ask.Question{Task: "t", VarName: "N", Optional: true, Default: "keep"}),
			keys:     []tea.KeyMsg{esc},
			wantDone: true,
		},
		{
			name:  "given a required prompt, when esc is pressed, then nothing happens",
			model: newTextModel(ask.Question{Task: "t", VarName: "N"}),
			keys:  []tea.KeyMsg{esc},
		},
		{
			name:     "given any prompt, when ctrl+c is pressed, then it aborts",
			model:    newTextModel(ask.Question{Task: "t", VarName: "N", Optional: true}),
			keys:     []tea.KeyMsg{{Type: tea.KeyCtrlC}},
			wantQuit: true,
		},
		{
			name:       "given a refused answer, when enter is pressed, then the judge's refusal shows on the error line and the prompt stays open",
			model:      newTextModel(ask.Question{Task: "t", VarName: "N", Check: failing, CheckDesc: "N >= 1"}),
			typed:      "0",
			keys:       []tea.KeyMsg{enter},
			wantInView: "✗ validation failed: N >= 1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			model := typeText(test.model, test.typed)

			// Act
			for _, key := range test.keys {
				model = keyPress(model, key)
			}

			// Assert
			if model.answer.String() != test.wantValue || model.done != test.wantDone || model.quit != test.wantQuit {
				t.Errorf("got value=%q done=%v quit=%v, want value=%q done=%v quit=%v",
					model.answer.String(), model.done, model.quit, test.wantValue, test.wantDone, test.wantQuit)
			}
			if test.wantInView != "" && !strings.Contains(model.View(), test.wantInView) {
				t.Errorf("view %q, want it to contain %q", model.View(), test.wantInView)
			}
		})
	}
}
