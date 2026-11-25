package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestCard_Render_TopEdgeMatchesCardWidth(t *testing.T) {
	// given a title wider than the card, when rendered, then the title is
	// truncated so the top edge is as wide as the rest of the box (why: an
	// overflowing border breaks the box on narrow terminals)
	tests := []struct {
		name  string
		frame card
	}{
		{
			name: "given a long optional title on a narrow terminal, when rendered, then every line is as wide as the card",
			frame: card{
				task: "some:long:task:name", varName: "A_VERY_LONG_VARIABLE_NAME_FOR_TESTING",
				optional: true, body: "body", termWidth: 40,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			frame := test.frame

			// Act
			lines := strings.Split(frame.render(), "\n")

			// Assert
			want := lipgloss.Width(lines[1])
			if got := lipgloss.Width(lines[0]); got != want || want != 40 {
				t.Errorf("top edge is %d wide, second line %d, want both 40: %q", got, want, lines[0])
			}
			if !strings.Contains(lines[0], "…") {
				t.Errorf("top edge %q, want the title truncated with an ellipsis", lines[0])
			}
		})
	}
}

func TestCard_Render_BorderColour(t *testing.T) {
	// given a card with and without an error, when rendered in colour, then
	// the whole border is red only while the error shows (why: the border
	// is how a failed answer is noticed at a glance)
	tests := []struct {
		name       string
		errMsg     string
		wantEscape string
	}{
		{name: "given no error, when rendered, then the border is dim", wantEscape: "\x1b[90m"},
		{name: "given an error, when rendered, then the border is red", errMsg: "bad", wantEscape: "\x1b[91m"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			previous := lipgloss.ColorProfile()
			lipgloss.SetColorProfile(termenv.ANSI)
			t.Cleanup(func() { lipgloss.SetColorProfile(previous) })
			frame := card{task: "t", varName: "V", body: "body", errMsg: test.errMsg}

			// Act
			lines := strings.Split(frame.render(), "\n")

			// Assert
			bottom := lines[len(lines)-2]
			for name, line := range map[string]string{"top edge": lines[0], "bottom edge": bottom} {
				if !strings.Contains(line, test.wantEscape) {
					t.Errorf("%s %q, want it to contain %q", name, line, test.wantEscape)
				}
			}
		})
	}
}
