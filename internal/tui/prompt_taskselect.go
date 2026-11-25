package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type TaskItem struct {
	Name string
	Info string
}

func PromptTaskSelect(ctx context.Context, tasks []TaskItem) (string, error) {
	model := taskSelectModel{tasks: tasks}
	program := tea.NewProgram(model, tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final := result.(taskSelectModel)
	if final.quit {
		return "", fmt.Errorf("aborted")
	}
	return final.value, nil
}

type taskSelectModel struct {
	tasks []TaskItem
	listState
	value     string
	quit      bool
	termWidth int
}

func (model taskSelectModel) Init() tea.Cmd { return nil }

func (model taskSelectModel) rows() []listRow {
	names := make([]string, len(model.tasks))
	for i, task := range model.tasks {
		names[i] = task.Name
	}
	return matchRows(names, model.filter)
}

func (model taskSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		model.setFilter("")
	case tea.KeyEnter:
		if rows := model.rows(); len(rows) > 0 {
			model.value = model.tasks[rows[model.cursor].index].Name
			return model, tea.Quit
		}
	default:
		model.handleKey(keyMsg, func() int { return len(model.rows()) })
	}
	return model, nil
}

func (model taskSelectModel) View() string {
	if model.value != "" {
		return ""
	}
	frame := card{
		heading:   "Select a task to run.",
		termWidth: model.termWidth,
		hints:     []hint{{"", "type to filter"}, {"↑↓", "move"}, {"enter", "run"}},
	}

	maxNameLen := 0
	for _, task := range model.tasks {
		maxNameLen = max(maxNameLen, lipgloss.Width(task.Name))
	}

	rows := model.rows()
	frame.body = model.body(frame.innerWidth(), len(rows), func(i int) string {
		task := model.tasks[rows[i].index]
		line := cursorPrefix(i == model.cursor)
		line += highlightMatch(task.Name, rows[i].positions)
		if task.Info != "" {
			line += strings.Repeat(" ", maxNameLen-lipgloss.Width(task.Name)) + "  " + SInfo.Render(task.Info)
		}
		return line
	})
	return frame.render()
}
