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
	tasks     []TaskItem
	filter    string
	cursor    int // position in the filtered rows
	offset    int // first visible filtered row
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
	case tea.KeyUp:
		model.moveTo(model.cursor - 1)
	case tea.KeyDown:
		model.moveTo(model.cursor + 1)
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
		if rows := model.rows(); len(rows) > 0 {
			model.value = model.tasks[rows[model.cursor].index].Name
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model *taskSelectModel) setFilter(filter string) {
	model.filter = filter
	model.cursor, model.offset = 0, 0
}

func (model *taskSelectModel) moveTo(cursor int) {
	model.cursor = max(0, min(cursor, len(model.rows())-1))
	model.offset = scrollTo(model.cursor, model.offset, menuListHeight)
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
	var lines []string
	if model.filter != "" {
		lines = append(lines, filterLine(model.filter, frame.innerWidth())...)
	}
	lines = append(lines, renderWindow(len(rows), model.offset, menuListHeight, func(i int) string {
		task := model.tasks[rows[i].index]
		line := "  "
		if i == model.cursor {
			line = SArrow.Render("▶ ")
		}
		line += highlightMatch(task.Name, rows[i].positions)
		if task.Info != "" {
			line += strings.Repeat(" ", maxNameLen-lipgloss.Width(task.Name)) + "  " + SInfo.Render(task.Info)
		}
		return line
	})...)
	frame.body = strings.Join(lines, "\n")
	return frame.render()
}
