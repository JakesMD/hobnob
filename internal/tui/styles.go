package tui

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// IsInterrupted reports whether err came from a prompt being torn down by ctx
// cancellation (a tea.Program's WithContext, or a web prompt's context),
// rather than the user declining the prompt normally.
func IsInterrupted(err error) bool {
	return errors.Is(err, tea.ErrProgramKilled) || errors.Is(err, context.Canceled)
}

var (
	SLabel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	SInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SBold    = lipgloss.NewStyle().Bold(true)
	SArrow   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	SChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	SError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	SHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SStep    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	colorBorder      = lipgloss.Color("8")
	colorBorderError = lipgloss.Color("9")

	sPathMatch = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	sPathDir   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	sPathInput = lipgloss.NewStyle().Bold(true)

	TaskPalette = []lipgloss.Color{"2", "3", "4", "5", "6", "10", "11", "12", "13", "14"}
)

// SecretMask replaces a secret value in both prompt display output (here)
// and the actual command-line masking runner.maskSecrets does — kept as one
// constant so the display placeholder and the real masking value can't drift
// apart.
const SecretMask = "****"

func TaskStyle(task string) lipgloss.Style {
	checksum := 0
	for _, char := range task {
		checksum += int(char)
	}
	return lipgloss.NewStyle().Foreground(TaskPalette[checksum%len(TaskPalette)])
}

// lead is the start of every log line: the task in its colour, then the
// mark that says what the line is, then a space. mark is rendered by the
// caller, so a line that carries its own colour (a grey │, a green ✓) keeps it.
func lead(task, mark string) string {
	return TaskStyle(task).Render(task) + " " + mark + " "
}

// taskMark renders a mark bold in the task's colour.
func taskMark(task, mark string) string {
	return TaskStyle(task).Bold(true).Render(mark)
}
