package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const (
	cardMaxWidth     = 76
	cardDefaultWidth = 80
	cardPadding      = 2
)

var (
	sBorder  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	sBorderE = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	sKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true)
)

// hint is one entry of the line under a card. An empty key makes it plain
// dim text, such as "type to filter".
type hint struct{ key, action string }

// card is the frame every prompt renders itself in: only body differs
// between prompt types. termWidth is the terminal's width, 0 until known.
type card struct {
	task, varName string
	optional      bool
	info          string
	body          string
	errMsg        string
	hints         []hint
	termWidth     int
}

func (c card) title() string {
	title := TaskStyle(c.task).Render(c.task) + " " + TaskStyle(c.task).Render("»") + " " + SLabel.Render(c.varName)
	if c.optional {
		title += " " + SHint.Render("(optional)")
	}
	return title
}

func (c card) outerWidth() int {
	width := c.termWidth
	if width <= 0 {
		width = cardDefaultWidth
	}
	return min(width, cardMaxWidth)
}

func (c card) render() string {
	outer := c.outerWidth()
	inner := outer - 2 - 2*cardPadding

	var content []string
	if c.info != "" {
		content = append(content, wrapLines(c.info, inner)...)
		content = append(content, "")
	}
	content = append(content, c.body)
	if c.errMsg != "" {
		content = append(content, "", SError.Render("✗ "+c.errMsg))
	}

	border, borderStyle := lipgloss.Color("8"), sBorder
	if c.errMsg != "" {
		border, borderStyle = lipgloss.Color("9"), sBorderE
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(border).
		Padding(1, cardPadding).Width(outer - 2).Render(strings.Join(content, "\n"))

	lines := strings.Split(box, "\n")
	title := c.title()
	fill := outer - lipgloss.Width(title) - 5
	lines[0] = borderStyle.Render("╭─ ") + title + " " + borderStyle.Render(strings.Repeat("─", max(fill, 1))+"╮")
	return strings.Join(lines, "\n") + "\n " + renderHints(c.hints)
}

func wrapLines(text string, width int) []string {
	lines := strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

func renderHints(hints []hint) string {
	parts := make([]string, len(hints))
	for i, h := range hints {
		if h.key == "" {
			parts[i] = SHint.Render(h.action)
			continue
		}
		parts[i] = sKey.Render(h.key) + " " + SHint.Render(h.action)
	}
	return strings.Join(parts, SHint.Render(" • "))
}
