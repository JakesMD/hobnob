package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"hobnob/internal/eval"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	SLabel   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	SInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SArrow   = lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true)
	SChecked = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	SError   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	SHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	SStep    = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))

	STaskName = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	SParamReq = lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	SParamOpt = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	STagReq   = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	STagOpt   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	SDefault  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	TaskPalette = []lipgloss.Color{"2", "3", "4", "5", "6", "10", "11", "12", "13", "14"}
)

func TaskStyle(task string) lipgloss.Style {
	sum := 0
	for _, c := range task {
		sum += int(c)
	}
	return lipgloss.NewStyle().Foreground(TaskPalette[sum%len(TaskPalette)])
}

func TaskPrefix(task string) string {
	return "[" + TaskStyle(task).Render(task) + "] "
}

type LineWriter struct {
	w      io.Writer
	prefix string
	buf    bytes.Buffer
}

func NewLineWriter(w io.Writer, prefix string) *LineWriter {
	return &LineWriter{w: w, prefix: prefix}
}

func (lw *LineWriter) Write(p []byte) (int, error) {
	totalBytes := len(p)
	for _, byteVal := range p {
		if byteVal == '\n' {
			lw.w.Write([]byte(lw.prefix))
			lw.w.Write(lw.buf.Bytes())
			lw.w.Write([]byte{'\n'})
			lw.buf.Reset()
		} else {
			lw.buf.WriteByte(byteVal)
		}
	}
	return totalBytes, nil
}

// Flush writes any buffered partial line. Errors from w are discarded because
// w is always os.Stdout/os.Stderr — if those fail the process is dying anyway.
func (lw *LineWriter) Flush() {
	if lw.buf.Len() > 0 {
		lw.w.Write([]byte(lw.prefix))
		lw.w.Write(lw.buf.Bytes())
		lw.w.Write([]byte{'\n'})
		lw.buf.Reset()
	}
}

func printGetLine(task, varName, displayVal string) {
	fmt.Println(SStep.Render("get:") + " " + TaskPrefix(task) + SStep.Render(varName+": "+displayVal))
}

func SkipLine(task string) string {
	return SInfo.Render("⊘") + " " + TaskPrefix(task) + SInfo.Render("skipped")
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

func PromptText(info, check, varName string, vars map[string]string, defaultVal string, task string, secret bool, optional bool) (string, error) {
	ti := textinput.New()
	ti.Prompt = SArrow.Render("› ")
	if secret {
		ti.EchoMode = textinput.EchoPassword
	}
	if defaultVal != "" {
		ti.SetValue(defaultVal)
	}
	ti.Focus()
	model := textModel{
		info:       info,
		check:      check,
		varName:    varName,
		vars:       vars,
		defaultVal: defaultVal,
		ti:         ti,
		secret:     secret,
		optional:   optional,
	}
	program := tea.NewProgram(model)
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final := result.(textModel)
	if final.quit {
		return "", fmt.Errorf("aborted")
	}
	displayVal := final.value
	if secret {
		displayVal = "****"
	}
	printGetLine(task, final.varName, displayVal)
	return final.value, nil
}

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
		selected: preselected,
	}
}

func PromptSelect(varName, info string, items []string, multi bool, defaultVal string, task string, secret bool) (string, error) {
	model := newSelectModel(varName, info, items, multi, defaultVal)
	program := tea.NewProgram(model)
	result, err := program.Run()
	if err != nil {
		return "", err
	}
	final := result.(selectModel)
	if final.quit {
		return "", fmt.Errorf("aborted")
	}
	display := final.value
	if final.multi {
		var vals []string
		// final.value is JSON produced by the TUI model, not user input — unmarshal
		// can only fail if there's an upstream code bug. Display is best-effort; the
		// actual return value (final.value) is correct regardless.
		_ = json.Unmarshal([]byte(final.value), &vals)
		display = strings.Join(vals, ", ")
	}
	if secret {
		display = "****"
	}
	printGetLine(task, final.varName, display)
	return final.value, nil
}

// ── text input model ─────────────────────────────────────────────────────────

type textModel struct {
	info       string
	check      string
	varName    string
	vars       map[string]string
	defaultVal string
	ti         textinput.Model
	errMsg     string
	value      string
	quit       bool
	secret     bool
	optional   bool
	done       bool
}

func (m textModel) Init() tea.Cmd { return textinput.Blink }

func (m textModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.ti, cmd = m.ti.Update(msg)
		return m, cmd
	}
	switch keyMsg.Type {
	case tea.KeyCtrlC:
		m.quit = true
		return m, tea.Quit
	case tea.KeyEnter:
		val := m.ti.Value()
		if m.optional && val == "" {
			m.done = true
			return m, tea.Quit
		}
		if val == "" && m.defaultVal != "" {
			val = m.defaultVal
		}
		if m.check != "" {
			merged := eval.CopyVars(m.vars)
			merged[m.varName] = val
			ok, err := eval.EvalCondition(m.check, merged)
			if err != nil || !ok {
				m.errMsg = fmt.Sprintf("validation failed: %s", m.check)
				return m, nil
			}
		}
		m.value = val
		m.done = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.ti, cmd = m.ti.Update(keyMsg)
	return m, cmd
}

func (m textModel) View() string {
	if m.done {
		return ""
	}
	header := "\nEnter a value for " + SLabel.Render(m.varName) + "."
	if m.info != "" {
		header += "\n" + SInfo.Render(m.info)
	}
	var sb strings.Builder
	sb.WriteString(header + "\n")
	sb.WriteString(m.ti.View() + "\n")
	if m.errMsg != "" {
		sb.WriteString(SError.Render(m.errMsg) + "\n")
	}
	sb.WriteString(SHint.Render("enter ↵"))
	return sb.String()
}

// ── task select model ────────────────────────────────────────────────

type TaskItem struct {
	Name string
	Info string
}

func PromptTaskSelect(tasks []TaskItem) (string, error) {
	model := taskSelectModel{tasks: tasks}
	program := tea.NewProgram(model)
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

func (m taskSelectModel) Init() tea.Cmd { return nil }

func (m taskSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch keyMsg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.tasks)-1 {
			m.cursor++
		}
	case "enter":
		m.value = m.tasks[m.cursor].Name
		return m, tea.Quit
	}
	return m, nil
}

func (m taskSelectModel) View() string {
	if m.value != "" {
		return ""
	}

	maxNameLen := 0
	for _, t := range m.tasks {
		if len(t.Name) > maxNameLen {
			maxNameLen = len(t.Name)
		}
	}

	var sb strings.Builder
	sb.WriteString("\nSelect a task to run.\n")
	for i, t := range m.tasks {
		if m.cursor == i {
			sb.WriteString(SArrow.Render("▶ "))
		} else {
			sb.WriteString("  ")
		}
		if t.Info != "" {
			pad := strings.Repeat(" ", maxNameLen-len(t.Name))
			sb.WriteString(t.Name + pad + "  " + SInfo.Render(t.Info) + "\n")
		} else {
			sb.WriteString(t.Name + "\n")
		}
	}
	sb.WriteString(SHint.Render("↑↓ move  enter select"))
	return sb.String()
}

// ── select model ─────────────────────────────────────────────────────────────

type selectModel struct {
	varName  string
	info     string
	items    []string
	multi    bool
	cursor   int
	selected map[int]bool
	value    string
	quit     bool
}

func (m selectModel) Init() tea.Cmd { return nil }

func (m selectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	itemCount := len(m.items)
	switch keyMsg.String() {
	case "ctrl+c", "q":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < itemCount-1 {
			m.cursor++
		}
	case " ":
		if m.multi {
			m.selected[m.cursor] = !m.selected[m.cursor]
		}
	case "enter":
		if m.multi {
			vals := []string{}
			for i, item := range m.items {
				if m.selected[i] {
					vals = append(vals, item)
				}
			}
			jsonBytes, _ := json.Marshal(vals)
			m.value = string(jsonBytes)
		} else {
			m.value = m.items[m.cursor]
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m selectModel) View() string {
	if m.value != "" {
		return ""
	}
	verb := "Select a value"
	if m.multi {
		verb = "Select values"
	}
	header := "\n" + verb + " for " + SLabel.Render(m.varName) + "."
	if m.info != "" {
		header += "\n" + SInfo.Render(m.info)
	}
	var sb strings.Builder
	sb.WriteString(header + "\n")
	for i, item := range m.items {
		if m.cursor == i {
			sb.WriteString(SArrow.Render("▶ "))
		} else {
			sb.WriteString("  ")
		}
		if m.multi {
			if m.selected[i] {
				sb.WriteString(SChecked.Render("◉ "))
			} else {
				sb.WriteString(SInfo.Render("○ "))
			}
		}
		sb.WriteString(item + "\n")
	}
	if m.multi {
		sb.WriteString(SHint.Render("↑↓ move  space toggle  enter confirm"))
	} else {
		sb.WriteString(SHint.Render("↑↓ move  enter select"))
	}
	return sb.String()
}
