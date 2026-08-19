package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ── task select model ────────────────────────────────────────────────

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
	tasks  []TaskItem
	cursor int
	value  string
	quit   bool
}

func (model taskSelectModel) Init() tea.Cmd { return nil }

func (model taskSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return model, nil
	}
	switch keyMsg.String() {
	case "ctrl+c", "q":
		model.quit = true
		return model, tea.Quit
	case "up", "k":
		if model.cursor > 0 {
			model.cursor--
		}
	case "down", "j":
		if model.cursor < len(model.tasks)-1 {
			model.cursor++
		}
	case "enter":
		model.value = model.tasks[model.cursor].Name
		return model, tea.Quit
	}
	return model, nil
}

func (model taskSelectModel) View() string {
	if model.value != "" {
		return ""
	}

	maxNameLen := 0
	for _, task := range model.tasks {
		if width := lipgloss.Width(task.Name); width > maxNameLen {
			maxNameLen = width
		}
	}

	var builder strings.Builder
	builder.WriteString("\nSelect a task to run.\n")
	for i, task := range model.tasks {
		if model.cursor == i {
			builder.WriteString(SArrow.Render("▶ "))
		} else {
			builder.WriteString("  ")
		}
		if task.Info != "" {
			pad := strings.Repeat(" ", maxNameLen-lipgloss.Width(task.Name))
			builder.WriteString(task.Name + pad + "  " + SInfo.Render(task.Info) + "\n")
		} else {
			builder.WriteString(task.Name + "\n")
		}
	}
	builder.WriteString(SHint.Render("↑↓ move  enter select"))
	return builder.String()
}
