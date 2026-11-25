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
		offset:   scrollTo(cursor, 0, menuListHeight),
		selected: preselected,
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

	display := final.value
	if final.multi {
		display = strings.Join(final.values, ", ")
	}
	if secret {
		display = SecretMask
	}
	printGetLine(task, final.varName, display)
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
	filter    string
	cursor    int // position in the filtered rows
	offset    int // first visible filtered row
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
		case model.optional:
			model.done, model.skipped = true, true
			return model, tea.Quit
		}
	case tea.KeyUp:
		model.moveTo(model.cursor - 1)
	case tea.KeyDown:
		model.moveTo(model.cursor + 1)
	case tea.KeyTab:
		if rows := model.rows(); model.multi && len(rows) > 0 {
			idx := rows[model.cursor].index
			model.selected[idx] = !model.selected[idx]
			model.errMsg = ""
		}
	case tea.KeyBackspace:
		if runes := []rune(model.filter); len(runes) > 0 {
			model.setFilter(string(runes[:len(runes)-1]))
		}
	case tea.KeySpace:
		if model.filter != "" {
			model.setFilter(model.filter + " ")
		}
	case tea.KeyRunes:
		model.setFilter(model.filter + string(keyMsg.Runes))
	case tea.KeyEnter:
		return model.confirm()
	}
	return model, nil
}

func (model *selectModel) setFilter(filter string) {
	model.filter = filter
	model.cursor, model.offset = 0, 0
	model.errMsg = ""
}

func (model *selectModel) moveTo(cursor int) {
	model.cursor = max(0, min(cursor, len(model.rows())-1))
	model.offset = scrollTo(model.cursor, model.offset, menuListHeight)
	model.errMsg = ""
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
	var lines []string
	if model.filter != "" {
		lines = append(lines, filterLine(model.filter, frame.innerWidth())...)
	}
	lines = append(lines, renderWindow(len(rows), model.offset, menuListHeight, model.renderRow(rows))...)
	frame.body = strings.Join(lines, "\n")
	return frame.render()
}

func (model selectModel) renderRow(rows []listRow) func(i int) string {
	return func(i int) string {
		row := rows[i]
		var b strings.Builder
		if i == model.cursor {
			b.WriteString(SArrow.Render("▶ "))
		} else {
			b.WriteString("  ")
		}
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
