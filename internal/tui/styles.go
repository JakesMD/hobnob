package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// IsInterrupted reports whether err came from a prompt's tea.Program being
// torn down by ctx cancellation (see WithContext below), rather than the user
// declining the prompt normally.
func IsInterrupted(err error) bool {
	return errors.Is(err, tea.ErrProgramKilled)
}

var (
	SLabel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	SInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SArrow   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	SChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	SError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	SHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SStep    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

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

func TaskPrefix(task string) string {
	return "[" + TaskStyle(task).Render(task) + "] "
}

func printGetLine(task, varName, displayVal string) {
	fmt.Println(SStep.Render("get:") + " " + TaskPrefix(task) + SStep.Render(varName+": "+displayVal))
}

func SkipLine(task string) string {
	return SInfo.Render("⊘") + " " + TaskPrefix(task) + SInfo.Render("skipped")
}

func RunSkipLine(task string) string {
	return SStep.Render("run:") + " " + TaskPrefix(task) + SInfo.Render("skipped (if: false)")
}

func RunDisplayLines(cmd, task, dir string) []string {
	prefix := TaskPrefix(task)
	lines := strings.Split(cmd, "\n")
	result := make([]string, len(lines))
	for i, line := range lines {
		if i == 0 {
			first := SStep.Render("run:") + " " + prefix + SStep.Render(line)
			if dir != "" {
				first += " " + SHint.Render("(dir: "+dir+")")
			}
			result[i] = first
		} else {
			result[i] = SStep.Render(line)
		}
	}
	return result
}
