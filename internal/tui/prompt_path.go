package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"hobnob/internal/ask"
	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
)

const pathListHeight = 12

type pathEntry struct {
	name     string
	isDir    bool
	pickable bool
}

type pathRow struct {
	pathEntry
	here      bool
	positions []int
}

type pathModel struct {
	question  ask.Question
	rules     ask.PathRules
	errMsg    string
	text      string // the filter line: always an absolute path; up to the last "/" is the listed directory
	cursor    int
	offset    int
	answer    value.Value
	picks     []string // multi: absolute paths picked so far, in pick order
	done      bool
	quit      bool
	termWidth int
}

func newPathModel(q ask.Question) pathModel {
	m := pathModel{question: q, rules: *q.Path}
	open, picks := m.rules.Opening(q.Multi)
	m.text = withSlash(open)
	if q.Multi {
		m.picks = picks
	}
	if target := m.rules.DefaultTarget(); target != "" {
		m.highlight(target)
	}
	return m
}

func withSlash(p string) string {
	p = filepath.ToSlash(p)
	if strings.HasSuffix(p, "/") {
		return p
	}
	return p + "/"
}

func (m *pathModel) highlight(abs string) {
	for i, row := range m.rows() {
		if !row.here && filepath.Join(m.dir(), row.name) == abs {
			m.cursor = i
		}
	}
	m.clamp()
}

// dirText is the text up to and including its last "/": the directory the
// listing shows.
func (m pathModel) dirText() string {
	return m.text[:strings.LastIndex(m.text, "/")+1]
}

// seg is the text after the last "/": the only part that is fuzzy-matched.
func (m pathModel) seg() string {
	return m.text[strings.LastIndex(m.text, "/")+1:]
}

// dir is the absolute directory being listed.
func (m pathModel) dir() string { return filepath.Clean(filepath.FromSlash(m.dirText())) }

func (m pathModel) dirExists() bool {
	info, err := os.Stat(m.dir())
	return err == nil && info.IsDir()
}

func (m pathModel) entries() []pathEntry {
	listed := m.rules.List(m.dir())
	out := make([]pathEntry, len(listed))
	for i, e := range listed {
		out[i] = pathEntry{e.Name, e.Dir, e.Pickable}
	}
	return out
}

func (m pathModel) rows() []pathRow {
	var out []pathRow
	if !m.dirExists() {
		return nil
	}
	seg := m.seg()
	if m.rules.Dir && seg == "" {
		here := m.rules.Allows(m.dir(), true)
		out = append(out, pathRow{pathEntry: pathEntry{name: ".", isDir: true, pickable: here}, here: true})
	}
	entries := m.entries()
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.name
	}
	for _, hit := range matchRows(names, seg) {
		out = append(out, pathRow{pathEntry: entries[hit.index], positions: hit.positions})
	}
	return out
}

// renderRow draws one picker row: cursor, pick marker, then the name.
func (m pathModel) renderRow(rows []pathRow) func(i int) string {
	return func(i int) string {
		row := rows[i]
		var b strings.Builder
		b.WriteString(cursorPrefix(i == m.cursor))
		if m.question.Multi {
			switch {
			case m.isPicked(m.rowPath(row)):
				b.WriteString(SChecked.Render("◉ "))
			case row.pickable:
				b.WriteString(SInfo.Render("○ "))
			default:
				b.WriteString("  ")
			}
		}
		switch {
		case row.here:
			b.WriteString(sPathDir.Render("./") + SInfo.Render(" (this directory)"))
		case row.isDir:
			b.WriteString(sPathDir.Render(highlightMatch(row.name, row.positions) + "/"))
		default:
			b.WriteString(highlightMatch(row.name, row.positions))
		}
		return b.String()
	}
}

func (m pathModel) finished() error {
	if m.quit {
		return fmt.Errorf("aborted")
	}
	return nil
}

func (m pathModel) Init() tea.Cmd { return nil }

func (m pathModel) highlighted() (pathRow, bool) {
	rows := m.rows()
	if m.cursor < 0 || m.cursor >= len(rows) {
		return pathRow{}, false
	}
	return rows[m.cursor], true
}

func (m *pathModel) clamp() {
	m.cursor = max(0, min(m.cursor, len(m.rows())-1))
	m.offset = scrollTo(m.cursor, m.offset, pathListHeight)
}

func (m *pathModel) setText(text string) {
	m.text = text
	m.cursor, m.offset = 0, 0
}

func (m *pathModel) up() {
	prev := m.dir()
	m.setText(withSlash(filepath.Dir(prev)))
	for i, row := range m.rows() {
		if filepath.Join(m.dir(), row.name) == prev {
			m.cursor = i
		}
	}
	m.clamp()
}

func (m pathModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.termWidth = size.Width
		return m, nil
	}
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	m.errMsg = ""
	row, hasRow := m.highlighted()
	switch key.String() {
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up":
		m.cursor--
	case "down":
		m.cursor++
	case "left":
		m.up()
	case "right":
		if hasRow && row.isDir && !row.here {
			m.setText(m.dirText() + row.name + "/")
		}
	case "backspace":
		if m.text != "/" {
			runes := []rune(m.text)
			m.setText(string(runes[:len(runes)-1]))
		}
	case "ctrl+u":
		m.setText(m.dirText())
	case "esc":
		switch {
		case m.seg() != "":
			m.setText(m.dirText())
		case m.question.Optional:
			return m.judge(ask.Answer{})
		}
	case "tab":
		switch {
		case !hasRow:
		case m.question.Multi:
			if row.pickable {
				m.toggle(m.rowPath(row))
				m.cursor++
			} else {
				m.errMsg = "can't pick that"
			}
		case !row.here:
			m.setText(m.dirText() + row.name)
		}
	case "enter":
		switch {
		case m.question.Multi && len(m.picks) > 0:
			return m.judge(ask.Answer{Picks: m.picks})
		case !hasRow:
			m.errMsg = "nothing matches"
		case row.isDir && !row.here && !m.rules.Dir:
			m.errMsg = "that's a directory: → opens it"
		case !row.pickable:
			m.errMsg = "doesn't match"
		default:
			return m.judge(ask.Answer{Picks: []string{m.rowPath(row)}})
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			text := string(key.Runes)
			if key.Type == tea.KeySpace {
				text = " "
			}
			m.setText(m.text + text)
		}
	}
	m.clamp()
	return m, nil
}

func (m pathModel) rowPath(row pathRow) string {
	if row.here {
		return m.dir()
	}
	return filepath.Join(m.dir(), row.name)
}

func (m *pathModel) toggle(path string) {
	for i, p := range m.picks {
		if p == path {
			m.picks = append(append([]string{}, m.picks[:i]...), m.picks[i+1:]...)
			return
		}
	}
	m.picks = append(m.picks, path)
}

func (m pathModel) isPicked(path string) bool {
	for _, p := range m.picks {
		if p == path {
			return true
		}
	}
	return false
}

// judge takes the verdict on answer: a refusal shows on the error line, an
// acceptance or a skip ends the prompt.
func (m pathModel) judge(answer ask.Answer) (tea.Model, tea.Cmd) {
	verdict := m.question.Judge(answer)
	if verdict.Refusal != "" {
		m.errMsg = verdict.Refusal
		return m, nil
	}
	m.answer = verdict.Value
	m.done = true
	return m, tea.Quit
}

// pathLimit is how many characters of the path the filter line shows.
const pathLimit = 40

// shortPath cuts a long path from the left, "…" standing for the cut part.
// Display only: typing and backspace edit the full path.
func shortPath(text string) string {
	runes := []rune(text)
	if len(runes) <= pathLimit {
		return text
	}
	return "…" + string(runes[len(runes)-pathLimit+1:])
}

func (m pathModel) View() string {
	if m.done {
		return ""
	}
	frame := card{
		task:      m.question.Task,
		varName:   m.question.VarName,
		optional:  m.question.Optional,
		info:      m.question.Info,
		errMsg:    m.errMsg,
		termWidth: m.termWidth,
	}
	tab := hint{"tab", "complete"}
	enter := hint{"enter", "pick"}
	if m.question.Multi {
		tab, enter = hint{"tab", "toggle"}, hint{"enter", "confirm"}
	}
	frame.hints = []hint{{"", "type to filter"}, {"↑↓", "move"}, {"←", "up"}, {"→", "open"}, tab, enter}
	if m.question.Optional {
		frame.hints = append(frame.hints, hint{"esc", "skip"})
	}

	var lines []string
	if len(m.picks) > 0 {
		lines = append(lines, SInfo.Render(fmt.Sprintf("picked (%d):", len(m.picks))))
		for _, p := range m.picks {
			lines = append(lines, "  "+SChecked.Render("◉ ")+p)
		}
		lines = append(lines, "")
	}
	lines = append(lines, filterLine(shortPath(m.text), frame.innerWidth())...)
	rows := m.rows()
	if m.dirExists() {
		lines = append(lines, renderWindow(len(rows), m.offset, pathListHeight, m.renderRow(rows))...)
	} else {
		lines = append(lines, SInfo.Render(fmt.Sprintf("  no directory %q", m.dirText())))
	}
	frame.body = strings.Join(lines, "\n")
	return frame.render()
}
