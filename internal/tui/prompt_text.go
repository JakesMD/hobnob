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
	model := newTextModel(
		card{task: task, varName: varName, optional: optional, info: info},
		textOptions{validate: validate, checkDesc: checkDesc, defaultVal: defaultVal, secret: secret},
	)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final := result.(textModel)
	if final.quit {
		return "", fmt.Errorf("aborted")
	}
	return final.value, nil
}

type textOptions struct {
	validate   func(string) (bool, error)
	checkDesc  string
	defaultVal string
	secret     bool
}

const secretDefaultMask = "••••"

type textModel struct {
	frame      card
	validate   func(string) (bool, error)
	checkDesc  string
	defaultVal string
	textInput  textinput.Model
	// maskedDefault is set for a secret default, which shows as a fixed
	// mask until edited so its length isn't revealed.
	maskedDefault bool
	value         string
	quit          bool
	done          bool
}

func newTextModel(frame card, opts textOptions) textModel {
	textInput := textinput.New()
	textInput.Prompt = SArrow.Render("› ")
	if opts.secret {
		textInput.EchoMode = textinput.EchoPassword
		textInput.EchoCharacter = '•'
	}
	textInput.SetValue(opts.defaultVal)
	textInput.Focus()
	return textModel{
		frame:      frame,
		validate:   opts.validate,
		checkDesc:  opts.checkDesc,
		defaultVal: opts.defaultVal,
		textInput:  textInput,

		maskedDefault: opts.secret && opts.defaultVal != "",
	}
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
		if model.frame.optional {
			model.done = true
			return model, tea.Quit
		}
		return model, nil
	case tea.KeyEnter:
		val := model.textInput.Value()
		if model.frame.optional && val == "" {
			model.done = true
			return model, tea.Quit
		}
		if val == "" && model.defaultVal != "" {
			val = model.defaultVal
		}
		if model.validate != nil {
			ok, err := model.validate(val)
			if err != nil || !ok {
				model.frame.errMsg = fmt.Sprintf("validation failed: %s", model.checkDesc)
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
	if model.frame.optional {
		hints = append(hints, hint{"esc", "skip"})
	}
	frame := model.frame
	frame.body = model.textInput.View()
	if model.maskedDefault && model.textInput.Value() == model.defaultVal {
		frame.body = model.textInput.Prompt + secretDefaultMask
	}
	frame.hints = hints
	return frame.render()
}
