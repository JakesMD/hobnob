package tui

import (
	"context"
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PromptText prompts for free text. validate, when non-nil, is called with
// the candidate value on Enter — errMsg uses checkDesc (the check:
// expression's own text) purely for display, so tui never needs to know how
// a check is evaluated or what a var's type is.
func PromptText(ctx context.Context, info, varName string, validate func(string) (bool, error), checkDesc, defaultVal, task string, secret, optional bool) (string, error) {
	model := newTextModel(info, varName, validate, checkDesc, defaultVal, task, secret, optional)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final := result.(textModel)
	if final.quit {
		return "", fmt.Errorf("aborted")
	}
	displayVal := final.value
	if secret {
		displayVal = SecretMask
	}
	printGetLine(task, final.varName, displayVal)
	return final.value, nil
}

type textModel struct {
	info       string
	validate   func(string) (bool, error)
	checkDesc  string
	varName    string
	task       string
	defaultVal string
	textInput  textinput.Model
	errMsg     string
	value      string
	quit       bool
	secret     bool
	optional   bool
	done       bool
	termWidth  int
}

func newTextModel(info, varName string, validate func(string) (bool, error), checkDesc, defaultVal, task string, secret, optional bool) textModel {
	textInput := textinput.New()
	textInput.Prompt = SArrow.Render("› ")
	if secret {
		textInput.EchoMode = textinput.EchoPassword
		textInput.EchoCharacter = '•'
	}
	textInput.SetValue(defaultVal)
	textInput.Focus()
	return textModel{
		info:       info,
		validate:   validate,
		checkDesc:  checkDesc,
		varName:    varName,
		task:       task,
		defaultVal: defaultVal,
		textInput:  textInput,
		secret:     secret,
		optional:   optional,
	}
}

func (model textModel) Init() tea.Cmd { return textinput.Blink }

func (model textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		model.termWidth = size.Width
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
		if model.optional {
			model.done = true
			return model, tea.Quit
		}
		return model, nil
	case tea.KeyEnter:
		val := model.textInput.Value()
		if model.optional && val == "" {
			model.done = true
			return model, tea.Quit
		}
		if val == "" && model.defaultVal != "" {
			val = model.defaultVal
		}
		if model.validate != nil {
			ok, err := model.validate(val)
			if err != nil || !ok {
				model.errMsg = fmt.Sprintf("validation failed: %s", model.checkDesc)
				return model, nil
			}
		}
		model.value = val
		model.done = true
		return model, tea.Quit
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
	if model.optional {
		hints = append(hints, hint{"esc", "skip"})
	}
	return card{
		task:      model.task,
		varName:   model.varName,
		optional:  model.optional,
		info:      model.info,
		body:      model.textInput.View(),
		errMsg:    model.errMsg,
		hints:     hints,
		termWidth: model.termWidth,
	}.render()
}
