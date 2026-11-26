package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// PromptSay shows message in a card and waits for Enter.
func PromptSay(ctx context.Context, message, task string) error {
	program := tea.NewProgram(newSayModel(card{task: task, body: message}), tea.WithContext(ctx))
	result, err := program.Run()
	if err != nil {
		return err
	}
	if result.(sayModel).quit {
		return fmt.Errorf("aborted")
	}
	return nil
}

type sayModel struct {
	frame card
	quit  bool
	done  bool
}

func newSayModel(frame card) sayModel {
	frame.hints = []hint{{"enter", "continue"}}
	return sayModel{frame: frame}
}

func (model sayModel) Init() tea.Cmd { return nil }

func (model sayModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		model.frame.termWidth = msg.Width
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			model.quit = true
			return model, tea.Quit
		case tea.KeyEnter:
			model.done = true
			return model, tea.Quit
		}
	}
	return model, nil
}

func (model sayModel) View() string {
	if model.done {
		return ""
	}
	frame := model.frame
	frame.body = wrapText(frame.body, frame.innerWidth())
	return frame.render()
}
