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
		varName:   varName,
		info:      info,
		items:     items,
		multi:     multi,
		listState: listState{cursor: cursor, offset: scrollTo(cursor, 0, menuListHeight)},
		selected:  preselected,
	}
}

// PromptSelect shows a menu. validate, when non-nil, judges the choice on
// enter (the whole list under multi) and checkDesc is what the error line
// shows when it refuses: tui never needs to know how a check is evaluated.
// Skipping an optional menu returns an empty value.
func PromptSelect(ctx context.Context, varName, info string, items []string, multi bool, defaultVal string, task string, secret, optional bool, validate func(value.Value) (bool, error), checkDesc string) (value.Value, error) {
	model := newSelectModel(varName, info, items, multi, defaultVal)
	model.task, model.optional, model.validate, model.checkDesc = task, optional, validate, checkDesc
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return value.Value{}, err
	}
	final := result.(selectModel)
	if final.quit {
		return value.Value{}, fmt.Errorf("aborted")
	}
	if final.skipped {
		if multi {
			return value.Of([]any{}), nil
		}
		return value.Str(""), nil
	}

	return final.answer(), nil
}

type selectModel struct {
	varName   string
	task      string
	info      string
	items     []string
	multi     bool
	optional  bool
	validate  func(value.Value) (bool, error)
	checkDesc string
	listState
	selected  map[int]bool
	value     string   // single-select result
	values    []string // multi-select result
	errMsg    string
	done      bool
	skipped   bool
	quit      bool
	termWidth int
}

func (model selectModel) Init() tea.Cmd { return nil }

func (model selectModel) rows() []listRow { return matchRows(model.items, model.filter) }

func (model selectModel) answer() value.Value {
	if !model.multi {
		return value.Str(model.value)
	}
	arr := make([]any, len(model.values))
	for i, val := range model.values {
		arr[i] = val
	}
	return value.Of(arr)
}

func (model selectModel) picks() []string {
	picked := []string{}
	for i, item := range model.items {
		if model.selected[i] {
			picked = append(picked, item)
		}
	}
	return picked
}

func (model selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		model.termWidth = size.Width
		return model, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch keyMsg.Type {
	case tea.KeyCtrlC:
		model.quit = true
		return model, tea.Quit
	case tea.KeyEsc:
		switch {
		case model.filter != "":
			model.setFilter("")
			model.errMsg = ""
		case model.optional:
			model.done, model.skipped = true, true
			return model, tea.Quit
		}
	case tea.KeyTab:
		if rows := model.rows(); model.multi && len(rows) > 0 {
			idx := rows[model.cursor].index
			model.selected[idx] = !model.selected[idx]
			model.errMsg = ""
		}
	case tea.KeyEnter:
		return model.confirm()
	default:
		if model.handleKey(keyMsg, func() int { return len(model.rows()) }) {
			model.errMsg = ""
		}
	}
	return model, nil
}

func (model selectModel) confirm() (tea.Model, tea.Cmd) {
	if model.multi {
		model.values = model.picks()
	} else {
		rows := model.rows()
		if len(rows) == 0 {
			return model, nil
		}
		model.value = model.items[rows[model.cursor].index]
	}
	if model.validate != nil && !(model.optional && model.answer().IsEmpty()) {
		ok, err := model.validate(model.answer())
		if err != nil || !ok {
			model.errMsg = fmt.Sprintf("validation failed: %s", model.checkDesc)
			return model, nil
		}
	}
	model.done = true
	return model, tea.Quit
}

func (model selectModel) View() string {
	if model.done {
		return ""
	}
	frame := card{
		task:      model.task,
		varName:   model.varName,
		optional:  model.optional,
		info:      model.info,
		errMsg:    model.errMsg,
		termWidth: model.termWidth,
	}
	hints := []hint{{"", "type to filter"}, {"↑↓", "move"}}
	if model.multi {
		hints = append(hints, hint{"tab", "toggle"}, hint{"enter", "confirm"})
	} else {
		hints = append(hints, hint{"enter", "select"})
	}
	if model.optional {
		hints = append(hints, hint{"esc", "skip"})
	}
	frame.hints = hints

	rows := model.rows()
	frame.body = model.body(frame.innerWidth(), len(rows), model.renderRow(rows))
	return frame.render()
}

func (model selectModel) renderRow(rows []listRow) func(i int) string {
	return func(i int) string {
		row := rows[i]
		var b strings.Builder
		b.WriteString(cursorPrefix(i == model.cursor))
		if model.multi {
			if model.selected[row.index] {
				b.WriteString(SChecked.Render("◉ "))
			} else {
				b.WriteString(SInfo.Render("○ "))
			}
		}
		b.WriteString(highlightMatch(model.items[row.index], row.positions))
		return b.String()
	}
}
