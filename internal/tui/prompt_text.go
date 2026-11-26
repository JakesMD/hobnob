package tui

import (
	"context"
	"fmt"

	"hobnob/internal/ask"
	"hobnob/internal/value"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PromptAsk shows q in the terminal: a menu when it has options, else a
// text box. It returns what q's judge accepted, or the empty value for a
// skipped optional question.
func PromptAsk(ctx context.Context, q ask.Question) (value.Value, error) {
	var model tea.Model = newTextModel(q)
	if q.IsMenu() {
		model = newSelectModel(q)
	}
	result, err := tea.NewProgram(model, tea.WithContext(ctx)).Run()
	if err != nil {
		return value.Value{}, err
	}
	switch final := result.(type) {
	case textModel:
		return final.answer, final.finished()
	case selectModel:
		return final.answer, final.finished()
	}
	return value.Value{}, fmt.Errorf("unexpected prompt model %T", result)
}

const secretDefaultMask = "••••"

type textModel struct {
	question  ask.Question
	frame     card
	textInput textinput.Model
	// maskedDefault is set for a secret default, which shows as a fixed
	// mask until edited so its length isn't revealed.
	maskedDefault bool
	answer        value.Value
	quit          bool
	done          bool
}

func newTextModel(q ask.Question) textModel {
	textInput := textinput.New()
	textInput.Prompt = SArrow.Render("› ")
	if q.Secret {
		textInput.EchoMode = textinput.EchoPassword
		textInput.EchoCharacter = '•'
	}
	textInput.SetValue(q.Default)
	textInput.Focus()
	return textModel{
		question:      q,
		frame:         card{task: q.Task, varName: q.VarName, optional: q.Optional, info: q.Info},
		textInput:     textInput,
		maskedDefault: q.Secret && q.Default != "",
	}
}

func (model textModel) finished() error {
	if model.quit {
		return fmt.Errorf("aborted")
	}
	return nil
}

// judge takes the verdict on answer: a refusal shows on the error line, an
// acceptance or a skip ends the prompt.
func (model textModel) judge(answer ask.Answer) (tea.Model, tea.Cmd) {
	verdict := model.question.Judge(answer)
	if verdict.Refusal != "" {
		model.frame.errMsg = verdict.Refusal
		return model, nil
	}
	model.answer = verdict.Value
	model.done = true
	return model, tea.Quit
}

func (model textModel) Init() tea.Cmd { return textinput.Blink }

func (model textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		model.frame.termWidth = size.Width
		return model, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		model.textInput, cmd = model.textInput.Update(msg)
		return model, cmd
	}
	switch keyMsg.Type {
	case tea.KeyCtrlC:
		model.quit = true
		return model, tea.Quit
	case tea.KeyEsc:
		if model.question.Optional {
			return model.judge(ask.Answer{})
		}
		return model, nil
	case tea.KeyEnter:
		return model.judge(ask.Answer{Text: model.textInput.Value()})
	}
	var cmd tea.Cmd
	model.textInput, cmd = model.textInput.Update(keyMsg)
	return model, cmd
}

func (model textModel) View() string {
	if model.done {
		return ""
	}
	hints := []hint{{"enter", "confirm"}}
	if model.frame.optional {
		hints = append(hints, hint{"esc", "skip"})
	}
	frame := model.frame
	frame.body = model.textInput.View()
	if model.maskedDefault && model.textInput.Value() == model.question.Default {
		frame.body = model.textInput.Prompt + secretDefaultMask
	}
	frame.hints = hints
	return frame.render()
}
