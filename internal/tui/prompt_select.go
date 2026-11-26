package tui

import (
	"fmt"
	"slices"
	"strings"

	"hobnob/internal/ask"
	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

func newSelectModel(q ask.Question) selectModel {
	cursor := 0
	selected := make(map[int]bool)
	for i, item := range q.Options {
		if len(q.Picks) > 0 && item == q.Picks[0] {
			cursor = i
		}
		if q.Multi && slices.Contains(q.Picks, item) {
			selected[i] = true
		}
	}
	return selectModel{
		question:  q,
		items:     q.Options,
		listState: listState{cursor: cursor, offset: scrollTo(cursor, 0, menuListHeight)},
		selected:  selected,
	}
}

type selectModel struct {
	question ask.Question
	items    []string
	listState
	selected  map[int]bool
	answer    value.Value
	errMsg    string
	done      bool
	quit      bool
	termWidth int
}

func (model selectModel) finished() error {
	if model.quit {
		return fmt.Errorf("aborted")
	}
	return nil
}

func (model selectModel) Init() tea.Cmd { return nil }

func (model selectModel) rows() []listRow { return matchRows(model.items, model.filter) }

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
		case model.question.Optional:
			return model.judge(ask.Answer{})
		}
	case tea.KeyTab:
		if rows := model.rows(); model.question.Multi && len(rows) > 0 {
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
	if model.question.Multi {
		return model.judge(ask.Answer{Picks: model.picks()})
	}
	rows := model.rows()
	if len(rows) == 0 {
		return model, nil
	}
	return model.judge(ask.Answer{Picks: []string{model.items[rows[model.cursor].index]}})
}

// judge takes the verdict on answer: a refusal shows on the error line, an
// acceptance or a skip ends the prompt.
func (model selectModel) judge(answer ask.Answer) (tea.Model, tea.Cmd) {
	verdict := model.question.Judge(answer)
	if verdict.Refusal != "" {
		model.errMsg = verdict.Refusal
		return model, nil
	}
	model.answer = verdict.Value
	model.done = true
	return model, tea.Quit
}

func (model selectModel) View() string {
	if model.done {
		return ""
	}
	frame := card{
		task:      model.question.Task,
		varName:   model.question.VarName,
		optional:  model.question.Optional,
		info:      model.question.Info,
		errMsg:    model.errMsg,
		termWidth: model.termWidth,
	}
	hints := []hint{{"", "type to filter"}, {"↑↓", "move"}}
	if model.question.Multi {
		hints = append(hints, hint{"tab", "toggle"}, hint{"enter", "confirm"})
	} else {
		hints = append(hints, hint{"enter", "select"})
	}
	if model.question.Optional {
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
		if model.question.Multi {
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
