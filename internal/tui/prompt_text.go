package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// PromptText prompts for free text. validate, when non-nil, is called with
// the candidate value on Enter — errMsg uses checkDesc (the check:
// expression's own text) purely for display, so tui never needs to know how
// a check is evaluated or what a var's type is.
func PromptText(ctx context.Context, info, varName string, validate func(string) (bool, error), checkDesc, defaultVal, task string, secret, optional bool) (string, error) {
	textInput := textinput.New()
	textInput.Prompt = SArrow.Render("› ")
	if secret {
		textInput.EchoMode = textinput.EchoPassword
	}
	if defaultVal != "" {
		textInput.SetValue(defaultVal)
	}
	textInput.Focus()
	model := textModel{
		info:       info,
		validate:   validate,
		checkDesc:  checkDesc,
		varName:    varName,
		defaultVal: defaultVal,
		textInput:  textInput,
		secret:     secret,
		optional:   optional,
	}
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

// ── text input model ─────────────────────────────────────────────────────────

type textModel struct {
	info       string
	validate   func(string) (bool, error)
	checkDesc  string
	varName    string
	defaultVal string
	textInput  textinput.Model
	errMsg     string
	value      string
	quit       bool
	secret     bool
	optional   bool
	done       bool
}

func (model textModel) Init() tea.Cmd { return textinput.Blink }

func (model textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	header := "\nEnter a value for " + SLabel.Render(model.varName) + "."
	if model.optional {
		header += " " + SHint.Render("(optional, leave blank to skip)")
	}
	if model.info != "" {
		header += "\n" + SInfo.Render(model.info)
	}
	var builder strings.Builder
	builder.WriteString(header + "\n")
	builder.WriteString(model.textInput.View() + "\n")
	if model.errMsg != "" {
		builder.WriteString(SError.Render(model.errMsg) + "\n")
	}
	builder.WriteString(SHint.Render("enter ↵"))
	return builder.String()
}
