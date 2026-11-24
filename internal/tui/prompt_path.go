package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"hobnob/internal/value"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// PathPrompt is everything the picker takes in. Show is the caller's
// predicate for what the listing shows (tui never learns about match:); it is
// applied on top of the type and the junk-name list.
type PathPrompt struct {
	Type    string // "file" or "dir"
	VarName string
	Info    string
	Start   string // start directory as written: relative to Invoke, ~-based or absolute; empty means Invoke
	Invoke  string // absolute directory that relative paths resolve against
	Show    func(path string, isDir bool) bool
}

var (
	sPathMatch = lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true)
	sPathDir   = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
)

const pathListHeight = 12

// junkNames are never listed, on every OS (spec §5). Compared
// case-insensitively; "._*" is handled as a prefix.
var junkNames = []string{
	".DS_Store", ".Spotlight-V100", ".Trashes", ".fseventsd", ".TemporaryItems",
	"Icon\r", "Thumbs.db", "desktop.ini", "$RECYCLE.BIN", "System Volume Information",
}

func isJunk(name string) bool {
	if strings.HasPrefix(name, "._") {
		return true
	}
	for _, junk := range junkNames {
		if strings.EqualFold(name, junk) {
			return true
		}
	}
	return false
}

type pathEntry struct {
	name  string
	isDir bool
}

type pathRow struct {
	pathEntry
	here      bool
	positions []int
}

func (r pathRow) label() string {
	switch {
	case r.here:
		return "./ (this directory)"
	case r.isDir:
		return r.name + "/"
	}
	return r.name
}

type pathModel struct {
	prompt PathPrompt
	home   string
	form   string // how the directory is written: "rel", "home" or "abs"
	cwd    string // absolute directory being listed
	seg    string // the fuzzy-matched segment after the last "/"
	cursor int
	offset int
	errMsg string
	picked string
	done   bool
	quit   bool
}

func newPathModel(prompt PathPrompt) pathModel {
	home, _ := os.UserHomeDir()
	m := pathModel{prompt: prompt, home: home, form: "rel", cwd: filepath.Clean(prompt.Invoke)}
	switch {
	case prompt.Start == "":
	case prompt.Start == "~" || strings.HasPrefix(prompt.Start, "~/"):
		m.form = "home"
		m.cwd = filepath.Join(home, strings.TrimPrefix(prompt.Start, "~"))
	case filepath.IsAbs(prompt.Start):
		m.form = "abs"
		m.cwd = filepath.Clean(prompt.Start)
	default:
		m.cwd = filepath.Join(prompt.Invoke, prompt.Start)
	}
	return m
}

// dirText writes the listed directory the way the user started writing it.
func (m pathModel) dirText() string {
	withSlash := func(p string) string {
		if p == "/" || strings.HasSuffix(p, string(filepath.Separator)) {
			return p
		}
		return p + "/"
	}
	switch m.form {
	case "home":
		if m.home != "" && (m.cwd == m.home || strings.HasPrefix(m.cwd, m.home+string(filepath.Separator))) {
			return withSlash("~" + filepath.ToSlash(strings.TrimPrefix(m.cwd, m.home)))
		}
	case "rel":
		if rel, err := filepath.Rel(m.prompt.Invoke, m.cwd); err == nil {
			if rel == "." {
				return ""
			}
			return withSlash(filepath.ToSlash(rel))
		}
	}
	return withSlash(filepath.ToSlash(m.cwd))
}

// entries lists the directory, applying the junk list, the type and the
// caller's predicate. Directories come first, then by name.
func (m pathModel) entries() []pathEntry {
	items, _ := os.ReadDir(m.cwd)
	var dirs, files []pathEntry
	for _, item := range items {
		if isJunk(item.Name()) {
			continue
		}
		abs := filepath.Join(m.cwd, item.Name())
		isDir := item.IsDir()
		if item.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(abs); err == nil {
				isDir = info.IsDir()
			}
		}
		if m.prompt.Type == "dir" && !isDir {
			continue
		}
		if m.prompt.Show != nil && !m.prompt.Show(abs, isDir) {
			continue
		}
		if isDir {
			dirs = append(dirs, pathEntry{item.Name(), true})
		} else {
			files = append(files, pathEntry{item.Name(), false})
		}
	}
	byName := func(s []pathEntry) {
		sort.SliceStable(s, func(i, j int) bool { return s[i].name < s[j].name })
	}
	byName(dirs)
	byName(files)
	return append(dirs, files...)
}

func (m pathModel) rows() []pathRow {
	var out []pathRow
	if m.prompt.Type == "dir" && m.seg == "" {
		out = append(out, pathRow{pathEntry: pathEntry{name: ".", isDir: true}, here: true})
	}
	type scored struct {
		row   pathRow
		score int
	}
	var hits []scored
	for _, e := range m.entries() {
		score, positions, ok := fuzzyMatch(m.seg, e.name)
		if ok {
			hits = append(hits, scored{pathRow{pathEntry: e, positions: positions}, score})
		}
	}
	if m.seg != "" {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	}
	for _, h := range hits {
		out = append(out, h.row)
	}
	return out
}

// fuzzyMatch reports whether query's letters appear in target in order,
// with a score (higher is better) and the matched rune positions.
func fuzzyMatch(query, target string) (score int, positions []int, ok bool) {
	if query == "" {
		return 0, nil, true
	}
	q := []rune(strings.ToLower(query))
	t := []rune(strings.ToLower(target))
	qi, last := 0, -2
	for ti := 0; ti < len(t) && qi < len(q); ti++ {
		if t[ti] != q[qi] {
			continue
		}
		positions = append(positions, ti)
		if ti == last+1 {
			score += 5
		}
		if ti == 0 || strings.ContainsRune("_-.", t[ti-1]) {
			score += 3
		}
		score++
		last = ti
		qi++
	}
	if qi < len(q) {
		return 0, nil, false
	}
	return score*100 - len(t), positions, true
}

func highlightMatch(text string, positions []int) string {
	if len(positions) == 0 {
		return text
	}
	set := map[int]bool{}
	for _, p := range positions {
		set[p] = true
	}
	var b strings.Builder
	for i, r := range []rune(text) {
		if set[i] {
			b.WriteString(sPathMatch.Render(string(r)))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
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
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+pathListHeight {
		m.offset = m.cursor - pathListHeight + 1
	}
}

func (m *pathModel) setSegment(seg string) {
	m.seg = seg
	m.cursor, m.offset = 0, 0
}

// enter moves the listing into dir, keeping the written form.
func (m *pathModel) enter(dir string) {
	m.cwd = filepath.Clean(dir)
	m.setSegment("")
}

func (m *pathModel) up() {
	prev := m.cwd
	m.enter(filepath.Dir(m.cwd))
	for i, row := range m.rows() {
		if filepath.Join(m.cwd, row.name) == prev {
			m.cursor = i
		}
	}
	m.clamp()
}

// slash turns the segment into a directory: an exact name if there is one,
// else the highlighted directory.
func (m *pathModel) slash() {
	switch m.seg {
	case "":
		if m.dirText() == "" {
			m.form = "abs"
			m.enter(string(filepath.Separator))
		}
		return
	case "~":
		m.form = "home"
		m.enter(m.home)
		return
	case ".":
		m.setSegment("")
		return
	case "..":
		m.up()
		return
	}
	exact := filepath.Join(m.cwd, m.seg)
	if info, err := os.Stat(exact); err == nil && info.IsDir() {
		m.enter(exact)
		return
	}
	if row, ok := m.highlighted(); ok && row.isDir && !row.here {
		m.enter(filepath.Join(m.cwd, row.name))
		return
	}
	m.errMsg = `no directory matching "` + m.seg + `" here`
}

func (m pathModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
			m.enter(filepath.Join(m.cwd, row.name))
		}
	case "backspace":
		if m.seg == "" {
			m.up()
		} else {
			runes := []rune(m.seg)
			m.setSegment(string(runes[:len(runes)-1]))
		}
	case "ctrl+u":
		m.setSegment("")
	case "tab":
		if hasRow && !row.here {
			m.setSegment(row.name)
		}
	case "enter":
		switch {
		case !hasRow:
			m.errMsg = "nothing matches"
		case row.here:
			return m.finish(m.cwd)
		case row.isDir && m.prompt.Type == "file":
			m.enter(filepath.Join(m.cwd, row.name))
		default:
			return m.finish(filepath.Join(m.cwd, row.name))
		}
	default:
		if key.Type == tea.KeyRunes || key.Type == tea.KeySpace {
			text := string(key.Runes)
			if key.Type == tea.KeySpace {
				text = " "
			}
			for _, r := range text {
				if r == '/' {
					m.slash()
				} else {
					m.setSegment(m.seg + string(r))
				}
			}
		}
	}
	m.clamp()
	return m, nil
}

func (m pathModel) finish(path string) (tea.Model, tea.Cmd) {
	m.picked = path
	m.done = true
	return m, tea.Quit
}

func (m pathModel) View() string {
	if m.done {
		return ""
	}
	noun := "a file"
	if m.prompt.Type == "dir" {
		noun = "a directory"
	}
	var b strings.Builder
	b.WriteString("\nPick " + noun + " for " + SLabel.Render(m.prompt.VarName) + ".")
	if m.prompt.Info != "" {
		b.WriteString("\n" + SInfo.Render(m.prompt.Info))
	}
	b.WriteString("\n")
	dir := SInfo.Render("./")
	if text := m.dirText(); text != "" {
		dir = sPathDir.Render(text)
	}
	b.WriteString(SArrow.Render("› ") + dir + m.seg + sPathMatch.Render("▏") + "\n")

	rows := m.rows()
	if len(rows) == 0 {
		b.WriteString(SInfo.Render("  nothing matches") + "\n")
	}
	end := min(m.offset+pathListHeight, len(rows))
	if m.offset > 0 {
		b.WriteString(SInfo.Render(fmt.Sprintf("  ↑ %d more", m.offset)) + "\n")
	}
	for i := m.offset; i < end; i++ {
		row := rows[i]
		if i == m.cursor {
			b.WriteString(SArrow.Render("▶ "))
		} else {
			b.WriteString("  ")
		}
		switch {
		case row.here:
			b.WriteString(sPathDir.Render("./") + SInfo.Render(" (this directory)"))
		case row.isDir:
			b.WriteString(sPathDir.Render(highlightMatch(row.name, row.positions) + "/"))
		default:
			b.WriteString(highlightMatch(row.name, row.positions))
		}
		b.WriteString("\n")
	}
	if end < len(rows) {
		b.WriteString(SInfo.Render(fmt.Sprintf("  ↓ %d more", len(rows)-end)) + "\n")
	}
	if m.errMsg != "" {
		b.WriteString(SError.Render(m.errMsg) + "\n")
	}
	b.WriteString(SHint.Render("type to filter  / enter dir  ↑↓ move  ← up  → open  tab complete  enter pick"))
	return b.String()
}

// PromptPath runs the picker and returns the picked absolute path.
func PromptPath(ctx context.Context, prompt PathPrompt, task string, secret bool) (value.Value, error) {
	result, err := tea.NewProgram(newPathModel(prompt), tea.WithContext(ctx)).Run()
	if err != nil {
		return value.Value{}, err
	}
	final := result.(pathModel)
	if final.quit {
		return value.Value{}, fmt.Errorf("aborted")
	}
	display := final.picked
	if secret {
		display = SecretMask
	}
	printGetLine(task, prompt.VarName, display)
	return value.Str(final.picked), nil
}
