package tui

import (
	"context"
	"fmt"
	"strings"

	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

func newSelectModel(varName, info string, items []string, multi bool, defaultVal string) selectModel {
	cursor := 0
	preselected := make(map[int]bool)
	if defaultVal != "" {
		for i, item := range items {
			if item == defaultVal {
				cursor = i
				if multi {
					preselected[i] = true
				}
				break
			}
		}
	}
	return selectModel{
		varName:  varName,
		info:     info,
		items:    items,
		multi:    multi,
		cursor:   cursor,
		selected: preselected,
	}
}

func PromptSelect(ctx context.Context, varName, info string, items []string, multi bool, defaultVal string, task string, secret bool) (value.Value, error) {
	model := newSelectModel(varName, info, items, multi, defaultVal)
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return value.Value{}, err
	}
	final := result.(selectModel)
	if final.quit {
		return value.Value{}, fmt.Errorf("aborted")
	}

	var chosen value.Value
	display := final.value
	if final.multi {
		arr := make([]any, len(final.values))
		for i, val := range final.values {
			arr[i] = val
		}
		chosen = value.Of(arr)
		display = strings.Join(final.values, ", ")
	} else {
		chosen = value.Str(final.value)
	}
	if secret {
		display = SecretMask
	}
	printGetLine(task, final.varName, display)
	return chosen, nil
}

// ── select model ─────────────────────────────────────────────────────────────

type selectModel struct {
	varName  string
	info     string
	items    []string
	multi    bool
	cursor   int
	selected map[int]bool
	value    string   // single-select result
	values   []string // multi-select result
	done     bool
	quit     bool
}

func (model selectModel) Init() tea.Cmd { return nil }

func (model selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	itemCount := len(model.items)
	switch keyMsg.String() {
	case "ctrl+c", "q":
		model.quit = true
		return model, tea.Quit
	case "up", "k":
		if model.cursor > 0 {
			model.cursor--
		}
	case "down", "j":
		if model.cursor < itemCount-1 {
			model.cursor++
		}
	case " ":
		if model.multi {
			model.selected[model.cursor] = !model.selected[model.cursor]
		}
	case "enter":
		if model.multi {
			vals := []string{}
			for i, item := range model.items {
				if model.selected[i] {
					vals = append(vals, item)
				}
			}
			model.values = vals
		} else {
			model.value = model.items[model.cursor]
		}
		model.done = true
		return model, tea.Quit
	}
	return model, nil
}

func (model selectModel) View() string {
	if model.done {
		return ""
	}
	verb := "Select a value"
	if model.multi {
		verb = "Select values"
	}
	header := "\n" + verb + " for " + SLabel.Render(model.varName) + "."
	if model.info != "" {
		header += "\n" + SInfo.Render(model.info)
	}
	var builder strings.Builder
	builder.WriteString(header + "\n")
	for i, item := range model.items {
		if model.cursor == i {
			builder.WriteString(SArrow.Render("▶ "))
		} else {
			builder.WriteString("  ")
		}
		if model.multi {
			if model.selected[i] {
				builder.WriteString(SChecked.Render("◉ "))
			} else {
				builder.WriteString(SInfo.Render("○ "))
			}
		}
		builder.WriteString(item + "\n")
	}
	if model.multi {
		builder.WriteString(SHint.Render("↑↓ move  space toggle  enter confirm"))
	} else {
		builder.WriteString(SHint.Render("↑↓ move  enter select"))
	}
	return builder.String()
}
